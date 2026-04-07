package inspect

import (
	"fmt"
	"strings"
)

const (
	colorReset  = "\033[0m"
	colorRed    = "\033[31m"
	colorGreen  = "\033[32m"
	colorYellow = "\033[33m"
	colorBlue   = "\033[34m"
	colorCyan   = "\033[36m"
	colorDim    = "\033[2m"
	colorBold   = "\033[1m"
)

func PrintReport(report InspectReport) string {
	var sb strings.Builder

	sb.WriteString("\n")
	sb.WriteString(colorBold + "══════════════════════════════════════════════════\n")
	sb.WriteString("  mantis context health inspector\n")
	sb.WriteString(fmt.Sprintf("  分析 %d 个 session\n", len(report.Sessions)))
	sb.WriteString("══════════════════════════════════════════════════" + colorReset + "\n\n")

	for _, a := range report.Sessions {
		printSessionReport(&sb, a)
	}

	if report.AgentAnalysis != "" {
		sb.WriteString(colorBold + "━━ Agent 分析报告 ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━" + colorReset + "\n\n")
		sb.WriteString(report.AgentAnalysis)
		sb.WriteString("\n\n")
	}

	return sb.String()
}

func printSessionReport(sb *strings.Builder, a SessionAnalysis) {
	sid := a.Session.Meta.ID
	if len(sid) > 8 {
		sid = sid[:8]
	}

	sb.WriteString(colorBold + fmt.Sprintf("━━ Session: %s (%s) ━━━━━━━━━━━━━", sid, a.Session.Project) + colorReset + "\n\n")

	// basic info
	sb.WriteString(fmt.Sprintf("  消息: %d 轮 · 模型: %s\n", a.MessageCount, a.Settings.Model))

	// cache
	cacheColor := colorGreen
	cacheIcon := "✓"
	if a.CacheAnalysis.HitRate < 70 {
		cacheColor = colorYellow
		cacheIcon = "⚠"
	}
	if a.CacheAnalysis.HitRate < 30 {
		cacheColor = colorRed
		cacheIcon = "✗"
	}
	sb.WriteString(fmt.Sprintf("  Cache 命中率: %s%.0f%% %s%s\n\n", cacheColor, a.CacheAnalysis.HitRate, cacheIcon, colorReset))

	// token usage
	sb.WriteString(colorDim + "  Token 用量:\n" + colorReset)
	sb.WriteString(fmt.Sprintf("    Input: %s  Output: %s  CacheRead: %s  CacheCreate: %s  Thinking: %s\n\n",
		fmtNum(a.CacheAnalysis.InputTokens),
		fmtNum(a.CacheAnalysis.OutputTokens),
		fmtNum(a.CacheAnalysis.CacheReadTokens),
		fmtNum(a.CacheAnalysis.CacheCreationTokens),
		fmtNum(a.CacheAnalysis.ThinkingTokens)))

	// distribution
	sb.WriteString("  上下文分布:\n")
	d := a.Distribution
	printBar(sb, "tool_result", d.ToolResult, d.Total)
	printBar(sb, "assistant_tool_use", d.AssistantToolUse, d.Total)
	printBar(sb, "assistant_thinking", d.AssistantThink, d.Total)
	printBar(sb, "system_prompt", d.SystemPrompt, d.Total)
	printBar(sb, "system_reminder", d.SystemReminder, d.Total)
	printBar(sb, "assistant_text", d.AssistantText, d.Total)
	printBar(sb, "user_text", d.UserText, d.Total)
	sb.WriteString("\n")

	// tool stats
	if len(a.ToolStats) > 0 {
		sb.WriteString("  Tool 热点:\n")
		totalResult := 0
		for _, t := range a.ToolStats {
			totalResult += t.ResultChars
		}
		for _, t := range a.ToolStats {
			pct := float64(0)
			if totalResult > 0 {
				pct = float64(t.ResultChars) / float64(totalResult) * 100
			}
			icon := colorGreen + "●" + colorReset
			if t.MaxResult > 20000 {
				icon = colorRed + "●" + colorReset
			} else if t.MaxResult > 5000 {
				icon = colorYellow + "●" + colorReset
			}
			sb.WriteString(fmt.Sprintf("    %s %-18s %4.1f%%  calls=%d  max=%s\n",
				icon, t.Name, pct, t.CallCount, fmtNum(t.MaxResult)))
		}
		sb.WriteString("\n")
	}

	// system prompt
	sb.WriteString("  System Prompt:\n")
	sb.WriteString(fmt.Sprintf("    总长: %s chars\n", fmtNum(a.SystemPrompt.TotalChars)))
	if a.SystemPrompt.ReminderCount > 0 {
		sb.WriteString(fmt.Sprintf("    system-reminder ×%d (累计 %s chars)\n",
			a.SystemPrompt.ReminderCount, fmtNum(a.SystemPrompt.ReminderChars)))
	}
	for _, seg := range a.SystemPrompt.Segments {
		if seg.Tag == "system-reminder" {
			continue // already shown
		}
		sb.WriteString(fmt.Sprintf("    <%s> ×%d (%s chars)\n", seg.Tag, seg.Count, fmtNum(seg.Chars)))
	}
	sb.WriteString("\n")
}

func printBar(sb *strings.Builder, label string, val, total int) {
	pct := float64(0)
	if total > 0 {
		pct = float64(val) / float64(total) * 100
	}
	barLen := int(pct / 2)
	if barLen > 30 {
		barLen = 30
	}
	bar := strings.Repeat("█", barLen)
	if barLen == 0 && val > 0 {
		bar = "▏"
	}

	color := colorGreen
	if pct > 50 {
		color = colorRed
	} else if pct > 20 {
		color = colorYellow
	}

	sb.WriteString(fmt.Sprintf("    %-20s %s%5.1f%% %s%s\n", label, color, pct, bar, colorReset))
}

func fmtNum(n int) string {
	if n >= 1000000 {
		return fmt.Sprintf("%.1fM", float64(n)/1000000)
	}
	if n >= 1000 {
		return fmt.Sprintf("%.1fK", float64(n)/1000)
	}
	return fmt.Sprintf("%d", n)
}
