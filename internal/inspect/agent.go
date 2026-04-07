package inspect

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

const agentSystemPrompt = `你是一个 Droid (Factory AI CLI) session 健康度分析师。
用户会提供多个 session 的静态分析数据，包括上下文分布、工具使用、system prompt 注入、缓存命中率等。

请用中文分析并给出结构化报告，包含以下部分：

## 1. System Prompt 膨胀分析
- 哪些注入段落是重复的或不必要的
- skills/plugins 是否过多占用上下文
- 跨 session 的 system prompt 一致性问题

## 2. 工具使用问题
- 是否有注册了但从未使用的工具（白白占用 schema token）
- 是否有工具功能重叠
- 是否有更高效的替代方案（如用 Grep tool 替代 Execute strings|rg）
- 哪些工具的 result 体积过大，有优化空间

## 3. 缓存效率
- cache 命中率是否健康（>70% 为健康）
- 是否存在频繁打断缓存的模式

## 4. 上下文消耗模式
- 哪些操作模式在大量消耗上下文
- 对话是否过长应该分拆
- tool_result 占比是否过高

## 5. 优化建议
分三个层面给出具体可执行的建议：
- **系统配置**：plugins、settings、AGENTS.md 怎么调整
- **使用习惯**：操作方式怎么改进
- **Droid 配置**：droids、skills 怎么优化

每条建议要具体、可操作，不要泛泛而谈。`

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

// RunAgentAnalysis sends static analysis results to LLM for deep analysis.
func RunAgentAnalysis(ctx context.Context, cfg config.LLMConfig, analyses []SessionAnalysis) (string, error) {
	prompt := buildAnalysisPrompt(analyses)

	body, err := json.Marshal(chatRequest{
		Model: cfg.Model,
		Messages: []chatMessage{
			{Role: "system", Content: agentSystemPrompt},
			{Role: "user", Content: prompt},
		},
	})
	if err != nil {
		return "", err
	}

	baseURL := strings.TrimRight(cfg.BaseURL, "/")
	req, err := http.NewRequestWithContext(ctx, "POST", baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+cfg.APIKey)

	client := &http.Client{Timeout: 180 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("LLM request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("LLM API error %d: %s", resp.StatusCode, truncateStr(string(respBody), 300))
	}

	var chatResp chatResponse
	if err := json.Unmarshal(respBody, &chatResp); err != nil {
		return "", fmt.Errorf("parse response: %w", err)
	}
	if len(chatResp.Choices) == 0 {
		return "", fmt.Errorf("no choices in LLM response")
	}

	return strings.TrimSpace(chatResp.Choices[0].Message.Content), nil
}

func buildAnalysisPrompt(analyses []SessionAnalysis) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("以下是 %d 个 Droid session 的静态分析数据：\n\n", len(analyses)))

	for i, a := range analyses {
		sb.WriteString(fmt.Sprintf("━━ Session %d: %s (%s) ━━\n", i+1, a.Session.Meta.ID[:8], a.Session.Project))
		sb.WriteString(fmt.Sprintf("模型: %s | 消息: %d 轮 | Token: input=%d output=%d cache_read=%d cache_create=%d thinking=%d\n",
			a.Settings.Model, a.MessageCount,
			a.CacheAnalysis.InputTokens, a.CacheAnalysis.OutputTokens,
			a.CacheAnalysis.CacheReadTokens, a.CacheAnalysis.CacheCreationTokens,
			a.CacheAnalysis.ThinkingTokens))
		sb.WriteString(fmt.Sprintf("Cache 命中率: %.1f%%\n\n", a.CacheAnalysis.HitRate))

		// distribution
		d := a.Distribution
		sb.WriteString("上下文分布 (chars):\n")
		sb.WriteString(fmt.Sprintf("  system_prompt:      %d (%.1f%%)\n", d.SystemPrompt, d.Pct(d.SystemPrompt)))
		sb.WriteString(fmt.Sprintf("  system_reminder:    %d (%.1f%%)\n", d.SystemReminder, d.Pct(d.SystemReminder)))
		sb.WriteString(fmt.Sprintf("  user_text:          %d (%.1f%%)\n", d.UserText, d.Pct(d.UserText)))
		sb.WriteString(fmt.Sprintf("  tool_result:        %d (%.1f%%)\n", d.ToolResult, d.Pct(d.ToolResult)))
		sb.WriteString(fmt.Sprintf("  assistant_text:     %d (%.1f%%)\n", d.AssistantText, d.Pct(d.AssistantText)))
		sb.WriteString(fmt.Sprintf("  assistant_thinking: %d (%.1f%%)\n", d.AssistantThink, d.Pct(d.AssistantThink)))
		sb.WriteString(fmt.Sprintf("  assistant_tool_use: %d (%.1f%%)\n", d.AssistantToolUse, d.Pct(d.AssistantToolUse)))
		sb.WriteString(fmt.Sprintf("  total:              %d\n\n", d.Total))

		// tools
		sb.WriteString("工具使用:\n")
		for _, t := range a.ToolStats {
			sb.WriteString(fmt.Sprintf("  %-20s calls=%d result_chars=%d max_single=%d\n",
				t.Name, t.CallCount, t.ResultChars, t.MaxResult))
		}
		sb.WriteString("\n")

		// system prompt segments
		sb.WriteString("System Prompt 注入段落:\n")
		sb.WriteString(fmt.Sprintf("  总长: %d chars\n", a.SystemPrompt.TotalChars))
		sb.WriteString(fmt.Sprintf("  system-reminder 注入: %d 次, %d chars\n", a.SystemPrompt.ReminderCount, a.SystemPrompt.ReminderChars))
		for _, seg := range a.SystemPrompt.Segments {
			sb.WriteString(fmt.Sprintf("  <%s>: %d 次, %d chars\n", seg.Tag, seg.Count, seg.Chars))
		}
		sb.WriteString("\n")

		// system prompt full text (truncated for LLM context)
		if a.SystemPrompt.FullText != "" {
			text := a.SystemPrompt.FullText
			if len(text) > 4000 {
				text = text[:4000] + "\n... (truncated)"
			}
			sb.WriteString("System Prompt 内容:\n")
			sb.WriteString(text)
			sb.WriteString("\n\n")
		}
	}

	return sb.String()
}

func truncateStr(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
