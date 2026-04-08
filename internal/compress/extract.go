package compress

import (
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/zhenninglang/mantis/internal/session"
)

var todoLinePattern = regexp.MustCompile(`^\d+\.\s+\[(completed|in_progress|pending)\]\s+(.*)$`)
var exitCodePattern = regexp.MustCompile(`\[Process exited with code (\d+)\]`)

const (
	compactionSummarySoftCapTokens = 2000
	compactionSummaryReserveTokens = 6000
	maxSummaryTurnTextLength       = 240
)

type toolUseRecord struct {
	Name  string
	Input any
}

type visibleTurn struct {
	RecentTurn
	VisibleIndex int
}

type phaseBuilder struct {
	Goal        string
	Outcome     string
	KeyFiles    []string
	KeyCommands []string
	OpenIssues  []string
	Skills      []string
	MaxVisible  int
}

func BuildCompressionInput(source session.Session, events []session.RawEvent) CompressionInput {
	anchorEventIndex := findLastCompactionAnchor(events)
	userMessages := collectMessageTexts(events, "user", anchorEventIndex)
	artifactTrail, unresolved := extractArtifacts(events, anchorEventIndex)
	compaction, recentWindow := buildCompactionWindow(events, anchorEventIndex)

	return CompressionInput{
		SourceSession: SourceSessionInfo{
			ID:      source.Meta.ID,
			Title:   source.Meta.Title,
			CWD:     source.Meta.WorkingDirectory,
			ModelID: source.Settings.Model,
		},
		Anchors: CompressionAnchors{
			InitialUserGoal: firstOrEmpty(userMessages),
			UserConstraints: collectAnchoredLines(userMessages, constraintMarkers()),
			UserCorrections: collectAnchoredLines(userMessages, correctionMarkers()),
			LastUserRequest: lastOrEmpty(userMessages),
		},
		TaskState:     extractTaskState(events),
		Compaction:    compaction,
		ArtifactTrail: artifactTrail,
		Errors: CompressionErrors{
			Unresolved: unresolved,
		},
		RecentWindow: recentWindow,
	}
}

func collectMessageTexts(events []session.RawEvent, role string, anchorEventIndex int) []string {
	var texts []string
	for i, event := range events {
		if i <= anchorEventIndex {
			continue
		}
		if event.Type != "message" || event.Message == nil || event.Message.Role != role {
			continue
		}
		if text := extractVisibleMessageText(event.Message.Content); text != "" {
			texts = append(texts, text)
		}
	}
	return texts
}

func extractVisibleMessageText(items []session.RawContentItem) string {
	var parts []string
	for _, item := range items {
		if item.Type != "text" {
			continue
		}
		if text := cleanVisibleText(item.Text); text != "" {
			parts = append(parts, text)
		}
	}
	return strings.TrimSpace(strings.Join(parts, "\n\n"))
}

func cleanVisibleText(text string) string {
	t := strings.TrimSpace(text)
	if t == "" {
		return ""
	}
	for _, prefix := range []string{"<system-reminder>", "<system-notification>", "<system_prompt>", "<system"} {
		if strings.HasPrefix(t, prefix) {
			return ""
		}
	}
	for _, prefix := range []string{"BYOK Error:"} {
		if strings.HasPrefix(t, prefix) {
			return ""
		}
	}
	return t
}

func collectAnchoredLines(messages []string, markers []string) []string {
	var out []string
	for _, msg := range messages {
		for _, block := range strings.Split(msg, "\n") {
			line := strings.TrimSpace(strings.TrimPrefix(block, "-"))
			if line == "" || !containsAny(line, markers) {
				continue
			}
			if !slices.Contains(out, line) {
				out = append(out, line)
			}
		}
	}
	return out
}

func constraintMarkers() []string {
	return []string{"不要", "必须", "记得", "only", "must", "do not", "don't", "support", "支持"}
}

func correctionMarkers() []string {
	return []string{"不是", "改成", "更正", "不对", "应该", "instead"}
}

func containsAny(text string, markers []string) bool {
	lower := strings.ToLower(text)
	for _, marker := range markers {
		if strings.Contains(lower, strings.ToLower(marker)) {
			return true
		}
	}
	return false
}

func extractTaskState(events []session.RawEvent) CompressionTaskState {
	for i := len(events) - 1; i >= 0; i-- {
		event := events[i]
		if event.Type != "todo_state" || event.Todos == nil {
			continue
		}
		return parseTodoState(event.Todos.Todos)
	}
	return CompressionTaskState{}
}

func parseTodoState(markdown string) CompressionTaskState {
	var state CompressionTaskState
	for _, line := range strings.Split(markdown, "\n") {
		match := todoLinePattern.FindStringSubmatch(strings.TrimSpace(line))
		if len(match) != 3 {
			continue
		}
		status, text := match[1], strings.TrimSpace(match[2])
		switch status {
		case "completed":
			state.Completed = append(state.Completed, text)
		case "in_progress":
			state.InProgress = append(state.InProgress, text)
		case "pending":
			state.Pending = append(state.Pending, text)
		}
	}
	return state
}

func extractArtifacts(events []session.RawEvent, anchorEventIndex int) (ArtifactTrail, []UnresolvedError) {
	var trail ArtifactTrail
	var unresolved []UnresolvedError
	pending := map[string]toolUseRecord{}

	for i, event := range events {
		if i <= anchorEventIndex {
			continue
		}
		if event.Type != "message" || event.Message == nil {
			continue
		}
		for _, item := range event.Message.Content {
			switch item.Type {
			case "tool_use":
				pending[item.ID] = toolUseRecord{Name: item.Name, Input: item.Input}
				recordToolUse(&trail, item.Name, item.Input)
			case "tool_result":
				call, ok := pending[item.ToolUseID]
				if !ok {
					continue
				}
				errText := recordToolResult(&trail, call, item)
				if errText != "" {
					unresolved = append(unresolved, UnresolvedError{
						Error:      errText,
						NextAction: fmt.Sprintf("Inspect command output for `%s`", extractCommand(call.Input)),
					})
				}
			}
		}
	}

	return trail, unresolved
}

func recordToolUse(trail *ArtifactTrail, name string, input any) {
	switch name {
	case "Read":
		for _, path := range extractPaths(input) {
			appendUniqueFileRead(&trail.FilesRead, ArtifactFileRead{Path: path})
		}
	case "ApplyPatch", "Edit", "MultiEdit":
		for _, path := range extractPaths(input) {
			appendUniqueFileChange(&trail.FilesModified, ArtifactFileChange{Path: path})
		}
	case "Create", "Write":
		for _, path := range extractPaths(input) {
			appendUniqueFileCreate(&trail.FilesCreated, ArtifactFileCreate{Path: path})
		}
	}
}

func recordToolResult(trail *ArtifactTrail, call toolUseRecord, result session.RawContentItem) string {
	switch call.Name {
	case "Execute":
		cmd := extractCommand(call.Input)
		status := "ok"
		if result.IsError || commandExitCode(result.Content) != 0 {
			status = "error"
		}
		evidence := firstMeaningfulLine(result.Content)
		trail.Commands = append(trail.Commands, ArtifactCommand{
			Cmd:      cmd,
			Status:   status,
			Evidence: evidence,
		})
		if strings.Contains(cmd, "git ") || strings.HasPrefix(cmd, "git ") {
			trail.GitOps = append(trail.GitOps, ArtifactGitOp{Op: cmd, Evidence: evidence})
		}
		if status == "error" {
			return evidence
		}
	case "Grep", "Glob":
		trail.Searches = append(trail.Searches, ArtifactSearch{
			Tool:    call.Name,
			Query:   extractSearchQuery(call.Input),
			Finding: firstMeaningfulLine(result.Content),
		})
	}
	return ""
}

func extractPaths(input any) []string {
	switch v := input.(type) {
	case map[string]any:
		for _, key := range []string{"file_path", "path", "filePath"} {
			if path, ok := v[key].(string); ok && strings.TrimSpace(path) != "" {
				return []string{path}
			}
		}
		if raw, ok := v["patterns"].([]any); ok {
			var out []string
			for _, item := range raw {
				if s, ok := item.(string); ok {
					out = append(out, s)
				}
			}
			if len(out) > 0 {
				return out
			}
		}
	case string:
		return extractApplyPatchPaths(v)
	}
	return nil
}

func extractApplyPatchPaths(patch string) []string {
	var paths []string
	for _, line := range strings.Split(patch, "\n") {
		line = strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(line, "*** Add File: "):
			paths = append(paths, strings.TrimPrefix(line, "*** Add File: "))
		case strings.HasPrefix(line, "*** Update File: "):
			paths = append(paths, strings.TrimPrefix(line, "*** Update File: "))
		}
	}
	return paths
}

func extractCommand(input any) string {
	if m, ok := input.(map[string]any); ok {
		if cmd, ok := m["command"].(string); ok {
			return cmd
		}
	}
	return ""
}

func extractSearchQuery(input any) string {
	if m, ok := input.(map[string]any); ok {
		switch {
		case m["pattern"] != nil:
			return stringify(m["pattern"])
		case m["patterns"] != nil:
			return stringify(m["patterns"])
		case m["query"] != nil:
			return stringify(m["query"])
		}
	}
	return ""
}

func stringify(v any) string {
	switch vv := v.(type) {
	case string:
		return vv
	case []any:
		var parts []string
		for _, item := range vv {
			parts = append(parts, stringify(item))
		}
		return strings.Join(parts, ", ")
	default:
		return fmt.Sprint(v)
	}
}

func commandExitCode(content string) int {
	match := exitCodePattern.FindStringSubmatch(content)
	if len(match) != 2 {
		return 0
	}
	if match[1] == "0" {
		return 0
	}
	return 1
}

func firstMeaningfulLine(content string) string {
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || exitCodePattern.MatchString(line) {
			continue
		}
		return line
	}
	return ""
}

func appendUniqueFileRead(items *[]ArtifactFileRead, item ArtifactFileRead) {
	for _, existing := range *items {
		if existing.Path == item.Path {
			return
		}
	}
	*items = append(*items, item)
}

func appendUniqueFileChange(items *[]ArtifactFileChange, item ArtifactFileChange) {
	for _, existing := range *items {
		if existing.Path == item.Path {
			return
		}
	}
	*items = append(*items, item)
}

func appendUniqueFileCreate(items *[]ArtifactFileCreate, item ArtifactFileCreate) {
	for _, existing := range *items {
		if existing.Path == item.Path {
			return
		}
	}
	*items = append(*items, item)
}

func buildCompactionWindow(events []session.RawEvent, anchorEventIndex int) (CompactionWindow, []RecentTurn) {
	turns := collectVisibleTurns(events, anchorEventIndex)
	preserveStart := len(turns)
	tokens := 0
	for i := len(turns) - 1; i >= 0; i-- {
		turnTokens := estimateTokens(turns[i].Text)
		if preserveStart == len(turns) || tokens+turnTokens <= compactionSummaryReserveTokens {
			preserveStart = i
			tokens += turnTokens
			continue
		}
		break
	}

	summarizedCandidates := turns[:preserveStart]
	preservedTurns := make([]RecentTurn, 0, len(turns)-preserveStart)
	for _, turn := range turns[preserveStart:] {
		preservedTurns = append(preservedTurns, turn.RecentTurn)
	}
	return CompactionWindow{
		AnchorMessageIndex:   anchorEventIndex,
		RemovedCount:         len(summarizedCandidates),
		PreservedCount:       len(preservedTurns),
		SummarySoftCapTokens: compactionSummarySoftCapTokens,
		SummaryReserveTokens: compactionSummaryReserveTokens,
		ActiveSkills:         collectActiveSkills(events, anchorEventIndex),
		SummarizedTurns:      compactTurnsForSummary(summarizedCandidates, compactionSummarySoftCapTokens),
		SummarizedPhases:     buildPhaseSummaries(events, anchorEventIndex, preserveStart),
	}, preservedTurns
}

func firstOrEmpty(items []string) string {
	if len(items) == 0 {
		return ""
	}
	return items[0]
}

func lastOrEmpty(items []string) string {
	if len(items) == 0 {
		return ""
	}
	return items[len(items)-1]
}

func collectVisibleTurns(events []session.RawEvent, anchorEventIndex int) []visibleTurn {
	var turns []visibleTurn
	visibleIndex := -1
	for i, event := range events {
		if i <= anchorEventIndex {
			continue
		}
		if event.Type != "message" || event.Message == nil {
			continue
		}
		text := extractVisibleMessageText(event.Message.Content)
		if text == "" {
			continue
		}
		visibleIndex++
		turns = append(turns, visibleTurn{
			RecentTurn:   RecentTurn{Role: event.Message.Role, Text: text},
			VisibleIndex: visibleIndex,
		})
	}
	return turns
}

func findLastCompactionAnchor(events []session.RawEvent) int {
	for i := len(events) - 1; i >= 0; i-- {
		event := events[i]
		if event.Type != "message" || event.Message == nil {
			continue
		}
		for _, item := range event.Message.Content {
			if item.Type == "text" && strings.Contains(item.Text, "<compressed_session_handoff") {
				return i
			}
		}
	}
	return -1
}

func collectActiveSkills(events []session.RawEvent, anchorEventIndex int) []string {
	var skills []string
	seen := map[string]bool{}
	for i, event := range events {
		if i <= anchorEventIndex || event.Type != "message" || event.Message == nil {
			continue
		}
		for _, item := range event.Message.Content {
			if item.Type != "tool_use" || item.Name != "Skill" {
				continue
			}
			if input, ok := item.Input.(map[string]any); ok {
				if skill, ok := input["skill"].(string); ok && strings.TrimSpace(skill) != "" && !seen[skill] {
					seen[skill] = true
					skills = append(skills, skill)
				}
			}
		}
	}
	return skills
}

func compactTurnsForSummary(turns []visibleTurn, softCap int) []RecentTurn {
	if len(turns) == 0 {
		return nil
	}
	clipped := make([]RecentTurn, 0, len(turns))
	for _, turn := range turns {
		clipped = append(clipped, RecentTurn{
			Role: turn.Role,
			Text: clipText(turn.Text, maxSummaryTurnTextLength),
		})
	}
	if totalTurnTokens(clipped) <= softCap {
		return clipped
	}

	var selected []RecentTurn
	add := func(turn RecentTurn) bool {
		for _, existing := range selected {
			if existing.Role == turn.Role && existing.Text == turn.Text {
				return true
			}
		}
		next := append(selected, turn)
		if totalTurnTokens(next) > softCap {
			return false
		}
		selected = next
		return true
	}

	for _, turn := range clipped[:minInt(2, len(clipped))] {
		add(turn)
	}
	for _, turn := range clipped[maxInt(len(clipped)-2, 0):] {
		add(turn)
	}
	if len(selected) == 0 {
		return []RecentTurn{clipped[0]}
	}
	if totalTurnTokens(selected) >= softCap || len(clipped) <= 4 {
		return selected
	}
	middle := clipped[2 : len(clipped)-2]
	if len(middle) == 0 {
		return selected
	}
	step := maxInt(1, len(middle)/4)
	for i := 0; i < len(middle); i += step {
		if !add(middle[i]) {
			break
		}
	}
	return selected
}

func totalTurnTokens(turns []RecentTurn) int {
	total := 0
	for _, turn := range turns {
		total += estimateTokens(turn.Text)
	}
	return total
}

func estimateTokens(text string) int {
	if strings.TrimSpace(text) == "" {
		return 0
	}
	return maxInt(1, (len(text)+3)/4)
}

func clipText(text string, maxLen int) string {
	text = strings.TrimSpace(text)
	if len(text) <= maxLen {
		return text
	}
	return text[:maxLen] + "..."
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func buildPhaseSummaries(events []session.RawEvent, anchorEventIndex, preserveStart int) []CompactionPhase {
	if preserveStart <= 0 {
		return nil
	}

	var phases []CompactionPhase
	var current *phaseBuilder
	pending := map[string]toolUseRecord{}
	visibleIndex := -1

	finalize := func() {
		if current == nil || current.MaxVisible >= preserveStart {
			current = nil
			return
		}
		if current.Goal == "" && current.Outcome == "" && len(current.KeyFiles) == 0 && len(current.KeyCommands) == 0 && len(current.OpenIssues) == 0 {
			current = nil
			return
		}
		phases = append(phases, CompactionPhase{
			Goal:        orDefault(current.Goal, "(implicit continuation)"),
			Outcome:     current.Outcome,
			KeyFiles:    append([]string{}, current.KeyFiles...),
			KeyCommands: append([]string{}, current.KeyCommands...),
			OpenIssues:  append([]string{}, current.OpenIssues...),
			Skills:      append([]string{}, current.Skills...),
		})
		current = nil
	}

	ensureCurrent := func() {
		if current != nil {
			return
		}
		current = &phaseBuilder{MaxVisible: visibleIndex}
	}

	for i, event := range events {
		if i <= anchorEventIndex {
			continue
		}
		if event.Type == "todo_state" && event.Todos != nil {
			ensureCurrent()
			current.MaxVisible = maxInt(current.MaxVisible, visibleIndex)
			state := parseTodoState(event.Todos.Todos)
			if len(state.InProgress) > 0 {
				current.setOutcome("Todo in progress: " + strings.Join(state.InProgress, "; "))
			}
			continue
		}
		if event.Type != "message" || event.Message == nil {
			continue
		}

		text := extractVisibleMessageText(event.Message.Content)
		hasVisibleText := text != ""
		if hasVisibleText {
			visibleIndex++
		}

		if event.Message.Role == "user" && hasVisibleText {
			finalize()
			current = &phaseBuilder{
				Goal:       normalizeUserGoal(text),
				MaxVisible: visibleIndex,
			}
		} else if hasVisibleText {
			ensureCurrent()
			current.MaxVisible = maxInt(current.MaxVisible, visibleIndex)
			if event.Message.Role == "assistant" {
				current.setOutcome(text)
			}
		}

		for _, item := range event.Message.Content {
			switch item.Type {
			case "tool_use":
				pending[item.ID] = toolUseRecord{Name: item.Name, Input: item.Input}
				ensureCurrent()
				current.MaxVisible = maxInt(current.MaxVisible, visibleIndex)
				recordPhaseToolUse(current, item.Name, item.Input)
			case "tool_result":
				call, ok := pending[item.ToolUseID]
				if !ok {
					continue
				}
				ensureCurrent()
				current.MaxVisible = maxInt(current.MaxVisible, visibleIndex)
				recordPhaseToolResult(current, call, item)
			}
		}
	}

	finalize()
	return phases
}

func normalizeUserGoal(text string) string {
	text = clipText(text, maxSummaryTurnTextLength)
	lines := strings.Split(text, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if isLikelyLogLine(line) {
			continue
		}
		return line
	}
	return text
}

func isLikelyLogLine(line string) bool {
	return strings.HasPrefix(line, "[") || strings.Contains(line, ".parquet") || strings.Contains(line, "rows")
}

func recordPhaseToolUse(phase *phaseBuilder, name string, input any) {
	switch name {
	case "Read", "ApplyPatch", "Edit", "MultiEdit", "Create", "Write":
		for _, path := range extractPaths(input) {
			phase.addFile(path)
		}
	case "Execute":
		if cmd := extractCommand(input); cmd != "" {
			phase.addCommand(cmd)
		}
	case "Skill":
		if m, ok := input.(map[string]any); ok {
			if skill, ok := m["skill"].(string); ok {
				phase.addSkill(skill)
			}
		}
	}
}

func recordPhaseToolResult(phase *phaseBuilder, call toolUseRecord, result session.RawContentItem) {
	switch call.Name {
	case "Execute":
		evidence := firstMeaningfulLine(result.Content)
		if result.IsError || commandExitCode(result.Content) != 0 {
			phase.addIssue(evidence)
			return
		}
		if evidence != "" {
			phase.setOutcome(evidence)
		}
	}
}

func (p *phaseBuilder) addFile(path string) {
	path = strings.TrimSpace(path)
	if path == "" {
		return
	}
	for _, existing := range p.KeyFiles {
		if existing == path {
			return
		}
	}
	if len(p.KeyFiles) < 5 {
		p.KeyFiles = append(p.KeyFiles, path)
	}
}

func (p *phaseBuilder) addCommand(cmd string) {
	cmd = clipText(strings.TrimSpace(cmd), 120)
	if cmd == "" {
		return
	}
	for _, existing := range p.KeyCommands {
		if existing == cmd {
			return
		}
	}
	if len(p.KeyCommands) < 4 {
		p.KeyCommands = append(p.KeyCommands, cmd)
	}
}

func (p *phaseBuilder) addIssue(issue string) {
	issue = clipText(strings.TrimSpace(issue), 120)
	if issue == "" {
		return
	}
	for _, existing := range p.OpenIssues {
		if existing == issue {
			return
		}
	}
	if len(p.OpenIssues) < 3 {
		p.OpenIssues = append(p.OpenIssues, issue)
	}
}

func (p *phaseBuilder) addSkill(skill string) {
	skill = strings.TrimSpace(skill)
	if skill == "" {
		return
	}
	for _, existing := range p.Skills {
		if existing == skill {
			return
		}
	}
	p.Skills = append(p.Skills, skill)
}

func (p *phaseBuilder) setOutcome(text string) {
	text = normalizeUserGoal(text)
	if text == "" {
		return
	}
	if p.Outcome == "" {
		p.Outcome = text
		return
	}
	if len(p.Outcome) >= maxSummaryTurnTextLength {
		return
	}
	if strings.Contains(p.Outcome, text) {
		return
	}
	p.Outcome = clipText(p.Outcome+" / "+text, maxSummaryTurnTextLength)
}
