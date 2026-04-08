package compress

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/zhenninglang/mantis/internal/llmstream"
)

const compressSystemPrompt = `You are a session compression engine for a coding assistant.

You will receive a deterministic CompressionInput JSON.
Return ONLY valid JSON matching this exact schema:
{
  "compressed_title": "string",
  "objective": "string",
  "constraints": ["string"],
  "active_skills": ["string"],
  "compacted_history": [
    {
      "goal": "string",
      "outcome": "string",
      "key_files": ["string"],
      "key_commands": ["string"],
      "open_issues": ["string"],
      "skills": ["string"]
    }
  ],
  "key_decisions": [
    {"decision": "string", "status": "active|superseded", "why": "string|null"}
  ],
  "task_state": {
    "completed": ["string"],
    "in_progress": ["string"],
    "pending": ["string"]
  },
  "artifact_focus": {
    "must_keep_files": [
      {"path": "string", "symbols": ["string"], "reason": "string"}
    ],
    "other_touched_files": ["string"],
    "key_commands": [
      {"cmd": "string", "outcome": "ok|error", "evidence": "string"}
    ],
    "unresolved_errors": [
      {"error": "string", "next_action": "string"}
    ]
  },
  "current_state": {
    "done": "string",
    "open_questions": ["string"],
    "next_steps": ["string"]
  },
  "resume_instruction": "string"
}

Rules:
- Be concise. Each value should be ONE brief sentence max. No filler text.
- resume_instruction must be one actionable sentence, not a detailed plan.
- Do not rewrite file paths, symbols, commands, or error strings.
- Preserve active skill names exactly; if any are active, remind the next session to re-invoke them with the Skill tool.
- If a field is unknown, use null, [] or an empty string.
- Do not invent facts that are not present in the input.
- Output JSON only.`

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatCompletionResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}

type responsesAPIResponse struct {
	Error      *responsesError `json:"error,omitempty"`
	OutputText string          `json:"output_text,omitempty"`
	Output     []responsesItem `json:"output,omitempty"`
}

type responsesError struct {
	Message string `json:"message"`
	Type    string `json:"type"`
}

type responsesItem struct {
	Type    string                 `json:"type"`
	Content []responsesContentItem `json:"content,omitempty"`
}

type responsesContentItem struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

func GenerateHandoff(auth LLMAuth, model string, input CompressionInput) (*LLMHandoff, error) {
	return generateChatCompletionsHandoff(auth, model, input)
}

func generateOpenAIResponsesHandoff(auth LLMAuth, model string, input CompressionInput) (*LLMHandoff, error) {
	payload, err := buildResponsesRequest(auth.ExtraArgs, model, input)
	if err != nil {
		return nil, err
	}

	content, err := llmstream.Responses(context.Background(), llmstream.Auth{
		BaseURL: auth.BaseURL,
		APIKey:  auth.APIKey,
	}, payload)
	if err != nil {
		return nil, err
	}
	return parseLLMHandoff(content)
}

func generateChatCompletionsHandoff(auth LLMAuth, model string, input CompressionInput) (*LLMHandoff, error) {
	payload, err := buildChatRequest(auth.ExtraArgs, model, input)
	if err != nil {
		return nil, err
	}

	content, err := llmstream.ChatCompletions(context.Background(), llmstream.Auth{
		BaseURL: auth.BaseURL,
		APIKey:  auth.APIKey,
	}, payload)
	if err != nil {
		return nil, err
	}
	if content == "" {
		return nil, fmt.Errorf("llm response returned empty content")
	}
	return parseLLMHandoff(content)
}

func buildChatRequest(extraArgs map[string]any, model string, input CompressionInput) ([]byte, error) {
	inputJSON, err := json.MarshalIndent(input, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal compression input: %w", err)
	}

	payload := map[string]any{
		"model": model,
		"messages": []chatMessage{
			{Role: "system", Content: compressSystemPrompt},
			{Role: "user", Content: string(inputJSON)},
		},
	}
	for key, value := range extraArgs {
		payload[key] = value
	}
	return json.Marshal(payload)
}

func buildResponsesRequest(extraArgs map[string]any, model string, input CompressionInput) ([]byte, error) {
	inputJSON, err := json.MarshalIndent(input, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal compression input: %w", err)
	}

	payload := map[string]any{
		"model":        model,
		"instructions": compressSystemPrompt,
		"input": []map[string]any{{
			"role": "user",
			"content": []map[string]any{{
				"type": "input_text",
				"text": string(inputJSON),
			}},
		}},
		"text": map[string]any{
			"format": map[string]any{"type": "json_object"},
		},
	}
	for key, value := range extraArgs {
		payload[key] = value
	}
	return json.Marshal(payload)
}

func extractResponsesOutputText(respBody []byte) (string, error) {
	var resp responsesAPIResponse
	if err := json.Unmarshal(respBody, &resp); err != nil {
		return "", fmt.Errorf("parse responses api payload: %w", err)
	}
	if resp.Error != nil && strings.TrimSpace(resp.Error.Message) != "" {
		return "", fmt.Errorf("responses api error: %s", resp.Error.Message)
	}
	if text := strings.TrimSpace(resp.OutputText); text != "" {
		return text, nil
	}
	for _, item := range resp.Output {
		for _, content := range item.Content {
			if text := strings.TrimSpace(content.Text); text != "" {
				return text, nil
			}
		}
	}
	return "", fmt.Errorf("responses api returned empty output")
}

func parseLLMHandoff(content string) (*LLMHandoff, error) {
	content = strings.TrimSpace(content)
	content = strings.TrimPrefix(content, "```json")
	content = strings.TrimPrefix(content, "```")
	content = strings.TrimSuffix(content, "```")
	content = strings.TrimSpace(content)

	var handoff LLMHandoff
	if err := json.Unmarshal([]byte(content), &handoff); err != nil {
		return nil, fmt.Errorf("parse handoff json: %w", err)
	}
	return &handoff, nil
}

func truncate(body []byte, n int) string {
	text := string(body)
	if len(text) <= n {
		return text
	}
	return text[:n] + "..."
}
