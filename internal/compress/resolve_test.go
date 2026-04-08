package compress

import (
	"strings"
	"testing"

	"github.com/zhenninglang/mantis/internal/session"
)

func TestResolveSourceByPrefix(t *testing.T) {
	sessions := []session.Session{
		{Meta: session.SessionMeta{ID: "abc123"}},
		{Meta: session.SessionMeta{ID: "def456"}},
	}

	got, err := ResolveSourceByPrefix(sessions, "abc")
	if err != nil {
		t.Fatalf("ResolveSourceByPrefix() error = %v", err)
	}
	if got.Meta.ID != "abc123" {
		t.Fatalf("ResolveSourceByPrefix() id = %q, want %q", got.Meta.ID, "abc123")
	}
}

func TestResolveSourceByPrefixErrors(t *testing.T) {
	sessions := []session.Session{
		{Meta: session.SessionMeta{ID: "abc123"}},
		{Meta: session.SessionMeta{ID: "abc999"}},
	}

	tests := []struct {
		name   string
		prefix string
		want   string
	}{
		{name: "no match", prefix: "zzz", want: "no session matches prefix"},
		{name: "multiple matches", prefix: "abc", want: "multiple sessions match prefix"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ResolveSourceByPrefix(sessions, tt.prefix)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("ResolveSourceByPrefix() error = %v, want substring %q", err, tt.want)
			}
		})
	}
}

func TestResolveCompressionModel(t *testing.T) {
	settings := DroidSettings{
		CustomModels: []CustomModel{
			{ID: "custom:GPT-5.4-1", Model: "gpt-5.4"},
		},
	}

	got, err := ResolveCompressionModel("custom:GPT-5.4-1", settings)
	if err != nil {
		t.Fatalf("ResolveCompressionModel() error = %v", err)
	}
	if got != "gpt-5.4" {
		t.Fatalf("ResolveCompressionModel() = %q, want %q", got, "gpt-5.4")
	}

	got, err = ResolveCompressionModel("gpt-5.5", settings)
	if err != nil {
		t.Fatalf("ResolveCompressionModel() error = %v", err)
	}
	if got != "gpt-5.5" {
		t.Fatalf("ResolveCompressionModel() = %q, want %q", got, "gpt-5.5")
	}
}

func TestResolveCompressionAuth(t *testing.T) {
	settings := DroidSettings{
		CustomModels: []CustomModel{
			{ID: "custom:Claude-Opus-4.6-0", BaseURL: "http://localhost:8317", APIKey: "token", Provider: "anthropic", ExtraArgs: map[string]any{"foo": "bar"}},
			{ID: "custom:GPT-5.4-1", BaseURL: "http://localhost:8317/v1", APIKey: "token2", Provider: "openai"},
		},
		SessionDefaultSettings: DroidSessionDefaults{Model: "custom:GPT-5.4-1"},
	}

	auth, err := ResolveCompressionAuth(settings)
	if err != nil {
		t.Fatalf("ResolveCompressionAuth() error = %v", err)
	}
	if auth.BaseURL != "http://localhost:8317/v1" || auth.APIKey != "token2" || auth.Provider != "openai" {
		t.Fatalf("ResolveCompressionAuth() = %#v", auth)
	}
	if auth.ExtraArgs != nil {
		t.Fatalf("ResolveCompressionAuth() extra args = %#v, want nil", auth.ExtraArgs)
	}
}

func TestResolveCompressionAuthErrorsOnMissingDefaultCustomModel(t *testing.T) {
	settings := DroidSettings{
		SessionDefaultSettings: DroidSessionDefaults{Model: "custom:missing"},
	}

	_, err := ResolveCompressionAuth(settings)
	if err == nil || !strings.Contains(err.Error(), "default custom model") {
		t.Fatalf("ResolveCompressionAuth() error = %v, want default custom model error", err)
	}
}
