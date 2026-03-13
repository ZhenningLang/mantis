package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/mattn/go-runewidth"
)

func extractText(content any) string {
	switch v := content.(type) {
	case string:
		return singleLine(v)
	case []any:
		for _, item := range v {
			if m, ok := item.(map[string]any); ok {
				if m["type"] == "text" {
					if t, ok := m["text"].(string); ok {
						return singleLine(t)
					}
				}
			}
		}
	}
	return ""
}

func singleLine(s string) string {
	return strings.TrimSpace(strings.ReplaceAll(s, "\n", " "))
}

var timeAgoThresholds = []struct {
	limit  time.Duration
	format func(time.Duration) string
}{
	{time.Minute, func(time.Duration) string { return "just now" }},
	{time.Hour, func(d time.Duration) string { return fmt.Sprintf("%dm ago", int(d.Minutes())) }},
	{24 * time.Hour, func(d time.Duration) string { return fmt.Sprintf("%dh ago", int(d.Hours())) }},
	{30 * 24 * time.Hour, func(d time.Duration) string { return fmt.Sprintf("%dd ago", int(d.Hours()/24)) }},
}

func timeAgo(t time.Time) string {
	d := time.Since(t)
	for _, th := range timeAgoThresholds {
		if d < th.limit {
			return th.format(d)
		}
	}
	return t.Format("Jan 02")
}

func formatDuration(d time.Duration) string {
	secs := int(d.Seconds())
	if secs < 60 {
		return fmt.Sprintf("%ds", secs)
	}
	mins := secs / 60
	if mins < 60 {
		return fmt.Sprintf("%dm %ds", mins, secs%60)
	}
	hours := mins / 60
	return fmt.Sprintf("%dh %dm", hours, mins%60)
}

func formatTokens(n int) string {
	if n < 1000 {
		return fmt.Sprintf("%d", n)
	}
	if n < 1_000_000 {
		return fmt.Sprintf("%.1fK", float64(n)/1000)
	}
	return fmt.Sprintf("%.1fM", float64(n)/1_000_000)
}

// truncateDisplay truncates s to fit within maxWidth display columns.
func truncateDisplay(s string, maxWidth int) string {
	w := runewidth.StringWidth(s)
	if w <= maxWidth {
		return s
	}
	ellipsis := "..."
	target := maxWidth - runewidth.StringWidth(ellipsis)
	if target <= 0 {
		return ellipsis[:maxWidth]
	}
	cur := 0
	for i, r := range s {
		rw := runewidth.RuneWidth(r)
		if cur+rw > target {
			return s[:i] + ellipsis
		}
		cur += rw
	}
	return s
}
