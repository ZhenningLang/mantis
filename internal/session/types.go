package session

import "time"

type TokenUsage struct {
	InputTokens         int `json:"inputTokens"`
	OutputTokens        int `json:"outputTokens"`
	CacheCreationTokens int `json:"cacheCreationTokens"`
	CacheReadTokens     int `json:"cacheReadTokens"`
	ThinkingTokens      int `json:"thinkingTokens"`
}

type Settings struct {
	AssistantActiveTimeMs   int        `json:"assistantActiveTimeMs"`
	Model                   string     `json:"model"`
	ReasoningEffort         string     `json:"reasoningEffort"`
	InteractionMode         string     `json:"interactionMode"`
	AutonomyLevel           string     `json:"autonomyLevel"`
	AutonomyMode            string     `json:"autonomyMode"`
	SpecModeReasoningEffort string     `json:"specModeReasoningEffort"`
	ProviderLock            string     `json:"providerLock"`
	TokenUsage              TokenUsage `json:"tokenUsage"`
}

type SessionMeta struct {
	ID               string `json:"id"`
	Title            string `json:"title"`
	WorkingDirectory string `json:"cwd"`
}

type Message struct {
	Role    string `json:"role"`
	Content any    `json:"content"`
}

type Session struct {
	Meta        SessionMeta
	Settings    Settings
	Project     string // short name (last segment)
	ProjectFull string // full directory path
	ModTime     time.Time
	FilePath    string
	Messages    []Message
	Selected    bool
}

func (s *Session) ProjectShort() string {
	if s.Project == "" {
		return "global"
	}
	return s.Project
}

func (s *Session) ProjectDisplay(full bool) string {
	if full {
		if s.ProjectFull != "" {
			return s.ProjectFull
		}
		if s.Meta.WorkingDirectory != "" {
			return s.Meta.WorkingDirectory
		}
	}
	return s.ProjectShort()
}

func (s *Session) TotalTokens() int {
	u := s.Settings.TokenUsage
	return u.InputTokens + u.OutputTokens + u.CacheCreationTokens + u.CacheReadTokens + u.ThinkingTokens
}

func (s *Session) ActiveDuration() time.Duration {
	return time.Duration(s.Settings.AssistantActiveTimeMs) * time.Millisecond
}

// RawEvent represents a single line from a session JSONL file with full fidelity.
type RawEvent struct {
	Type             string      `json:"type"`
	Message          *RawMessage `json:"message,omitempty"`
	Todos            *RawTodos   `json:"todos,omitempty"`
	ID               string      `json:"id,omitempty"`
	Title            string      `json:"title,omitempty"`
	CWD              string      `json:"cwd,omitempty"`
	CallingSessionID string      `json:"callingSessionId,omitempty"`
}

type RawMessage struct {
	Role    string           `json:"role"`
	Content []RawContentItem `json:"content"`
}

type RawContentItem struct {
	Type      string `json:"type"`
	Text      string `json:"text,omitempty"`
	Content   string `json:"content,omitempty"` // tool_result content
	IsError   bool   `json:"is_error,omitempty"`
	ID        string `json:"id,omitempty"`   // tool_use id
	Name      string `json:"name,omitempty"` // tool_use name
	ToolUseID string `json:"tool_use_id,omitempty"`
	Input     any    `json:"input,omitempty"`
	Thinking  string `json:"thinking,omitempty"`
	Signature string `json:"signature,omitempty"`
}

type RawTodos struct {
	Todos string `json:"todos"`
}
