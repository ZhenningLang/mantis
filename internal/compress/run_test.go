package compress

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/zhenninglang/mantis/internal/session"
)

func TestRunnerRunPrintsProgress(t *testing.T) {
	var logs bytes.Buffer

	r := runner{
		out: &logs,
		loadSessions: func() ([]session.Session, error) {
			return []session.Session{{
				Meta:     session.SessionMeta{ID: "abc123", Title: "Original", WorkingDirectory: "/tmp/project"},
				Settings: session.Settings{Model: "custom:GPT-5.4-1"},
				FilePath: "/tmp/project/abc123.jsonl",
			}}, nil
		},
		parseEvents: func(path string) ([]session.RawEvent, error) {
			return []session.RawEvent{{
				Type: "message",
				Message: &session.RawMessage{
					Role: "user",
					Content: []session.RawContentItem{{
						Type: "text",
						Text: "请实现 compress",
					}},
				},
			}}, nil
		},
		loadDroidSettings: func(path string) (DroidSettings, error) {
			return DroidSettings{
				CustomModels:           []CustomModel{{ID: "custom:GPT-5.4-1", Model: "gpt-5.4", BaseURL: "http://localhost:8317/v1", APIKey: "token"}},
				SessionDefaultSettings: DroidSessionDefaults{Model: "custom:GPT-5.4-1"},
			}, nil
		},
		handoffProgressInterval: time.Millisecond,
		generateHandoff: func(auth LLMAuth, model string, input CompressionInput) (*LLMHandoff, error) {
			time.Sleep(5 * time.Millisecond)
			return &LLMHandoff{Objective: "compress"}, nil
		},
		createCompressedSession: func(source session.Session, handoff LLMHandoff) (string, error) {
			return "new-session", nil
		},
	}

	got, err := r.run([]string{"abc"})
	if err != nil {
		t.Fatalf("run() error = %v", err)
	}
	if got != "new-session" {
		t.Fatalf("run() id = %q, want %q", got, "new-session")
	}

	output := logs.String()
	for _, want := range []string{
		"Resolving source session",
		"Matched source session abc123",
		"Parsing source session events",
		"Compaction window prepared",
		"Loading Droid settings",
		"Generating structured handoff",
		"Still generating structured handoff",
		"Structured handoff generated in",
		"Created compressed session new-session",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("log output missing %q:\n%s", want, output)
		}
	}
}
