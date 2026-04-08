package compress

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zhenninglang/mantis/internal/session"
)

func TestCreateCompressedSessionWritesJSONLAndSettings(t *testing.T) {
	dir := t.TempDir()
	sourceID := "source-session"
	sourceFile := filepath.Join(dir, sourceID+".jsonl")
	settingsFile := filepath.Join(dir, sourceID+".settings.json")

	firstLine := map[string]any{
		"type":         "session_start",
		"id":           sourceID,
		"title":        "Original Title",
		"sessionTitle": "New Session",
		"owner":        "tester",
		"version":      2,
		"cwd":          "/tmp/project",
	}
	firstJSON, _ := json.Marshal(firstLine)
	messageLine := `{"type":"message","message":{"role":"user","content":[{"type":"text","text":"hello"}]}}`
	if err := os.WriteFile(sourceFile, []byte(string(firstJSON)+"\n"+messageLine+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	settingsJSON := `{"assistantActiveTimeMs":12,"model":"custom:GPT-5.4-1","reasoningEffort":"high","autonomyMode":"auto-high","tokenUsage":{"inputTokens":1,"outputTokens":2,"cacheCreationTokens":3,"cacheReadTokens":4,"thinkingTokens":5}}`
	if err := os.WriteFile(settingsFile, []byte(settingsJSON), 0o644); err != nil {
		t.Fatal(err)
	}

	source := session.Session{
		Meta: session.SessionMeta{
			ID:               sourceID,
			Title:            "Original Title",
			WorkingDirectory: "/tmp/project",
		},
		FilePath: sourceFile,
	}
	handoff := LLMHandoff{
		Objective: "compress session",
		RecentTranscript: []RecentTurn{
			{Role: "user", Text: "先看压缩输出放到了哪里"},
			{Role: "assistant", Text: "它被写进了新 session 的首条 user message"},
		},
		TaskState: HandoffTaskState{InProgress: []string{"write code"}},
		CurrentState: HandoffCurrentState{
			Done:      "tests red",
			NextSteps: []string{"implement code"},
		},
		ResumeInstruction: "continue",
	}

	newID, err := CreateCompressedSession(source, handoff)
	if err != nil {
		t.Fatalf("CreateCompressedSession() error = %v", err)
	}

	newJSONL, err := os.ReadFile(filepath.Join(dir, newID+".jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(newJSONL)), "\n")
	if len(lines) != 4 {
		t.Fatalf("jsonl line count = %d, want 4", len(lines))
	}
	if !strings.Contains(lines[0], `"title":"Original Title [compressed]"`) {
		t.Fatalf("first line = %s", lines[0])
	}
	if !strings.Contains(lines[1], `"role":"assistant"`) || !strings.Contains(lines[1], "<compressed_session_handoff") {
		t.Fatalf("handoff line = %s", lines[1])
	}
	if !strings.Contains(lines[2], `"role":"user"`) || !strings.Contains(lines[2], "先看压缩输出放到了哪里") {
		t.Fatalf("recent user line = %s", lines[2])
	}
	if !strings.Contains(lines[3], `"role":"assistant"`) || !strings.Contains(lines[3], "它被写进了新 session 的首条 user message") {
		t.Fatalf("recent assistant line = %s", lines[3])
	}

	newSettings, err := os.ReadFile(filepath.Join(dir, newID+".settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(newSettings), `"model":"custom:GPT-5.4-1"`) {
		t.Fatalf("settings = %s", string(newSettings))
	}
	if !strings.Contains(string(newSettings), `"assistantActiveTimeMs":0`) {
		t.Fatalf("settings = %s", string(newSettings))
	}
	if !strings.Contains(string(newSettings), `"thinkingTokens":0`) {
		t.Fatalf("settings = %s", string(newSettings))
	}
}

func TestRenderHandoffMessageLeavesRecentTranscriptForTailMessages(t *testing.T) {
	source := session.Session{
		Meta: session.SessionMeta{
			ID:               "source-session",
			WorkingDirectory: "/tmp/project",
		},
		Settings: session.Settings{Model: "custom:GPT-5.4-1"},
	}

	handoff := LLMHandoff{
		Objective: "compress session",
		RecentTranscript: []RecentTurn{
			{Role: "user", Text: "先看压缩输出放到了哪里"},
			{Role: "assistant", Text: "它被写进了新 session 的首条 user message"},
		},
		CurrentState: HandoffCurrentState{
			NextSteps: []string{"implement code"},
		},
	}

	got := RenderHandoffMessage(source, handoff)
	if strings.Contains(got, "## Recent Transcript") {
		t.Fatalf("handoff should not inline recent transcript when tail messages are preserved:\n%s", got)
	}
	if !strings.Contains(got, "后续消息保留了最近的原始对话") {
		t.Fatalf("handoff missing preserved tail hint:\n%s", got)
	}
}

func TestBuildRecentTranscriptKeepsLatestTurns(t *testing.T) {
	turns := []RecentTurn{
		{Role: "user", Text: "turn-1"},
		{Role: "assistant", Text: "turn-2"},
		{Role: "user", Text: "turn-3"},
		{Role: "assistant", Text: "turn-4"},
		{Role: "user", Text: "turn-5"},
		{Role: "assistant", Text: "turn-6"},
		{Role: "user", Text: "turn-7"},
	}

	got := buildRecentTranscript(turns)
	if len(got) != len(turns) {
		t.Fatalf("len(buildRecentTranscript()) = %d, want %d (all turns fit within budget)", len(got), len(turns))
	}
	if got[0].Text != "turn-1" || got[len(got)-1].Text != "turn-7" {
		t.Fatalf("buildRecentTranscript() = %#v", got)
	}
}
