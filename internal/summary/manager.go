package summary

import (
	"context"

	"github.com/zhenninglang/mantis/internal/config"
	"github.com/zhenninglang/mantis/internal/session"
)

type Progress struct {
	Done    int
	Total   int
	Current string // session title being processed
	Summary *Summary
	Index   int // session index
	Err     error
}

// GenerateMissing processes sessions that lack summaries.
// It sends progress updates on the returned channel and closes it when done.
func GenerateMissing(ctx context.Context, cfg config.LLMConfig, sessions []session.Session) <-chan Progress {
	ch := make(chan Progress, 1)

	var pending []int
	for i := range sessions {
		if !HasSummary(sessions[i].FilePath) {
			pending = append(pending, i)
		}
	}

	total := len(pending)
	if total == 0 {
		close(ch)
		return ch
	}

	go func() {
		defer close(ch)
		for done, idx := range pending {
			if ctx.Err() != nil {
				return
			}

			s := &sessions[idx]
			msgs := extractUserMessages(s)

			p := Progress{
				Done:    done,
				Total:   total,
				Current: s.Meta.Title,
				Index:   idx,
			}

			if len(msgs) == 0 {
				p.Done = done + 1
				ch <- p
				continue
			}

			sum, err := Generate(ctx, cfg, msgs)
			if err != nil {
				p.Err = err
				p.Done = done + 1
				ch <- p
				continue
			}

			SaveSummary(s.FilePath, sum)
			p.Summary = sum
			p.Done = done + 1
			ch <- p
		}
	}()

	return ch
}

// extractUserMessages selects user messages strategically:
// first 3 + last 3 + up to 4 sampled from middle, max 10 total.
func extractUserMessages(s *session.Session) []string {
	var all []string
	for _, msg := range s.Messages {
		if msg.Role == "user" {
			text := extractMsgText(msg.Content)
			if text != "" {
				all = append(all, text)
			}
		}
	}

	n := len(all)
	if n <= 10 {
		return all
	}

	// first 3 + last 3
	selected := make([]string, 0, 10)
	selected = append(selected, all[:3]...)

	// sample 4 from middle
	middle := all[3 : n-3]
	step := len(middle) / 5 // 4 samples, 5 gaps
	if step < 1 {
		step = 1
	}
	count := 0
	for i := step; i < len(middle) && count < 4; i += step {
		selected = append(selected, middle[i])
		count++
	}

	selected = append(selected, all[n-3:]...)
	return selected
}

func extractMsgText(content any) string {
	switch v := content.(type) {
	case string:
		return v
	case []any:
		for _, item := range v {
			if m, ok := item.(map[string]any); ok {
				if m["type"] == "text" {
					if t, ok := m["text"].(string); ok {
						return t
					}
				}
			}
		}
	}
	return ""
}
