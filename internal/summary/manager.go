package summary

import (
	"context"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/zhenninglang/mantis/internal/config"
	"github.com/zhenninglang/mantis/internal/session"
)

const numWorkers = 5

type Progress struct {
	Done    int
	Total   int
	Current string // session title being processed
	Summary *Summary
	Index   int // session index
	Err     error
}

// GenerateMissing processes sessions that lack summaries using parallel workers.
func GenerateMissing(ctx context.Context, cfg config.LLMConfig, sessions []session.Session) (<-chan Progress, int) {
	ch := make(chan Progress, numWorkers)

	var pending []int
	for i := range sessions {
		if !HasSummary(sessions[i].FilePath) {
			pending = append(pending, i)
		}
	}

	total := len(pending)
	if total == 0 {
		close(ch)
		return ch, 0
	}

	var done atomic.Int32
	jobs := make(chan int, numWorkers)

	go func() {
		defer close(ch)

		var wg sync.WaitGroup
		for range numWorkers {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for idx := range jobs {
					if ctx.Err() != nil {
						return
					}
					s := &sessions[idx]
					msgs := extractUserMessages(s)
					d := int(done.Add(1))

					p := Progress{
						Done:    d,
						Total:   total,
						Current: s.Meta.Title,
						Index:   idx,
					}

					if len(msgs) == 0 {
						empty := &Summary{GeneratedAt: time.Now(), Model: cfg.Model}
						SaveSummary(s.FilePath, empty)
						p.Summary = empty
						ch <- p
						continue
					}

					sum, err := Generate(ctx, cfg, msgs)
					if err != nil {
						p.Err = err
						ch <- p
						continue
					}

					SaveSummary(s.FilePath, sum)
					p.Summary = sum
					ch <- p
				}
			}()
		}

		for _, idx := range pending {
			if ctx.Err() != nil {
				break
			}
			jobs <- idx
		}
		close(jobs)
		wg.Wait()
	}()

	return ch, total
}

// noisePatterns are substrings that indicate non-meaningful user messages.
var noisePatterns = []string{
	"cancel",
	"cancelled",
	"canceled",
	"已取消",
	"已中断",
	"Request interrupted",
	"# Task Tool Invocation",
	"<system-reminder>",
}

func isNoise(text string) bool {
	t := strings.TrimSpace(text)
	if len(t) < 5 {
		return true
	}
	lower := strings.ToLower(t)
	for _, p := range noisePatterns {
		if strings.Contains(lower, strings.ToLower(p)) {
			return true
		}
	}
	return false
}

// extractUserMessages selects user messages strategically:
// first 3 + last 3 + up to 4 sampled from middle, max 10 total.
func extractUserMessages(s *session.Session) []string {
	var all []string
	for _, msg := range s.Messages {
		if msg.Role == "user" {
			text := extractMsgText(msg.Content)
			if text != "" && !isNoise(text) {
				all = append(all, text)
			}
		}
	}

	n := len(all)
	if n <= 10 {
		return all
	}

	selected := make([]string, 0, 10)
	selected = append(selected, all[:3]...)

	middle := all[3 : n-3]
	step := len(middle) / 5
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
						if strings.HasPrefix(strings.TrimSpace(t), "<") {
							continue
						}
						return t
					}
				}
			}
		}
	}
	return ""
}
