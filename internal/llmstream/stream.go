package llmstream

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type Auth struct {
	BaseURL string
	APIKey  string
}

func ChatCompletions(ctx context.Context, auth Auth, payload []byte) (string, error) {
	streamPayload, err := withStreamFlag(payload)
	if err != nil {
		return "", err
	}

	var text strings.Builder
	err = doStreamRequest(ctx, auth, "/chat/completions", streamPayload, func(_ string, data string) error {
		if data == "[DONE]" {
			return nil
		}

		var chunk chatStreamChunk
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			return fmt.Errorf("parse chat stream chunk: %w", err)
		}
		if chunk.Error != nil && strings.TrimSpace(chunk.Error.Message) != "" {
			return fmt.Errorf("llm stream error: %s", chunk.Error.Message)
		}
		for _, choice := range chunk.Choices {
			text.WriteString(extractText(choice.Delta.Content))
		}
		return nil
	})
	if err != nil {
		return "", err
	}

	content := strings.TrimSpace(text.String())
	if content == "" {
		return "", fmt.Errorf("llm stream returned empty content")
	}
	return content, nil
}

func Responses(ctx context.Context, auth Auth, payload []byte) (string, error) {
	streamPayload, err := withStreamFlag(payload)
	if err != nil {
		return "", err
	}

	var text strings.Builder
	var finalText string
	err = doStreamRequest(ctx, auth, "/responses", streamPayload, func(event, data string) error {
		if data == "[DONE]" {
			return nil
		}

		var chunk responsesStreamChunk
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			return fmt.Errorf("parse responses stream chunk: %w", err)
		}
		if chunk.Error != nil && strings.TrimSpace(chunk.Error.Message) != "" {
			return fmt.Errorf("responses api error: %s", chunk.Error.Message)
		}
		if chunk.Response != nil && chunk.Response.Error != nil && strings.TrimSpace(chunk.Response.Error.Message) != "" {
			return fmt.Errorf("responses api error: %s", chunk.Response.Error.Message)
		}

		eventType := strings.TrimSpace(event)
		if eventType == "" {
			eventType = strings.TrimSpace(chunk.Type)
		}

		switch eventType {
		case "response.output_text.delta":
			text.WriteString(chunk.Delta)
		case "response.output_text.done":
			if text.Len() == 0 {
				finalText = chunk.Text
			}
		}
		return nil
	})
	if err != nil {
		return "", err
	}

	content := strings.TrimSpace(text.String())
	if content == "" {
		content = strings.TrimSpace(finalText)
	}
	if content == "" {
		return "", fmt.Errorf("responses api returned empty output")
	}
	return content, nil
}

type chatStreamChunk struct {
	Error   *streamError `json:"error,omitempty"`
	Choices []struct {
		Delta struct {
			Content any `json:"content"`
		} `json:"delta"`
	} `json:"choices"`
}

type responsesStreamChunk struct {
	Type     string        `json:"type"`
	Delta    string        `json:"delta,omitempty"`
	Text     string        `json:"text,omitempty"`
	Error    *streamError  `json:"error,omitempty"`
	Response *responseMeta `json:"response,omitempty"`
}

type responseMeta struct {
	Error *streamError `json:"error,omitempty"`
}

type streamError struct {
	Message string `json:"message"`
}

func doStreamRequest(ctx context.Context, auth Auth, endpoint string, payload []byte, consume func(event, data string) error) error {
	baseURL := strings.TrimRight(auth.BaseURL, "/")
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+endpoint, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("create llm request: %w", err)
	}
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+auth.APIKey)

	client := &http.Client{Timeout: 120 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("llm request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, readErr := io.ReadAll(resp.Body)
		if readErr != nil {
			return fmt.Errorf("read llm error response: %w", readErr)
		}
		return fmt.Errorf("llm api error %d: %s", resp.StatusCode, truncate(body, 400))
	}

	if err := readSSE(resp.Body, consume); err != nil {
		return err
	}
	return nil
}

func withStreamFlag(payload []byte) ([]byte, error) {
	var body map[string]any
	if err := json.Unmarshal(payload, &body); err != nil {
		return nil, fmt.Errorf("parse request payload: %w", err)
	}
	body["stream"] = true
	return json.Marshal(body)
}

func readSSE(r io.Reader, consume func(event, data string) error) error {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 64*1024), 2*1024*1024)

	var event string
	var dataLines []string
	dispatch := func() error {
		if len(dataLines) == 0 {
			event = ""
			return nil
		}
		data := strings.Join(dataLines, "\n")
		dataLines = nil
		defer func() {
			event = ""
		}()
		return consume(event, data)
	}

	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			if err := dispatch(); err != nil {
				return err
			}
			continue
		}
		switch {
		case strings.HasPrefix(line, "event:"):
			event = strings.TrimSpace(line[len("event:"):])
		case strings.HasPrefix(line, "data:"):
			dataLines = append(dataLines, strings.TrimSpace(line[len("data:"):]))
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("read llm stream: %w", err)
	}
	if err := dispatch(); err != nil {
		return err
	}
	return nil
}

func extractText(content any) string {
	switch v := content.(type) {
	case string:
		return v
	case []any:
		var b strings.Builder
		for _, item := range v {
			b.WriteString(extractText(item))
		}
		return b.String()
	case map[string]any:
		if text, ok := v["text"].(string); ok {
			return text
		}
		if inner, ok := v["content"]; ok {
			return extractText(inner)
		}
	}
	return ""
}

func truncate(body []byte, n int) string {
	text := string(body)
	if len(text) <= n {
		return text
	}
	return text[:n] + "..."
}
