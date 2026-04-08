package compress

import (
	"bufio"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/zhenninglang/mantis/internal/session"
)

const (
	recentTranscriptMaxTurns         = 20
	recentTranscriptSoftCapTokens    = 6000
	recentTranscriptMaxTurnTextRunes = 1600
)

func CreateCompressedSession(source session.Session, handoff LLMHandoff) (string, error) {
	newID, err := newUUID()
	if err != nil {
		return "", fmt.Errorf("generate session id: %w", err)
	}

	startEvent, err := readJSONLFirstLine(source.FilePath)
	if err != nil {
		return "", fmt.Errorf("read source session start: %w", err)
	}
	settingsPath := strings.TrimSuffix(source.FilePath, ".jsonl") + ".settings.json"
	settings, err := readJSONMap(settingsPath)
	if err != nil {
		return "", fmt.Errorf("read source settings: %w", err)
	}
	if source.Settings.Model == "" {
		if model, ok := settings["model"].(string); ok {
			source.Settings.Model = model
		}
	}

	newTitle := source.Meta.Title + " [compressed]"
	startEvent["type"] = "session_start"
	startEvent["id"] = newID
	startEvent["title"] = newTitle
	if source.Meta.WorkingDirectory != "" {
		startEvent["cwd"] = source.Meta.WorkingDirectory
	}
	if startEvent["version"] == nil {
		startEvent["version"] = 2
	}

	handoffText := RenderHandoffMessage(source, handoff)
	messageEvents := buildCompressedSessionMessages(newID, handoffText, handoff.RecentTranscript)

	settings["assistantActiveTimeMs"] = 0
	settings["tokenUsage"] = map[string]any{
		"inputTokens":         0,
		"outputTokens":        0,
		"cacheCreationTokens": 0,
		"cacheReadTokens":     0,
		"thinkingTokens":      0,
	}
	if settings["model"] == nil && source.Settings.Model != "" {
		settings["model"] = source.Settings.Model
	}

	dir := filepath.Dir(source.FilePath)
	jsonlPath := filepath.Join(dir, newID+".jsonl")
	settingsOut := filepath.Join(dir, newID+".settings.json")
	if err := writeJSONL(jsonlPath, startEvent, messageEvents...); err != nil {
		return "", err
	}
	if err := writeJSONFile(settingsOut, settings); err != nil {
		return "", err
	}
	return newID, nil
}

func RenderHandoffMessage(source session.Session, handoff LLMHandoff) string {
	var b strings.Builder
	fmt.Fprintf(&b, "<compressed_session_handoff source_session=%q source_model=%q cwd=%q>\n\n", source.Meta.ID, source.Settings.Model, source.Meta.WorkingDirectory)

	fmt.Fprintf(&b, "Objective: %s\n", orDefault(handoff.Objective, "(empty)"))

	if len(handoff.Constraints) > 0 {
		b.WriteString("\nConstraints:\n")
		writeBullets(&b, handoff.Constraints, "")
	}

	if len(handoff.CompactedHistory) > 0 {
		b.WriteString("\nHistory:\n")
		for i, phase := range handoff.CompactedHistory {
			if phase.Outcome != "" {
				fmt.Fprintf(&b, "%d. %s → %s\n", i+1, orDefault(phase.Goal, "?"), phase.Outcome)
			} else {
				fmt.Fprintf(&b, "%d. %s\n", i+1, orDefault(phase.Goal, "?"))
			}
		}
	}

	writeCompactTaskState(&b, handoff.TaskState)

	if len(handoff.ArtifactFocus.MustKeepFiles) > 0 {
		b.WriteString("\nKey files:\n")
		for _, f := range handoff.ArtifactFocus.MustKeepFiles {
			if f.Reason != "" {
				fmt.Fprintf(&b, "- %s — %s\n", f.Path, f.Reason)
			} else {
				fmt.Fprintf(&b, "- %s\n", f.Path)
			}
		}
	}

	if len(handoff.ArtifactFocus.UnresolvedErrors) > 0 {
		b.WriteString("\nUnresolved errors:\n")
		for _, e := range handoff.ArtifactFocus.UnresolvedErrors {
			fmt.Fprintf(&b, "- %s\n", e.Error)
		}
	}

	if len(handoff.ActiveSkills) > 0 {
		fmt.Fprintf(&b, "\nActive skills: %s\n", strings.Join(handoff.ActiveSkills, ", "))
	}

	if len(handoff.CurrentState.NextSteps) > 0 {
		b.WriteString("\nNext steps:\n")
		writeNumberedList(&b, handoff.CurrentState.NextSteps)
	}

	b.WriteString("\nRules:\n")
	b.WriteString("1. Continue from this handoff.\n")
	b.WriteString("2. Do not ask the user to restate prior context.\n")
	b.WriteString("3. Treat file paths / symbols / commands verbatim.\n")
	if handoff.ResumeInstruction != "" {
		fmt.Fprintf(&b, "4. %s\n", handoff.ResumeInstruction)
	}
	b.WriteString("\n</compressed_session_handoff>\n\n")

	if len(handoff.RecentTranscript) > 0 {
		b.WriteString("后续消息保留了最近的原始对话，请结合这些消息继续。")
	} else {
		b.WriteString("请从 `next_steps[0]` 开始继续。")
	}
	return b.String()
}

func writeCompactTaskState(b *strings.Builder, ts HandoffTaskState) {
	hasAny := len(ts.Completed) > 0 || len(ts.InProgress) > 0 || len(ts.Pending) > 0
	if !hasAny {
		return
	}
	b.WriteString("\nTask state:\n")
	if len(ts.Completed) > 0 {
		fmt.Fprintf(b, "- done: %s\n", strings.Join(ts.Completed, "; "))
	}
	if len(ts.InProgress) > 0 {
		fmt.Fprintf(b, "- in_progress: %s\n", strings.Join(ts.InProgress, "; "))
	}
	if len(ts.Pending) > 0 {
		fmt.Fprintf(b, "- pending: %s\n", strings.Join(ts.Pending, "; "))
	}
}

func buildCompressedSessionMessages(sessionID, handoffText string, turns []RecentTurn) []map[string]any {
	events := []map[string]any{newMessageEvent(sessionID, "assistant", handoffText)}
	for _, turn := range turns {
		if strings.TrimSpace(turn.Text) == "" {
			continue
		}
		role := strings.TrimSpace(turn.Role)
		if role == "" {
			role = "user"
		}
		events = append(events, newMessageEvent(sessionID, role, turn.Text))
	}
	return events
}

func newMessageEvent(sessionID, role, text string) map[string]any {
	return map[string]any{
		"type":      "message",
		"id":        sessionID,
		"timestamp": time.Now().UTC().Format(time.RFC3339Nano),
		"message": map[string]any{
			"role": role,
			"content": []map[string]any{{
				"type": "text",
				"text": text,
			}},
		},
	}
}

func writeJSONL(path string, startEvent map[string]any, messageEvents ...map[string]any) error {
	startJSON, err := marshalCompactJSON(startEvent)
	if err != nil {
		return fmt.Errorf("marshal session start: %w", err)
	}
	content := append(startJSON, '\n')
	for _, messageEvent := range messageEvents {
		messageJSON, err := marshalCompactJSON(messageEvent)
		if err != nil {
			return fmt.Errorf("marshal handoff message: %w", err)
		}
		content = append(content, messageJSON...)
		content = append(content, '\n')
	}
	if err := os.WriteFile(path, content, 0o644); err != nil {
		return fmt.Errorf("write session file: %w", err)
	}
	return nil
}

func writeJSONFile(path string, value map[string]any) error {
	data, err := marshalCompactJSON(value)
	if err != nil {
		return fmt.Errorf("marshal settings: %w", err)
	}
	if err := os.WriteFile(path, append(data, '\n'), 0o600); err != nil {
		return fmt.Errorf("write settings file: %w", err)
	}
	return nil
}

func marshalCompactJSON(value any) ([]byte, error) {
	var buf strings.Builder
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(value); err != nil {
		return nil, err
	}
	return []byte(strings.TrimSuffix(buf.String(), "\n")), nil
}

func readJSONLFirstLine(path string) (map[string]any, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 1024*1024), 2*1024*1024)
	if !scanner.Scan() {
		if err := scanner.Err(); err != nil {
			return nil, err
		}
		return nil, fmt.Errorf("empty session file")
	}
	var out map[string]any
	if err := json.Unmarshal(scanner.Bytes(), &out); err != nil {
		return nil, err
	}
	return out, nil
}

func readJSONMap(path string) (map[string]any, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var out map[string]any
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func writeBullets(b *strings.Builder, items []string, fallback string) {
	if len(items) == 0 {
		b.WriteString(fallback)
		b.WriteByte('\n')
		return
	}
	for _, item := range items {
		b.WriteString("- ")
		b.WriteString(item)
		b.WriteByte('\n')
	}
}

func writeIndentedBullets(b *strings.Builder, items []string) {
	if len(items) == 0 {
		b.WriteString("  - (none)\n")
		return
	}
	for _, item := range items {
		b.WriteString("  - ")
		b.WriteString(item)
		b.WriteByte('\n')
	}
}

func writeNumberedList(b *strings.Builder, items []string) {
	if len(items) == 0 {
		b.WriteString("  1. (none)\n")
		return
	}
	for i, item := range items {
		fmt.Fprintf(b, "  %d. %s\n", i+1, item)
	}
}

func writeCompactedHistory(b *strings.Builder, phases []CompactionPhase) {
	if len(phases) == 0 {
		b.WriteString("- (none)\n")
		return
	}
	for i, phase := range phases {
		fmt.Fprintf(b, "%d. goal: %s\n", i+1, orDefault(phase.Goal, "(empty)"))
		if phase.Outcome != "" {
			fmt.Fprintf(b, "   outcome: %s\n", phase.Outcome)
		}
		if len(phase.KeyFiles) > 0 {
			fmt.Fprintf(b, "   key_files: %s\n", formatInlineList(phase.KeyFiles))
		}
		if len(phase.KeyCommands) > 0 {
			fmt.Fprintf(b, "   key_commands: %s\n", formatInlineList(phase.KeyCommands))
		}
		if len(phase.OpenIssues) > 0 {
			fmt.Fprintf(b, "   open_issues: %s\n", formatInlineList(phase.OpenIssues))
		}
		if len(phase.Skills) > 0 {
			fmt.Fprintf(b, "   skills: %s\n", formatInlineList(phase.Skills))
		}
	}
}

func formatInlineList(items []string) string {
	if len(items) == 0 {
		return "(none)"
	}
	return strings.Join(items, "; ")
}

func orDefault(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func buildRecentTranscript(turns []RecentTurn) []RecentTurn {
	if len(turns) == 0 {
		return nil
	}

	selected := make([]RecentTurn, 0, minInt(len(turns), recentTranscriptMaxTurns))
	tokens := 0
	for i := len(turns) - 1; i >= 0 && len(selected) < recentTranscriptMaxTurns; i-- {
		normalized := normalizeRecentTranscriptText(turns[i].Text)
		if normalized == "" {
			continue
		}

		turn := RecentTurn{
			Role: turns[i].Role,
			Text: normalized,
		}
		turnTokens := estimateTokens(turn.Text)
		if len(selected) > 0 && tokens+turnTokens > recentTranscriptSoftCapTokens {
			break
		}
		tokens += turnTokens
		selected = append(selected, turn)
	}

	for i, j := 0, len(selected)-1; i < j; i, j = i+1, j-1 {
		selected[i], selected[j] = selected[j], selected[i]
	}
	return selected
}

func normalizeRecentTranscriptText(text string) string {
	compacted := strings.Join(strings.Fields(strings.TrimSpace(text)), " ")
	if compacted == "" {
		return ""
	}
	return clipMiddleRunes(compacted, recentTranscriptMaxTurnTextRunes)
}

func clipMiddleRunes(text string, maxRunes int) string {
	runes := []rune(text)
	if maxRunes <= 0 || len(runes) <= maxRunes {
		return text
	}
	head := maxRunes * 2 / 3
	tail := maxRunes - head - len([]rune(" ... "))
	if tail < 8 {
		tail = 8
		head = maxRunes - tail - len([]rune(" ... "))
	}
	if head < 8 {
		head = 8
	}
	return string(runes[:head]) + " ... " + string(runes[len(runes)-tail:])
}

func newUUID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		b[0:4],
		b[4:6],
		b[6:8],
		b[8:10],
		b[10:16],
	), nil
}
