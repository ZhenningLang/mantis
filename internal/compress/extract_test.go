package compress

import (
	"strings"
	"testing"

	"github.com/zhenninglang/mantis/internal/session"
)

func TestBuildCompressionInputPreservesArtifactsAndTaskState(t *testing.T) {
	source := session.Session{
		Meta: session.SessionMeta{
			ID:               "abc123",
			Title:            "Original Title",
			WorkingDirectory: "/tmp/project",
		},
		Settings: session.Settings{Model: "custom:GPT-5.4-1"},
	}

	events := []session.RawEvent{
		{
			Type: "message",
			Message: &session.RawMessage{
				Role: "user",
				Content: []session.RawContentItem{
					{Type: "text", Text: "请实现 mantis compress，注意不要依赖内建 compress。"},
				},
			},
		},
		{
			Type: "message",
			Message: &session.RawMessage{
				Role: "assistant",
				Content: []session.RawContentItem{
					{Type: "tool_use", ID: "read1", Name: "Read", Input: map[string]any{"file_path": "/tmp/project/main.go"}},
				},
			},
		},
		{
			Type: "message",
			Message: &session.RawMessage{
				Role: "user",
				Content: []session.RawContentItem{
					{Type: "tool_result", ToolUseID: "read1", Content: "package main", IsError: false},
				},
			},
		},
		{
			Type: "message",
			Message: &session.RawMessage{
				Role: "assistant",
				Content: []session.RawContentItem{
					{Type: "tool_use", ID: "cmd1", Name: "Execute", Input: map[string]any{"command": "go test ./...", "riskLevel": "low"}},
				},
			},
		},
		{
			Type: "message",
			Message: &session.RawMessage{
				Role: "user",
				Content: []session.RawContentItem{
					{Type: "tool_result", ToolUseID: "cmd1", Content: "boom\n[Process exited with code 1]", IsError: true},
				},
			},
		},
		{
			Type: "message",
			Message: &session.RawMessage{
				Role: "assistant",
				Content: []session.RawContentItem{
					{Type: "tool_use", ID: "search1", Name: "Grep", Input: map[string]any{"pattern": "compress", "path": "/tmp/project"}},
				},
			},
		},
		{
			Type: "message",
			Message: &session.RawMessage{
				Role: "user",
				Content: []session.RawContentItem{
					{Type: "tool_result", ToolUseID: "search1", Content: "./main.go:12:compress", IsError: false},
				},
			},
		},
		{
			Type: "todo_state",
			Todos: &session.RawTodos{
				Todos: "1. [completed] 阅读 spec\n2. [in_progress] 写失败测试\n3. [pending] 实现压缩功能",
			},
		},
		{
			Type: "message",
			Message: &session.RawMessage{
				Role: "user",
				Content: []session.RawContentItem{
					{Type: "text", Text: "开始实现 spec 中的内容。"},
				},
			},
		},
	}

	input := BuildCompressionInput(source, events)

	if input.SourceSession.ID != "abc123" || input.SourceSession.Title != "Original Title" || input.SourceSession.ModelID != "custom:GPT-5.4-1" {
		t.Fatalf("unexpected source session: %#v", input.SourceSession)
	}
	if input.Anchors.InitialUserGoal != "请实现 mantis compress，注意不要依赖内建 compress。" {
		t.Fatalf("InitialUserGoal = %q", input.Anchors.InitialUserGoal)
	}
	if input.Anchors.LastUserRequest != "开始实现 spec 中的内容。" {
		t.Fatalf("LastUserRequest = %q", input.Anchors.LastUserRequest)
	}
	if len(input.TaskState.Completed) != 1 || input.TaskState.Completed[0] != "阅读 spec" {
		t.Fatalf("Completed = %#v", input.TaskState.Completed)
	}
	if len(input.TaskState.InProgress) != 1 || input.TaskState.InProgress[0] != "写失败测试" {
		t.Fatalf("InProgress = %#v", input.TaskState.InProgress)
	}
	if len(input.TaskState.Pending) != 1 || input.TaskState.Pending[0] != "实现压缩功能" {
		t.Fatalf("Pending = %#v", input.TaskState.Pending)
	}
	if len(input.ArtifactTrail.FilesRead) != 1 || input.ArtifactTrail.FilesRead[0].Path != "/tmp/project/main.go" {
		t.Fatalf("FilesRead = %#v", input.ArtifactTrail.FilesRead)
	}
	if len(input.ArtifactTrail.Commands) != 1 || input.ArtifactTrail.Commands[0].Cmd != "go test ./..." {
		t.Fatalf("Commands = %#v", input.ArtifactTrail.Commands)
	}
	if input.ArtifactTrail.Commands[0].Status != "error" {
		t.Fatalf("Commands[0].Status = %q", input.ArtifactTrail.Commands[0].Status)
	}
	if len(input.ArtifactTrail.Searches) != 1 || input.ArtifactTrail.Searches[0].Query != "compress" {
		t.Fatalf("Searches = %#v", input.ArtifactTrail.Searches)
	}
	if len(input.Errors.Unresolved) != 1 || input.Errors.Unresolved[0].Error != "boom" {
		t.Fatalf("Unresolved = %#v", input.Errors.Unresolved)
	}
	if len(input.RecentWindow) == 0 {
		t.Fatalf("RecentWindow should not be empty")
	}
}

func TestBuildCompressionInputUsesCompactionAnchorAndSkillState(t *testing.T) {
	source := session.Session{
		Meta: session.SessionMeta{
			ID:               "abc123",
			Title:            "Original Title",
			WorkingDirectory: "/tmp/project",
		},
		Settings: session.Settings{Model: "custom:GPT-5.4-1"},
	}

	makeText := func(prefix string, size int) string {
		return prefix + strings.Repeat("x", size)
	}

	events := []session.RawEvent{
		{
			Type: "message",
			Message: &session.RawMessage{
				Role:    "user",
				Content: []session.RawContentItem{{Type: "text", Text: "这条消息应该被 anchor 忽略"}},
			},
		},
		{
			Type: "message",
			Message: &session.RawMessage{
				Role:    "user",
				Content: []session.RawContentItem{{Type: "text", Text: "<compressed_session_handoff source_session=\"old\"></compressed_session_handoff>"}},
			},
		},
		{
			Type: "message",
			Message: &session.RawMessage{
				Role:    "assistant",
				Content: []session.RawContentItem{{Type: "tool_use", ID: "skill1", Name: "Skill", Input: map[string]any{"skill": "se-tdd"}}},
			},
		},
		{
			Type: "message",
			Message: &session.RawMessage{
				Role:    "user",
				Content: []session.RawContentItem{{Type: "text", Text: "新的目标：继续实现压缩算法"}},
			},
		},
	}

	for i := 0; i < 7; i++ {
		role := "assistant"
		if i%2 == 0 {
			role = "user"
		}
		events = append(events, session.RawEvent{
			Type: "message",
			Message: &session.RawMessage{
				Role: role,
				Content: []session.RawContentItem{{
					Type: "text",
					Text: makeText("turn-", 10000),
				}},
			},
		})
	}

	input := BuildCompressionInput(source, events)

	if input.Anchors.InitialUserGoal != "新的目标：继续实现压缩算法" {
		t.Fatalf("InitialUserGoal = %q", input.Anchors.InitialUserGoal)
	}
	if input.Compaction.AnchorMessageIndex != 1 {
		t.Fatalf("AnchorMessageIndex = %d", input.Compaction.AnchorMessageIndex)
	}
	if len(input.Compaction.ActiveSkills) != 1 || input.Compaction.ActiveSkills[0] != "se-tdd" {
		t.Fatalf("ActiveSkills = %#v", input.Compaction.ActiveSkills)
	}
	if len(input.RecentWindow) < 2 {
		t.Fatalf("RecentWindow len = %d, want >= 2", len(input.RecentWindow))
	}
	if input.Compaction.RemovedCount == 0 || len(input.Compaction.SummarizedTurns) == 0 {
		t.Fatalf("Compaction = %#v", input.Compaction)
	}
	foundGoal := false
	for _, phase := range input.Compaction.SummarizedPhases {
		if phase.Goal == "新的目标：继续实现压缩算法" {
			foundGoal = true
			break
		}
	}
	if !foundGoal {
		t.Fatalf("SummarizedPhases = %#v", input.Compaction.SummarizedPhases)
	}
	if len(input.Compaction.ActiveSkills) != 1 || input.Compaction.ActiveSkills[0] != "se-tdd" {
		t.Fatalf("ActiveSkills = %#v", input.Compaction.ActiveSkills)
	}
}

func TestBuildCompressionInputSkipsBYOKErrorMessages(t *testing.T) {
	source := session.Session{
		Meta: session.SessionMeta{
			ID:               "abc123",
			Title:            "Original Title",
			WorkingDirectory: "/tmp/project",
		},
		Settings: session.Settings{Model: "custom:GPT-5.4-1"},
	}

	events := []session.RawEvent{
		{
			Type: "message",
			Message: &session.RawMessage{
				Role: "user",
				Content: []session.RawContentItem{
					{Type: "text", Text: "开始，fire in the hole"},
				},
			},
		},
		{
			Type: "message",
			Message: &session.RawMessage{
				Role: "user",
				Content: []session.RawContentItem{
					{Type: "text", Text: "BYOK Error: No text content returned from chat completion If this persists, verify your custom model configuration in settings."},
				},
			},
		},
		{
			Type: "message",
			Message: &session.RawMessage{
				Role: "user",
				Content: []session.RawContentItem{
					{Type: "text", Text: "继续"},
				},
			},
		},
	}

	input := BuildCompressionInput(source, events)

	if input.Anchors.InitialUserGoal != "开始，fire in the hole" {
		t.Fatalf("InitialUserGoal = %q", input.Anchors.InitialUserGoal)
	}
	if input.Anchors.LastUserRequest != "继续" {
		t.Fatalf("LastUserRequest = %q", input.Anchors.LastUserRequest)
	}
	if len(input.RecentWindow) != 2 {
		t.Fatalf("RecentWindow len = %d, want 2", len(input.RecentWindow))
	}
	for _, turn := range input.RecentWindow {
		if strings.HasPrefix(turn.Text, "BYOK Error:") {
			t.Fatalf("RecentWindow should skip BYOK errors: %#v", input.RecentWindow)
		}
	}
}
