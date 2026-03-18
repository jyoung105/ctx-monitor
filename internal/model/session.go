package model

// ClaudeSession holds parsed data from a Claude Code JSONL session file.
type ClaudeSession struct {
	SessionID    string     `json:"sessionId"`
	Model        string     `json:"model"`
	Version      string     `json:"version"`
	Cwd          string     `json:"cwd"`
	Timestamps   TimeRange  `json:"timestamps"`
	Continuation bool       `json:"isContinuation"`
	Messages     []Message  `json:"messages"`
	ToolCalls    []ToolCall `json:"toolCalls"`
	ToolResults  []ToolResult `json:"toolResults"`
	ThinkingStats ThinkingStats `json:"thinkingBlocks"`
	CompactionEvents CompactionStats `json:"compactionEvents"`
	Turns        []Turn       `json:"turns"`
	Attachments  []Attachment `json:"attachments"`
	SkillActivations []SkillActivation `json:"skillActivations"`
	SubagentSpawns   []SubagentSpawn   `json:"subagentSpawns"`
	PlanUsage    []PlanEvent  `json:"planUsage"`
	Usage        *UsageData   `json:"usage"`
	TokenBuckets TokenBuckets `json:"tokenBuckets"`
}

// CodexSession holds parsed data from a Codex CLI JSONL session file.
type CodexSession struct {
	File              string       `json:"file"`
	SessionID         string       `json:"sessionId"`
	Model             string       `json:"model"`
	ContextWindowSize int          `json:"contextWindowSize"`
	ReasoningEffort   string       `json:"reasoningEffort"`
	TokenUsage        CodexTokenUsage `json:"tokenUsage"`
	LastTokenUsage    CodexTokenUsage `json:"lastTokenUsage"`
	ToolCalls         []CodexToolCall `json:"toolCalls"`
	ToolResults       []ToolResult    `json:"toolResults"`
	CompactionEvents  []CompactionEvent `json:"compactionEvents"`
	SubagentSpawns    []CodexToolCall   `json:"subagentSpawns"`
	PlanUsage         []CodexToolCall   `json:"planUsage"`
	Turns             []interface{}     `json:"turns"`
	TokenBuckets      TokenBuckets      `json:"tokenBuckets"`
	RawStats          RawStats          `json:"_raw"`
}

// TimeRange tracks first and last timestamps.
type TimeRange struct {
	First string `json:"first"`
	Last  string `json:"last"`
}

// Message represents a parsed message from a session.
type Message struct {
	Role             string `json:"role"`
	Type             string `json:"type"`
	UUID             string `json:"uuid,omitempty"`
	ParentUUID       string `json:"parentUuid,omitempty"`
	Timestamp        string `json:"timestamp,omitempty"`
	Text             string `json:"text,omitempty"`
	TokenEstimate    int    `json:"tokenEstimate"`
	Model            string `json:"model,omitempty"`
	ToolCallCount    int    `json:"toolCallCount,omitempty"`
	ToolResultCount  int    `json:"toolResultCount,omitempty"`
	ThinkingBlockCount int  `json:"thinkingBlockCount,omitempty"`
	AttachmentCount  int    `json:"attachmentCount,omitempty"`
	AttachmentTokens int    `json:"attachmentTokens,omitempty"`
	UserType         string `json:"userType,omitempty"`
	IsSidechain      bool   `json:"isSidechain,omitempty"`
}

// ToolResult represents the output of a tool call.
type ToolResult struct {
	ToolUseID     string `json:"toolUseId,omitempty"`
	ToolCallID    string `json:"toolCallId,omitempty"`
	Content       string `json:"content"`
	TokenEstimate int    `json:"tokenEstimate"`
}

// ThinkingStats aggregates thinking block statistics.
type ThinkingStats struct {
	Count                int `json:"count"`
	TotalEstimatedTokens int `json:"totalEstimatedTokens"`
}

// CompactionStats tracks compaction events for Claude sessions.
type CompactionStats struct {
	Count      int      `json:"count"`
	Timestamps []string `json:"timestamps"`
}

// SkillActivation records a skill being loaded into context.
type SkillActivation struct {
	Skill     string `json:"skill"`
	Args      string `json:"args,omitempty"`
	Timestamp string `json:"timestamp,omitempty"`
	ID        string `json:"id,omitempty"`
}

// PlanEvent records plan/todo tool usage.
type PlanEvent struct {
	Tool      string      `json:"tool"`
	Input     interface{} `json:"input,omitempty"`
	Timestamp string      `json:"timestamp,omitempty"`
	ID        string      `json:"id,omitempty"`
}

// UsageData contains API usage metrics from the last assistant message.
type UsageData struct {
	InputTokens              int `json:"input_tokens,omitempty"`
	CacheCreationInputTokens int `json:"cache_creation_input_tokens,omitempty"`
	CacheReadInputTokens     int `json:"cache_read_input_tokens,omitempty"`
	OutputTokens             int `json:"output_tokens,omitempty"`
}

// TokenBuckets accumulates estimated token counts by category.
type TokenBuckets struct {
	UserMsg     int `json:"user_msg"`
	ToolResults int `json:"tool_results"`
	Responses   int `json:"responses"`
	Subagent    int `json:"subagent"`
	SkillBody   int `json:"skill_body"`
	Plan        int `json:"plan"`
	Thinking    int `json:"thinking"`
	Reasoning   int `json:"reasoning"`
}

// CodexTokenUsage tracks Codex-specific token usage.
type CodexTokenUsage struct {
	Total     int `json:"total"`
	Input     int `json:"input"`
	Cached    int `json:"cached"`
	Output    int `json:"output"`
	Reasoning int `json:"reasoning"`
}

// CodexToolCall represents a tool call in a Codex session.
type CodexToolCall struct {
	Name          string      `json:"name"`
	Arguments     interface{} `json:"arguments,omitempty"`
	ID            string      `json:"id,omitempty"`
	Timestamp     string      `json:"timestamp,omitempty"`
	TokenEstimate int         `json:"tokenEstimate"`
	PatchInfo     *PatchInfo  `json:"patchInfo,omitempty"`
}

// PatchInfo contains parsed apply_patch metadata.
type PatchInfo struct {
	Files []string  `json:"files"`
	Hunks []HunkInfo `json:"hunks"`
}

// HunkInfo describes a single diff hunk.
type HunkInfo struct {
	OldStart int `json:"oldStart"`
	OldCount int `json:"oldCount"`
	NewStart int `json:"newStart"`
	NewCount int `json:"newCount"`
}

// RawStats tracks parse metadata.
type RawStats struct {
	LineCount   int `json:"lineCount"`
	ParseErrors int `json:"parseErrors"`
}
