package compress

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestParseLLMHandoffRejectsNonJSON(t *testing.T) {
	_, err := parseLLMHandoff("not json")
	if err == nil || !strings.Contains(err.Error(), "parse handoff json") {
		t.Fatalf("parseLLMHandoff() error = %v, want parse handoff json error", err)
	}
}

func TestParseLLMHandoffAcceptsMarkdownFences(t *testing.T) {
	content := "```json\n{\n  \"compressed_title\": \"ignored\",\n  \"objective\": \"compress session\",\n  \"constraints\": [\"keep paths verbatim\"],\n  \"key_decisions\": [],\n  \"task_state\": {\"completed\": [], \"in_progress\": [\"write code\"], \"pending\": []},\n  \"artifact_focus\": {\"must_keep_files\": [], \"other_touched_files\": [], \"key_commands\": [], \"unresolved_errors\": []},\n  \"current_state\": {\"done\": \"tests red\", \"open_questions\": [], \"next_steps\": [\"implement code\"]},\n  \"resume_instruction\": \"continue\"\n}\n```"

	got, err := parseLLMHandoff(content)
	if err != nil {
		t.Fatalf("parseLLMHandoff() error = %v", err)
	}
	if got.Objective != "compress session" {
		t.Fatalf("Objective = %q", got.Objective)
	}
	if len(got.CurrentState.NextSteps) != 1 || got.CurrentState.NextSteps[0] != "implement code" {
		t.Fatalf("NextSteps = %#v", got.CurrentState.NextSteps)
	}
}

func TestExtractResponsesOutputTextErrorsOnEmptyOutput(t *testing.T) {
	payload := []byte(`{"output":[]}`)
	_, err := extractResponsesOutputText(payload)
	if err == nil || !strings.Contains(err.Error(), "empty output") {
		t.Fatalf("extractResponsesOutputText() error = %v, want empty output error", err)
	}
}

func TestExtractResponsesOutputTextReadsOutputTextField(t *testing.T) {
	payload := []byte(`{"output_text":"{\"objective\":\"ok\"}"}`)
	text, err := extractResponsesOutputText(payload)
	if err != nil {
		t.Fatalf("extractResponsesOutputText() error = %v", err)
	}
	if text != `{"objective":"ok"}` {
		t.Fatalf("extractResponsesOutputText() = %q", text)
	}
}

func TestBuildResponsesRequestUsesInstructionsAndInputArray(t *testing.T) {
	payload, err := buildResponsesRequest(nil, "gpt-5.4", CompressionInput{Anchors: CompressionAnchors{InitialUserGoal: "实现压缩"}})
	if err != nil {
		t.Fatalf("buildResponsesRequest() error = %v", err)
	}

	var body map[string]any
	if err := json.Unmarshal(payload, &body); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	if body["instructions"] == "" {
		t.Fatalf("instructions missing in payload: %s", string(payload))
	}
	input, ok := body["input"].([]any)
	if !ok || len(input) != 1 {
		t.Fatalf("input = %#v", body["input"])
	}
}

func TestGenerateHandoffStreamsChatCompletions(t *testing.T) {
	handoffJSON := `{"compressed_title":"ignored","objective":"compress session","constraints":[],"active_skills":[],"compacted_history":[],"key_decisions":[],"task_state":{"completed":[],"in_progress":["write code"],"pending":[]},"artifact_focus":{"must_keep_files":[],"other_touched_files":[],"key_commands":[],"unresolved_errors":[]},"current_state":{"done":"tests red","open_questions":[],"next_steps":["implement code"]},"resume_instruction":"continue"}`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Fatalf("path = %s, want /chat/completions", r.URL.Path)
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
		escaped := strings.ReplaceAll(handoffJSON, `"`, `\"`)
		fmt.Fprintf(w, "data: {\"choices\":[{\"delta\":{\"content\":\"%s\"}}]}\n\n", escaped)
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer server.Close()

	got, err := GenerateHandoff(
		LLMAuth{BaseURL: server.URL, APIKey: "token", Provider: "openai"},
		"gpt-5.4",
		CompressionInput{Anchors: CompressionAnchors{InitialUserGoal: "实现压缩"}},
	)
	if err != nil {
		t.Fatalf("GenerateHandoff() error = %v", err)
	}
	if got.Objective != "compress session" {
		t.Fatalf("Objective = %q", got.Objective)
	}
}
