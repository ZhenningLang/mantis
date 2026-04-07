package inspect

import (
	"encoding/json"
	"regexp"
	"strings"

	"github.com/zhenninglang/mantis/internal/session"
)

var tagPatterns = []struct {
	tag     string
	pattern *regexp.Regexp
}{
	{"system-reminder", regexp.MustCompile(`(?s)<system-reminder>.*?</system-reminder>`)},
	{"EXTREMELY_IMPORTANT", regexp.MustCompile(`(?s)<EXTREMELY_IMPORTANT>.*?</EXTREMELY_IMPORTANT>`)},
	{"coding_guidelines", regexp.MustCompile(`(?s)<coding_guidelines>.*?</coding_guidelines>`)},
	{"available_skills", regexp.MustCompile(`(?s)<available_skills>.*?</available_skills>`)},
}

// Analyze performs static analysis on a single session.
func Analyze(s session.Session) SessionAnalysis {
	events, _ := session.ParseAllEvents(s.FilePath)

	a := SessionAnalysis{
		Session:      s,
		Settings:     s.Settings,
		Events:       events,
		MessageCount: countMessages(events),
		IsSubagent:   len(events) > 0 && events[0].CallingSessionID != "",
	}

	a.Distribution = analyzeDistribution(events)
	a.ToolStats = analyzeTools(events)
	a.SystemPrompt = analyzeSystemPrompt(events)
	a.CacheAnalysis = analyzeCacheFromSettings(s.Settings)

	return a
}

func countMessages(events []session.RawEvent) int {
	n := 0
	for _, ev := range events {
		if ev.Type == "message" {
			n++
		}
	}
	return n
}

func analyzeDistribution(events []session.RawEvent) ContentDistribution {
	var d ContentDistribution
	isFirstUser := true

	for _, ev := range events {
		if ev.Type != "message" || ev.Message == nil {
			continue
		}
		for _, item := range ev.Message.Content {
			size := contentSize(item)
			switch ev.Message.Role {
			case "user":
				switch item.Type {
				case "text":
					if isFirstUser {
						d.SystemPrompt += size
						isFirstUser = false
					} else if strings.Contains(item.Text, "<system-reminder>") {
						d.SystemReminder += size
					} else {
						d.UserText += size
					}
				case "tool_result":
					d.ToolResult += size
				}
			case "assistant":
				switch item.Type {
				case "text":
					d.AssistantText += size
				case "thinking":
					d.AssistantThink += size
				case "tool_use":
					d.AssistantToolUse += size
				}
			}
		}
	}

	d.Total = d.SystemPrompt + d.SystemReminder + d.UserText + d.ToolResult +
		d.AssistantText + d.AssistantThink + d.AssistantToolUse
	return d
}

func analyzeTools(events []session.RawEvent) []ToolStat {
	toolCalls := map[string]string{} // tool_use_id -> tool_name
	stats := map[string]*ToolStat{}

	for _, ev := range events {
		if ev.Type != "message" || ev.Message == nil {
			continue
		}
		for _, item := range ev.Message.Content {
			if item.Type == "tool_use" && item.Name != "" {
				toolCalls[item.ID] = item.Name
				if stats[item.Name] == nil {
					stats[item.Name] = &ToolStat{Name: item.Name}
				}
				stats[item.Name].CallCount++
			}
			if item.Type == "tool_result" && item.ToolUseID != "" {
				name := toolCalls[item.ToolUseID]
				if name == "" {
					name = "unknown"
				}
				if stats[name] == nil {
					stats[name] = &ToolStat{Name: name}
				}
				size := len(item.Content)
				stats[name].ResultChars += size
				if size > stats[name].MaxResult {
					stats[name].MaxResult = size
				}
			}
		}
	}

	var result []ToolStat
	for _, s := range stats {
		result = append(result, *s)
	}
	// sort by result chars descending
	for i := 0; i < len(result); i++ {
		for j := i + 1; j < len(result); j++ {
			if result[j].ResultChars > result[i].ResultChars {
				result[i], result[j] = result[j], result[i]
			}
		}
	}
	return result
}

func analyzeSystemPrompt(events []session.RawEvent) SystemPromptAnalysis {
	var sp SystemPromptAnalysis
	segMap := map[string]*PromptSegment{}

	for _, ev := range events {
		if ev.Type != "message" || ev.Message == nil || ev.Message.Role != "user" {
			continue
		}
		for _, item := range ev.Message.Content {
			if item.Type != "text" {
				continue
			}
			text := item.Text

			// capture first user text as full system prompt
			if sp.FullText == "" {
				sp.FullText = text
				sp.TotalChars = len(text)
			}

			// count system-reminder injections
			reminderRe := tagPatterns[0].pattern
			for _, m := range reminderRe.FindAllString(text, -1) {
				sp.ReminderCount++
				sp.ReminderChars += len(m)
			}

			// extract tagged segments
			for _, tp := range tagPatterns {
				for _, m := range tp.pattern.FindAllString(text, -1) {
					if segMap[tp.tag] == nil {
						segMap[tp.tag] = &PromptSegment{Tag: tp.tag}
					}
					segMap[tp.tag].Count++
					segMap[tp.tag].Chars += len(m)
				}
			}
		}
	}

	for _, seg := range segMap {
		sp.Segments = append(sp.Segments, *seg)
	}
	return sp
}

func analyzeCacheFromSettings(s session.Settings) CacheAnalysis {
	u := s.TokenUsage
	ca := CacheAnalysis{
		InputTokens:         u.InputTokens,
		OutputTokens:        u.OutputTokens,
		CacheReadTokens:     u.CacheReadTokens,
		CacheCreationTokens: u.CacheCreationTokens,
		ThinkingTokens:      u.ThinkingTokens,
	}
	total := u.InputTokens + u.CacheReadTokens
	if total > 0 {
		ca.HitRate = float64(u.CacheReadTokens) / float64(total) * 100
	}
	return ca
}

func contentSize(item session.RawContentItem) int {
	switch item.Type {
	case "text":
		return len(item.Text)
	case "tool_result":
		return len(item.Content)
	case "thinking":
		return len(item.Thinking) + len(item.Signature)
	case "tool_use":
		if item.Input != nil {
			b, _ := json.Marshal(item.Input)
			return len(b)
		}
		return 0
	default:
		return 0
	}
}
