package tui

import (
	"fmt"
	"strings"

	"github.com/zhenninglang/mantis/internal/session"
	"github.com/zhenninglang/mantis/internal/summary"
)

func renderPreview(s *session.Session, sum *summary.Summary, width int) string {
	if s == nil {
		return dimStyle.Render("No session selected")
	}

	var b strings.Builder

	b.WriteString(previewTitleStyle.Render(s.Meta.Title))
	b.WriteString("\n")

	// show AI title if available
	if sum != nil && sum.Title != "" {
		b.WriteString(dimStyle.Render("[AI] ") + previewValueStyle.Render(sum.Title))
		b.WriteString("\n")
	}

	info := fmt.Sprintf("%s  |  %s  |  %s",
		previewLabelStyle.Render("Project: ")+previewValueStyle.Render(s.ProjectShort()),
		previewLabelStyle.Render("Model: ")+previewValueStyle.Render(modelShort(s.Settings.Model)),
		previewLabelStyle.Render("Updated: ")+previewValueStyle.Render(timeAgo(s.ModTime)),
	)
	b.WriteString(info)
	b.WriteString("\n")

	tokens := fmt.Sprintf("%s  |  %s",
		previewLabelStyle.Render("Tokens: ")+previewValueStyle.Render(
			fmt.Sprintf("%s in / %s out",
				formatTokens(s.Settings.TokenUsage.InputTokens),
				formatTokens(s.Settings.TokenUsage.OutputTokens))),
		previewLabelStyle.Render("Active Time: ")+previewValueStyle.Render(formatDuration(s.ActiveDuration())),
	)
	b.WriteString(tokens)
	b.WriteString("\n")

	// extra token details + mode (only non-zero values)
	var extras []string
	if mode := s.Settings.AutonomyMode; mode != "" {
		extras = append(extras, previewLabelStyle.Render("Mode: ")+previewValueStyle.Render(mode))
	}
	u := s.Settings.TokenUsage
	if u.ThinkingTokens > 0 {
		extras = append(extras, previewLabelStyle.Render("Thinking: ")+previewValueStyle.Render(formatTokens(u.ThinkingTokens)))
	}
	if u.CacheReadTokens > 0 || u.CacheCreationTokens > 0 {
		extras = append(extras, previewLabelStyle.Render("Cache: ")+previewValueStyle.Render(
			fmt.Sprintf("%s read / %s created", formatTokens(u.CacheReadTokens), formatTokens(u.CacheCreationTokens))))
	}
	if len(extras) > 0 {
		b.WriteString(strings.Join(extras, "  |  "))
		b.WriteString("\n")
	}

	// show AI summary topics if available
	if sum != nil && len(sum.Topics) > 0 {
		for _, t := range sum.Topics {
			b.WriteString(previewLabelStyle.Render("● ") + previewValueStyle.Render(t.Summary))
			if len(t.Keywords) > 0 {
				b.WriteString("  " + dimStyle.Render(strings.Join(t.Keywords, ", ")))
			}
			b.WriteString("\n")
		}
	}

	sep := dimStyle.Render(strings.Repeat("─", min(width-2, 60)))
	b.WriteString(sep)
	b.WriteString("\n")

	// show first few conversation turns
	count := 0
	for _, msg := range s.Messages {
		if msg.Role != "user" && msg.Role != "assistant" {
			continue
		}
		text := extractText(msg.Content)
		if text == "" {
			continue
		}
		if strings.HasPrefix(text, "<system-reminder>") || strings.HasPrefix(text, "<EXTREMELY") {
			continue
		}
		if len(text) > 120 {
			text = text[:117] + "..."
		}

		if msg.Role == "user" {
			b.WriteString(userMsgStyle.Render("User: ") + previewValueStyle.Render(text))
		} else {
			b.WriteString(assistantMsgStyle.Render("Asst: ") + dimStyle.Render(text))
		}
		b.WriteString("\n")
		count++
		if count >= 3 {
			break
		}
	}

	// show last meaningful assistant reply
	lastReply := lastAssistantReply(s)
	if lastReply != "" {
		if count > 0 {
			b.WriteString(dimStyle.Render("  ..."))
			b.WriteString("\n")
		}
		b.WriteString(assistantMsgStyle.Render("Last: ") + dimStyle.Render(lastReply))
		b.WriteString("\n")
	}

	if count == 0 && lastReply == "" {
		b.WriteString(dimStyle.Render("(no messages)"))
	}

	return b.String()
}

func lastAssistantReply(s *session.Session) string {
	for i := len(s.Messages) - 1; i >= 0; i-- {
		msg := s.Messages[i]
		if msg.Role != "assistant" {
			continue
		}
		text := extractText(msg.Content)
		if text == "" || strings.HasPrefix(text, "<") {
			continue
		}
		if len(text) > 150 {
			text = text[:147] + "..."
		}
		return text
	}
	return ""
}

