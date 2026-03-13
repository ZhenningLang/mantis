package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/zhenninglang/mantis/internal/action"
	"github.com/zhenninglang/mantis/internal/session"
)

type viewMode int

const (
	viewList viewMode = iota
	viewStats
	viewConfirmDelete
	viewRename
	viewBatchSelect
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
	fullPath    bool // toggle project name display
}

func New(sessions []session.Session) *Model {
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

	return &Model{
		sessions: sessions,
		filtered: indices,
		search:   ti,
		rename:   ri,
	}
}

func (m *Model) Init() tea.Cmd {
	return textinput.Blink
}

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case tea.KeyMsg:
		return m.handleKey(msg)
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
		return m, tea.Quit
	}

	switch m.mode {
	case viewConfirmDelete:
		return m.handleConfirmDelete(key)
	case viewRename:
		return m.handleRename(msg)
	case viewStats:
		if key == "ctrl+s" || key == "esc" {
			m.mode = viewList
			m.search.Focus()
		}
		return m, nil
	case viewBatchSelect:
		return m.handleBatchSelect(key)
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
	m.filtered = filterSessions(m.sessions, m.search.Value())
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
	}

	var b strings.Builder

	// header
	header := titleStyle.Render("mantis") + " " + m.search.View() +
		dimStyle.Render(fmt.Sprintf("  [%d/%d]", len(m.filtered), len(m.sessions)))
	b.WriteString(header)
	b.WriteString("\n")
	b.WriteString(dimStyle.Render(strings.Repeat("─", m.width)))
	b.WriteString("\n")

	// calculate layout
	listHeight := (m.height - 6) * 2 / 3
	if listHeight < 3 {
		listHeight = 3
	}
	previewHeight := m.height - listHeight - 6

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
		b.WriteString(renderListItem(s, m.width, isSelected, s.Selected, m.fullPath))
		b.WriteString("\n")
	}

	// separator
	b.WriteString(dimStyle.Render(strings.Repeat("─", m.width)))
	b.WriteString("\n")

	// preview
	if previewHeight > 0 {
		preview := renderPreview(m.selectedSession(), m.width)
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
		if m.errMsg != "" {
			b.WriteString(dimStyle.Render(m.errMsg) + "  ")
		}
		help := helpStyle.Render("↑↓:nav  Enter:resume  Tab:path  ^D:del  ^X:batch  ^R:rename  ^S:stats  Esc:quit")
		b.WriteString(help)
	}

	return lipgloss.NewStyle().MaxWidth(m.width).MaxHeight(m.height).Render(b.String())
}

// ResumeID returns the session ID to resume after quitting, or empty string.
func (m *Model) ResumeID() string {
	return m.resumeID
}

func (m *Model) Quit() bool {
	return m.quit
}
