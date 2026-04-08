package inspect

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/zhenninglang/mantis/internal/config"
	"github.com/zhenninglang/mantis/internal/session"
)

func TestRunAgentAnalysisStreamsChatCompletionsByDefault(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Fatalf("path = %s", r.URL.Path)
		}

		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		if body["stream"] != true {
			http.Error(w, `{"error":{"message":"stream required"}}`, http.StatusBadRequest)
			return
		}

		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"分析完成\"}}]}\n\n"))
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{},\"finish_reason\":\"stop\"}]}\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer server.Close()

	got, err := RunAgentAnalysis(context.Background(), config.LLMConfig{
		BaseURL: server.URL,
		APIKey:  "token",
		Model:   "gpt-5.4",
	}, []SessionAnalysis{{
		Session: session.Session{
			Meta: session.SessionMeta{ID: "12345678-abcd"},
		},
	}})
	if err != nil {
		t.Fatalf("RunAgentAnalysis() error = %v", err)
	}
	if got != "分析完成" {
		t.Fatalf("RunAgentAnalysis() = %q", got)
	}
}
