package tui

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/zhenninglang/mantis/internal/action"
	"github.com/zhenninglang/mantis/internal/config"
	"github.com/zhenninglang/mantis/internal/session"
	"github.com/zhenninglang/mantis/internal/summary"
)

type viewMode int

const (
	viewList viewMode = iota
	viewStats
	viewConfirmDelete
	viewRename
	viewBatchSelect
	viewProjectSelect
)

type Model struct {
	sessions    []session.Session
	filtered    []int
	cursor      int
	search      textinput.Model
	rename      textinput.Model
	width       int
	height      int
	mode        viewMode
	deleteIdx   int
	resumeID    string
	quit        bool
	errMsg      string
	fullPath      bool
	version       string
	projectFilter string
	projects      []string
	projectCursor int
	projectQuery  string
	cfg           config.Config
	summaries     map[int]*summary.Summary // session index -> summary
	indexDone     int
	indexTotal    int
	indexCancel   context.CancelFunc
	indexCh       <-chan summary.Progress
}

type summaryUpdatedMsg struct {
	index   int
	summary *summary.Summary
	done    int
	total   int
}

type summaryDoneMsg struct{}

func New(sessions []session.Session, version string, cfg config.Config, cwd string) *Model {
	ti := textinput.New()
	ti.Placeholder = "Search sessions..."
	ti.Focus()
	ti.CharLimit = 200

	ri := textinput.New()
	ri.Placeholder = "New title..."
	ri.CharLimit = 200

	indices := make([]int, len(sessions))
	for i := range indices {
		indices[i] = i
	}

	// preload existing summaries
	sums := make(map[int]*summary.Summary, len(sessions))
	for i := range sessions {
		if s := summary.LoadSummary(sessions[i].FilePath); s != nil {
			sums[i] = s
		}
	}

	// auto-filter to current directory's project
	var initFilter string
	if cwd != "" {
		cwd = filepath.Clean(cwd)
		bestLen := 0
		for i := range sessions {
			sp := filepath.Clean(sessions[i].ProjectFull)
			if sp == "" {
				continue
			}
			if sp == cwd {
				initFilter = sessions[i].ProjectShort()
				break
			}
			// cwd is under this project — pick the longest (most specific) match
			if strings.HasPrefix(cwd, sp+"/") && len(sp) > bestLen {
				bestLen = len(sp)
				initFilter = sessions[i].ProjectShort()
			}
		}
	}

	m := &Model{
		sessions:      sessions,
		filtered:      indices,
		search:        ti,
		rename:        ri,
		version:       version,
		projects:      collectProjects(sessions),
		cfg:           cfg,
		summaries:     sums,
		projectFilter: initFilter,
	}
	if initFilter != "" {
		m.refilter()
	}
	return m
}

func collectProjects(sessions []session.Session) []string {
	seen := map[string]bool{}
	for i := range sessions {
		seen[sessions[i].ProjectShort()] = true
	}
	projects := make([]string, 0, len(seen))
	for p := range seen {
		projects = append(projects, p)
	}
	sort.Slice(projects, func(i, j int) bool {
		return strings.ToLower(projects[i]) < strings.ToLower(projects[j])
	})
	return projects
}

func (m *Model) Init() tea.Cmd {
	cmds := []tea.Cmd{textinput.Blink}
	if m.cfg.HasLLM() {
		cmds = append(cmds, m.startIndexing())
	}
	return tea.Batch(cmds...)
}

func (m *Model) startIndexing() tea.Cmd {
	ctx, cancel := context.WithCancel(context.Background())
	m.indexCancel = cancel
	ch, total := summary.GenerateMissing(ctx, m.cfg.LLM, m.sessions)
	m.indexCh = ch
	m.indexTotal = total
	return m.waitForNextSummary()
}

func (m *Model) waitForNextSummary() tea.Cmd {
	ch := m.indexCh
	return func() tea.Msg {
		p, ok := <-ch
		if !ok {
			return summaryDoneMsg{}
		}
		return summaryUpdatedMsg{
			index:   p.Index,
			summary: p.Summary,
			done:    p.Done,
			total:   p.Total,
		}
	}
}

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case tea.KeyMsg:
		return m.handleKey(msg)

	case summaryUpdatedMsg:
		m.indexDone = msg.done
		m.indexTotal = msg.total
		if msg.summary != nil {
			m.summaries[msg.index] = msg.summary
		}
		m.refilter()
		return m, m.waitForNextSummary()

	case summaryDoneMsg:
		m.indexDone = m.indexTotal
		return m, nil
	}

	// pass to active text input
	var cmd tea.Cmd
	switch m.mode {
	case viewRename:
		m.rename, cmd = m.rename.Update(msg)
	default:
		old := m.search.Value()
		m.search, cmd = m.search.Update(msg)
		if m.search.Value() != old {
			m.refilter()
		}
	}
	return m, cmd
}

func (m *Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()

	// global keys
	if key == "ctrl+c" {
		m.quit = true
		m.cancelIndexing()
		return m, tea.Quit
	}

	switch m.mode {
	case viewConfirmDelete:
		return m.handleConfirmDelete(key)
	case viewRename:
		return m.handleRename(msg)
	case viewStats:
		return m.handleStats(key)
	case viewBatchSelect:
		return m.handleBatchSelect(key)
	case viewProjectSelect:
		return m.handleProjectSelect(msg)
	}

	// list mode: search box is focused, so only intercept navigation/action keys
	// let all other keys pass through to the search input
	searching := m.search.Value() != ""

	switch key {
	case "up":
		if m.cursor > 0 {
			m.cursor--
		}
	case "down":
		if m.cursor < len(m.filtered)-1 {
			m.cursor++
		}
	case "enter":
		if s := m.selectedSession(); s != nil {
			m.resumeID = s.Meta.ID
			return m, tea.Quit
		}
	case "esc":
		if searching {
			m.search.SetValue("")
			m.refilter()
		} else if m.projectFilter != "" {
			m.projectFilter = ""
			m.refilter()
		} else {
			m.quit = true
			return m, tea.Quit
		}
	case "ctrl+d":
		if s := m.selectedSession(); s != nil {
			m.mode = viewConfirmDelete
			m.deleteIdx = m.filtered[m.cursor]
		}
	case "ctrl+x":
		m.mode = viewBatchSelect
		m.search.Blur()
	case "ctrl+r":
		if s := m.selectedSession(); s != nil {
			m.mode = viewRename
			m.search.Blur()
			m.rename.SetValue(s.Meta.Title)
			m.rename.Focus()
			return m, textinput.Blink
		}
	case "ctrl+s":
		m.mode = viewStats
		m.search.Blur()
	case "ctrl+p":
		m.mode = viewProjectSelect
		m.projectCursor = 0
		m.projectQuery = ""
		m.search.Blur()
	case "ctrl+q":
		m.quit = true
		return m, tea.Quit
	case "tab":
		m.fullPath = !m.fullPath
	default:
		var cmd tea.Cmd
		old := m.search.Value()
		m.search, cmd = m.search.Update(msg)
		if m.search.Value() != old {
			m.refilter()
		}
		return m, cmd
	}

	return m, nil
}

func (m *Model) filteredProjects() []string {
	if m.projectQuery == "" {
		return m.projects
	}
	q := strings.ToLower(m.projectQuery)
	var result []string
	for _, p := range m.projects {
		if strings.Contains(strings.ToLower(p), q) {
			result = append(result, p)
		}
	}
	return result
}

func (m *Model) handleProjectSelect(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()
	filtered := m.filteredProjects()
	total := len(filtered) + 1 // +1 for "All"

	switch key {
	case "up", "k":
		if m.projectCursor > 0 {
			m.projectCursor--
		}
	case "down", "j":
		if m.projectCursor < total-1 {
			m.projectCursor++
		}
	case "enter":
		if m.projectCursor == 0 {
			m.projectFilter = ""
		} else if m.projectCursor-1 < len(filtered) {
			m.projectFilter = filtered[m.projectCursor-1]
		}
		m.refilter()
		m.mode = viewList
		m.search.Focus()
	case "esc":
		m.mode = viewList
		m.search.Focus()
	case "backspace":
		if len(m.projectQuery) > 0 {
			m.projectQuery = m.projectQuery[:len(m.projectQuery)-1]
			m.projectCursor = 0
		}
	default:
		if len(key) == 1 && key[0] >= 32 && key[0] < 127 {
			m.projectQuery += key
			m.projectCursor = 0
		}
	}
	return m, nil
}

func (m *Model) handleStats(key string) (tea.Model, tea.Cmd) {
	if key == "ctrl+s" || key == "esc" {
		m.mode = viewList
		m.search.Focus()
	}
	return m, nil
}

func (m *Model) handleConfirmDelete(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "y", "Y":
		s := &m.sessions[m.deleteIdx]
		if err := action.Delete(s); err != nil {
			m.errMsg = fmt.Sprintf("Delete failed: %v", err)
		} else {
			m.sessions = append(m.sessions[:m.deleteIdx], m.sessions[m.deleteIdx+1:]...)
			m.refilter()
			if m.cursor >= len(m.filtered) {
				m.cursor = max(0, len(m.filtered)-1)
			}
		}
		m.mode = viewList
		m.search.Focus()
	case "n", "N", "esc":
		m.mode = viewList
		m.search.Focus()
	}
	return m, nil
}

func (m *Model) handleRename(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()
	switch key {
	case "enter":
		newTitle := m.rename.Value()
		if newTitle != "" {
			s := m.selectedSession()
			if s != nil {
				if err := action.Rename(s, newTitle); err != nil {
					m.errMsg = fmt.Sprintf("Rename failed: %v", err)
				} else {
					s.Meta.Title = newTitle
				}
			}
		}
		m.mode = viewList
		m.rename.SetValue("")
		m.search.Focus()
		return m, textinput.Blink
	case "esc":
		m.mode = viewList
		m.rename.SetValue("")
		m.search.Focus()
		return m, textinput.Blink
	default:
		var cmd tea.Cmd
		m.rename, cmd = m.rename.Update(msg)
		return m, cmd
	}
}

func (m *Model) handleBatchSelect(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
		}
	case "down", "j":
		if m.cursor < len(m.filtered)-1 {
			m.cursor++
		}
	case "tab", " ":
		if s := m.selectedSession(); s != nil {
			s.Selected = !s.Selected
		}
	case "d", "enter":
		// delete all selected
		count := 0
		var remaining []session.Session
		for i := range m.sessions {
			if m.sessions[i].Selected {
				action.Delete(&m.sessions[i])
				count++
			} else {
				remaining = append(remaining, m.sessions[i])
			}
		}
		m.sessions = remaining
		m.refilter()
		if m.cursor >= len(m.filtered) {
			m.cursor = max(0, len(m.filtered)-1)
		}
		m.errMsg = fmt.Sprintf("Deleted %d sessions", count)
		m.mode = viewList
		m.search.Focus()
	case "esc", "q":
		// cancel batch, clear selections
		for i := range m.sessions {
			m.sessions[i].Selected = false
		}
		m.mode = viewList
		m.search.Focus()
	}
	return m, nil
}

func (m *Model) refilter() {
	m.filtered = filterSessions(m.sessions, m.search.Value(), m.projectFilter, m.summaries)
	if m.cursor >= len(m.filtered) {
		m.cursor = max(0, len(m.filtered)-1)
	}
}

func (m *Model) selectedSession() *session.Session {
	if m.cursor < 0 || m.cursor >= len(m.filtered) {
		return nil
	}
	return &m.sessions[m.filtered[m.cursor]]
}

func (m *Model) View() string {
	if m.width == 0 {
		return "Loading..."
	}

	switch m.mode {
	case viewStats:
		return borderStyle.Width(m.width - 2).Height(m.height - 2).Render(
			renderStats(m.sessions, m.width-4, m.height-4))
	case viewProjectSelect:
		return borderStyle.Width(m.width - 2).Height(m.height - 2).Render(
			m.renderProjectSelect(m.width-4, m.height-4))
	}

	var b strings.Builder

	// header
	header := titleStyle.Render("mantis") + dimStyle.Render(" v"+m.version) + " " + m.search.View()
	if m.projectFilter != "" {
		header += " " + projectStyle.Render("["+m.projectFilter+"]")
	}
	b.WriteString(header)
	b.WriteString("\n")

	// index status line
	indexed := len(m.summaries)
	total := len(m.sessions)
	filtered := len(m.filtered)
	skipped := 0
	for _, s := range m.summaries {
		if s != nil && s.Title == "" {
			skipped++
		}
	}
	summarized := indexed - skipped
	waiting := total - indexed
	statusLine := dimStyle.Render(fmt.Sprintf("%d total, %d shown, %d indexed, %d skipped, %d waiting",
		total, filtered, summarized, skipped, waiting))
	if m.indexTotal > 0 && m.indexDone < m.indexTotal {
		statusLine += dimStyle.Render(fmt.Sprintf("  (indexing %d/%d...)", m.indexDone, m.indexTotal))
	}
	b.WriteString(statusLine)
	b.WriteString("\n")

	b.WriteString(dimStyle.Render(strings.Repeat("─", m.width)))
	b.WriteString("\n")

	// calculate layout (header=2 lines + separator + status bar + separator + bottom = 7 overhead)
	listHeight := (m.height - 7) * 2 / 3
	if listHeight < 3 {
		listHeight = 3
	}
	previewHeight := m.height - listHeight - 7

	// list
	start := 0
	if m.cursor >= start+listHeight {
		start = m.cursor - listHeight + 1
	}
	if m.cursor < start {
		start = m.cursor
	}

	for i := 0; i < listHeight; i++ {
		idx := start + i
		if idx >= len(m.filtered) {
			b.WriteString("\n")
			continue
		}
		s := &m.sessions[m.filtered[idx]]
		isSelected := idx == m.cursor
		b.WriteString(renderListItem(s, m.summaries[m.filtered[idx]], m.width, isSelected, s.Selected, m.fullPath))
		b.WriteString("\n")
	}

	// separator
	b.WriteString(dimStyle.Render(strings.Repeat("─", m.width)))
	b.WriteString("\n")

	// preview
	if previewHeight > 0 {
		var selSum *summary.Summary
		if sel := m.selectedSession(); sel != nil {
			selSum = m.summaries[m.filtered[m.cursor]]
		}
		preview := renderPreview(m.selectedSession(), selSum, m.width)
		lines := strings.Split(preview, "\n")
		for i := 0; i < previewHeight && i < len(lines); i++ {
			b.WriteString(lines[i])
			b.WriteString("\n")
		}
	}

	// status bar
	b.WriteString(dimStyle.Render(strings.Repeat("─", m.width)))
	b.WriteString("\n")

	switch m.mode {
	case viewConfirmDelete:
		s := &m.sessions[m.deleteIdx]
		b.WriteString(confirmStyle.Render(fmt.Sprintf("Delete \"%s\"? (y/n)", s.Meta.Title)))
	case viewRename:
		b.WriteString("Rename: " + m.rename.View())
	case viewBatchSelect:
		selected := 0
		for i := range m.sessions {
			if m.sessions[i].Selected {
				selected++
			}
		}
		b.WriteString(markedStyle.Render(fmt.Sprintf("BATCH SELECT (%d marked)", selected)) +
			helpStyle.Render("  Tab:mark  d:delete marked  Esc:cancel"))
	default:
		if !m.cfg.HasLLM() {
			b.WriteString(dimStyle.Render("Run `mantis config` for smart search.") + "  ")
		} else if m.errMsg != "" {
			b.WriteString(dimStyle.Render(m.errMsg) + "  ")
		}
		help := helpStyle.Render("↑↓:nav  Enter:resume  Tab:path  ^P:project  ^D:del  ^X:batch  ^R:rename  ^S:stats  Esc:quit")
		b.WriteString(help)
	}

	return lipgloss.NewStyle().MaxWidth(m.width).MaxHeight(m.height).Render(b.String())
}

func (m *Model) cancelIndexing() {
	if m.indexCancel != nil {
		m.indexCancel()
	}
}

// ResumeID returns the session ID to resume after quitting, or empty string.
func (m *Model) ResumeID() string {
	return m.resumeID
}

func (m *Model) Quit() bool {
	return m.quit
}

func (m *Model) renderProjectSelect(width, height int) string {
	var b strings.Builder
	b.WriteString(previewTitleStyle.Render("Filter by Project"))

	if m.projectQuery != "" {
		b.WriteString("  " + dimStyle.Render("> "+m.projectQuery+"_"))
	} else {
		b.WriteString("  " + dimStyle.Render("type to search..."))
	}
	b.WriteString("\n\n")

	filtered := m.filteredProjects()
	items := make([]string, 0, len(filtered)+1)
	items = append(items, "All")
	items = append(items, filtered...)

	start := 0
	visible := height - 5
	if visible < 3 {
		visible = 3
	}
	if m.projectCursor >= start+visible {
		start = m.projectCursor - visible + 1
	}

	for i := start; i < len(items) && i < start+visible; i++ {
		label := items[i]
		prefix := "  "
		if m.projectFilter != "" && label == m.projectFilter {
			prefix = "● "
		}
		if m.projectFilter == "" && i == 0 {
			prefix = "● "
		}

		line := fmt.Sprintf("%s%s", prefix, label)
		if i == m.projectCursor {
			b.WriteString(selectedStyle.Width(width).Render(line))
		} else {
			b.WriteString(normalStyle.Render(line))
		}
		b.WriteString("\n")
	}

	b.WriteString("\n")
	b.WriteString(helpStyle.Render("↑↓:nav  Enter:select  Esc:cancel"))
	return b.String()
}
