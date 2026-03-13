package summary

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/zhenninglang/mantis/internal/config"
)

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatRequest struct {
	Model    string        `json:"model"`
	Messages []chatMessage `json:"messages"`
}

type chatResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}

const systemPrompt = `You are a session summarizer. Given a list of user messages from a coding chat session, generate a structured summary.

Output ONLY valid JSON in this exact format:
{
  "title": "concise title describing the main task (under 60 chars)",
  "topics": [
    {
      "summary": "one sentence describing this topic",
      "keywords": ["keyword1", "keyword2", "keyword3"]
    }
  ]
}

Rules:
- title: concise, descriptive, under 60 characters. Focus on WHAT was done, not meta-commentary
- topics: 1-5 topics covering distinct themes in the session
- keywords: 2-6 lowercase keywords per topic, include technologies, actions, concepts
- Use the same language as the user messages (Chinese if messages are in Chinese)
- Even if messages are short or seem trivial, summarize what the user actually asked or discussed
- Never generate meta-commentary like "会话已取消" or "无技术讨论". Always describe the actual content
- Output ONLY the JSON, no markdown fences, no explanation`

func Generate(ctx context.Context, cfg config.LLMConfig, userMessages []string) (*Summary, error) {
	if len(userMessages) == 0 {
		return nil, fmt.Errorf("no messages to summarize")
	}

	input := "User messages from this session:\n\n"
	for i, msg := range userMessages {
		text := msg
		if len(text) > 500 {
			text = text[:500] + "..."
		}
		input += fmt.Sprintf("[%d] %s\n\n", i+1, text)
	}

	body, err := json.Marshal(chatRequest{
		Model: cfg.Model,
		Messages: []chatMessage{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: input},
		},
	})
	if err != nil {
		return nil, err
	}

	baseURL := strings.TrimRight(cfg.BaseURL, "/")
	req, err := http.NewRequestWithContext(ctx, "POST", baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+cfg.APIKey)

	client := &http.Client{Timeout: 120 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("llm request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("llm api error %d: %s", resp.StatusCode, truncate(string(respBody), 200))
	}

	var chatResp chatResponse
	if err := json.Unmarshal(respBody, &chatResp); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}
	if len(chatResp.Choices) == 0 {
		return nil, fmt.Errorf("no choices in response")
	}

	content := chatResp.Choices[0].Message.Content
	content = strings.TrimSpace(content)
	// strip markdown fences if present
	content = strings.TrimPrefix(content, "```json")
	content = strings.TrimPrefix(content, "```")
	content = strings.TrimSuffix(content, "```")
	content = strings.TrimSpace(content)

	var s Summary
	if err := json.Unmarshal([]byte(content), &s); err != nil {
		return nil, fmt.Errorf("parse summary json: %w (content: %s)", err, truncate(content, 200))
	}
	if s.Title == "" {
		return nil, fmt.Errorf("empty title in response (content: %s)", truncate(content, 200))
	}

	s.GeneratedAt = time.Now()
	s.Model = cfg.Model
	return &s, nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
