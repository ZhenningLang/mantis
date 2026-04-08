package summary

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/zhenninglang/mantis/internal/config"
)

func TestGenerateStreamsChatCompletionsByDefault(t *testing.T) {
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
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"{\\\"title\\\":\\\"stream ok\\\",\\\"topics\\\":[{\\\"summary\\\":\\\"works\\\",\\\"keywords\\\":[\\\"stream\\\"]}]}\"}}]}\n\n"))
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{},\"finish_reason\":\"stop\"}]}\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer server.Close()

	got, err := Generate(context.Background(), config.LLMConfig{
		BaseURL: server.URL,
		APIKey:  "token",
		Model:   "gpt-5.4",
	}, []string{"请总结这个 session"})
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if got.Title != "stream ok" {
		t.Fatalf("Title = %q", got.Title)
	}
}
