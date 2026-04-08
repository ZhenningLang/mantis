package compress

type CompressionInput struct {
	SourceSession SourceSessionInfo    `json:"source_session"`
	Anchors       CompressionAnchors   `json:"anchors"`
	TaskState     CompressionTaskState `json:"task_state"`
	Compaction    CompactionWindow     `json:"compaction"`
	ArtifactTrail ArtifactTrail        `json:"artifact_trail"`
	Errors        CompressionErrors    `json:"errors"`
	RecentWindow  []RecentTurn         `json:"recent_window"`
}

type SourceSessionInfo struct {
	ID      string `json:"id"`
	Title   string `json:"title"`
	CWD     string `json:"cwd"`
	ModelID string `json:"model_id"`
}

type CompressionAnchors struct {
	InitialUserGoal string   `json:"initial_user_goal"`
	UserConstraints []string `json:"user_constraints"`
	UserCorrections []string `json:"user_corrections"`
	LastUserRequest string   `json:"last_user_request"`
}

type CompressionTaskState struct {
	Completed  []string `json:"completed"`
	InProgress []string `json:"in_progress"`
	Pending    []string `json:"pending"`
}

type CompactionWindow struct {
	AnchorMessageIndex   int               `json:"anchor_message_index"`
	RemovedCount         int               `json:"removed_count"`
	PreservedCount       int               `json:"preserved_count"`
	SummarySoftCapTokens int               `json:"summary_soft_cap_tokens"`
	SummaryReserveTokens int               `json:"summary_reserve_tokens"`
	ActiveSkills         []string          `json:"active_skills"`
	SummarizedTurns      []RecentTurn      `json:"summarized_turns"`
	SummarizedPhases     []CompactionPhase `json:"summarized_phases"`
}

type CompactionPhase struct {
	Goal        string   `json:"goal"`
	Outcome     string   `json:"outcome"`
	KeyFiles    []string `json:"key_files"`
	KeyCommands []string `json:"key_commands"`
	OpenIssues  []string `json:"open_issues"`
	Skills      []string `json:"skills"`
}

type ArtifactTrail struct {
	FilesRead     []ArtifactFileRead   `json:"files_read"`
	FilesModified []ArtifactFileChange `json:"files_modified"`
	FilesCreated  []ArtifactFileCreate `json:"files_created"`
	Commands      []ArtifactCommand    `json:"commands"`
	Searches      []ArtifactSearch     `json:"searches"`
	GitOps        []ArtifactGitOp      `json:"git_ops"`
}

type ArtifactFileRead struct {
	Path string `json:"path"`
	Why  string `json:"why"`
}

type ArtifactFileChange struct {
	Path    string   `json:"path"`
	Symbols []string `json:"symbols"`
	Change  string   `json:"change"`
}

type ArtifactFileCreate struct {
	Path    string `json:"path"`
	Purpose string `json:"purpose"`
}

type ArtifactCommand struct {
	Cmd      string `json:"cmd"`
	Status   string `json:"status"`
	Evidence string `json:"evidence"`
}

type ArtifactSearch struct {
	Tool    string `json:"tool"`
	Query   string `json:"query"`
	Finding string `json:"finding"`
}

type ArtifactGitOp struct {
	Op       string `json:"op"`
	Evidence string `json:"evidence"`
}

type CompressionErrors struct {
	Resolved   []ResolvedError   `json:"resolved"`
	Unresolved []UnresolvedError `json:"unresolved"`
}

type ResolvedError struct {
	Error      string `json:"error"`
	Resolution string `json:"resolution"`
}

type UnresolvedError struct {
	Error      string `json:"error"`
	NextAction string `json:"next_action"`
}

type RecentTurn struct {
	Role string `json:"role"`
	Text string `json:"text"`
}

type LLMHandoff struct {
	CompressedTitle   string               `json:"compressed_title"`
	Objective         string               `json:"objective"`
	Constraints       []string             `json:"constraints"`
	ActiveSkills      []string             `json:"active_skills"`
	CompactedHistory  []CompactionPhase    `json:"compacted_history"`
	RecentTranscript  []RecentTurn         `json:"-"`
	KeyDecisions      []HandoffDecision    `json:"key_decisions"`
	TaskState         HandoffTaskState     `json:"task_state"`
	ArtifactFocus     HandoffArtifactFocus `json:"artifact_focus"`
	CurrentState      HandoffCurrentState  `json:"current_state"`
	ResumeInstruction string               `json:"resume_instruction"`
}

type HandoffDecision struct {
	Decision string  `json:"decision"`
	Status   string  `json:"status"`
	Why      *string `json:"why"`
}

type HandoffTaskState struct {
	Completed  []string `json:"completed"`
	InProgress []string `json:"in_progress"`
	Pending    []string `json:"pending"`
}

type HandoffArtifactFocus struct {
	MustKeepFiles     []HandoffFile     `json:"must_keep_files"`
	OtherTouchedFiles []string          `json:"other_touched_files"`
	KeyCommands       []HandoffCommand  `json:"key_commands"`
	UnresolvedErrors  []UnresolvedError `json:"unresolved_errors"`
}

type HandoffFile struct {
	Path    string   `json:"path"`
	Symbols []string `json:"symbols"`
	Reason  string   `json:"reason"`
}

type HandoffCommand struct {
	Cmd      string `json:"cmd"`
	Outcome  string `json:"outcome"`
	Evidence string `json:"evidence"`
}

type HandoffCurrentState struct {
	Done          string   `json:"done"`
	OpenQuestions []string `json:"open_questions"`
	NextSteps     []string `json:"next_steps"`
}

type DroidSettings struct {
	CustomModels           []CustomModel        `json:"customModels"`
	SessionDefaultSettings DroidSessionDefaults `json:"sessionDefaultSettings"`
}

type DroidSessionDefaults struct {
	Model string `json:"model"`
}

type CustomModel struct {
	Model     string         `json:"model"`
	ID        string         `json:"id"`
	BaseURL   string         `json:"baseUrl"`
	APIKey    string         `json:"apiKey"`
	Provider  string         `json:"provider"`
	ExtraArgs map[string]any `json:"extraArgs,omitempty"`
}

type LLMAuth struct {
	BaseURL   string
	APIKey    string
	Provider  string
	ExtraArgs map[string]any
}
