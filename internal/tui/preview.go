package tui

import (
	"fmt"
	"strings"

	"github.com/zhenninglang/mantis/internal/session"
	"github.com/zhenninglang/mantis/internal/summary"
)

func renderPreview(s *session.Session, sum *summary.Summary, width, height int) string {
	if s == nil {
		return dimStyle.Render("No session selected")
	}

	var b strings.Builder
	used := 0

	// line 1: title (prefer AI title)
	title := s.Meta.Title
	if sum != nil && sum.Title != "" {
		title = "[AI] " + sum.Title
	}
	b.WriteString(previewTitleStyle.Render(truncateDisplay(title, width-2)))
	b.WriteString("\n")
	used++

	// line 2-3: metadata
	info := fmt.Sprintf("%s  |  %s  |  %s",
		previewLabelStyle.Render("Project: ")+previewValueStyle.Render(s.ProjectShort()),
		previewLabelStyle.Render("Model: ")+previewValueStyle.Render(modelShort(s.Settings.Model)),
		previewLabelStyle.Render("Updated: ")+previewValueStyle.Render(timeAgo(s.ModTime)),
	)
	b.WriteString(info)
	b.WriteString("\n")
	used++

	tokens := fmt.Sprintf("%s  |  %s",
		previewLabelStyle.Render("Tokens: ")+previewValueStyle.Render(
			fmt.Sprintf("%s in / %s out",
				formatTokens(s.Settings.TokenUsage.InputTokens),
				formatTokens(s.Settings.TokenUsage.OutputTokens))),
		previewLabelStyle.Render("Active Time: ")+previewValueStyle.Render(formatDuration(s.ActiveDuration())),
	)
	b.WriteString(tokens)
	b.WriteString("\n")
	used++

	// extra token details + mode
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
		used++
	}

	// AI topics
	if sum != nil && len(sum.Topics) > 0 {
		maxTopics := min(len(sum.Topics), height-used-4) // reserve lines for separator + conversation
		if maxTopics > 0 {
			for _, t := range sum.Topics[:maxTopics] {
				line := previewLabelStyle.Render("● ") + previewValueStyle.Render(truncateDisplay(t.Summary, width-10))
				if len(t.Keywords) > 0 {
					kw := dimStyle.Render("  " + strings.Join(t.Keywords, ", "))
					line += kw
				}
				b.WriteString(line)
				b.WriteString("\n")
				used++
			}
		}
	}

	// separator
	b.WriteString(dimStyle.Render(strings.Repeat("─", min(width-2, 60))))
	b.WriteString("\n")
	used++

	// remaining lines for conversation
	msgLines := height - used
	if msgLines < 2 {
		msgLines = 2
	}
	maxLen := width - 8

	// collect meaningful messages
	type turnMsg struct {
		role string
		text string
	}
	var turns []turnMsg
	for _, msg := range s.Messages {
		if msg.Role != "user" && msg.Role != "assistant" {
			continue
		}
		text := extractText(msg.Content)
		if text == "" {
			continue
		}
		if len(text) > maxLen {
			text = text[:maxLen-3] + "..."
		}
		turns = append(turns, turnMsg{msg.Role, text})
	}

	if len(turns) == 0 {
		b.WriteString(dimStyle.Render("(no messages)"))
		return b.String()
	}

	if len(turns) <= msgLines {
		for _, t := range turns {
			b.WriteString(formatTurn(t.role, t.text))
			b.WriteString("\n")
		}
	} else {
		showHead := msgLines / 2
		showTail := msgLines - showHead - 1 // -1 for "..." line
		if showTail < 1 {
			showTail = 1
			showHead = msgLines - showTail - 1
		}
		if showHead > 0 {
			for _, t := range turns[:showHead] {
				b.WriteString(formatTurn(t.role, t.text))
				b.WriteString("\n")
			}
		}
		b.WriteString(dimStyle.Render(fmt.Sprintf("  ... (%d more) ...", len(turns)-showHead-showTail)))
		b.WriteString("\n")
		for _, t := range turns[len(turns)-showTail:] {
			b.WriteString(formatTurn(t.role, t.text))
			b.WriteString("\n")
		}
	}

	return b.String()
}

func formatTurn(role, text string) string {
	if role == "user" {
		return userMsgStyle.Render("User: ") + previewValueStyle.Render(text)
	}
	return assistantMsgStyle.Render("Asst: ") + dimStyle.Render(text)
}

