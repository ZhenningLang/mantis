package inspect

import "github.com/zhenninglang/mantis/internal/session"

// SessionAnalysis holds all analysis results for a single session.
type SessionAnalysis struct {
	Session  session.Session
	Settings session.Settings
	Events   []session.RawEvent

	MessageCount int
	IsSubagent   bool

	Distribution  ContentDistribution
	ToolStats     []ToolStat
	SystemPrompt  SystemPromptAnalysis
	CacheAnalysis CacheAnalysis
}

type ContentDistribution struct {
	SystemPrompt     int // chars
	SystemReminder   int
	UserText         int
	ToolResult       int
	AssistantText    int
	AssistantThink   int
	AssistantToolUse int
	Total            int
}

func (d ContentDistribution) Pct(val int) float64 {
	if d.Total == 0 {
		return 0
	}
	return float64(val) / float64(d.Total) * 100
}

type ToolStat struct {
	Name        string
	CallCount   int
	ResultChars int
	MaxResult   int // largest single result
}

type SystemPromptAnalysis struct {
	FullText          string
	TotalChars        int
	ReminderCount     int
	ReminderChars     int
	Segments          []PromptSegment
}

type PromptSegment struct {
	Tag   string
	Chars int
	Count int
}

type CacheAnalysis struct {
	InputTokens         int
	OutputTokens        int
	CacheReadTokens     int
	CacheCreationTokens int
	ThinkingTokens      int
	HitRate             float64 // cacheRead / (input + cacheRead)
}

// InspectReport is the final output containing all sessions + agent analysis.
type InspectReport struct {
	Sessions      []SessionAnalysis
	AgentAnalysis string // raw LLM response
}
