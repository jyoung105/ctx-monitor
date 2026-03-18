package model

// Component represents a single context window component with its token count.
type Component struct {
	Key       string  `json:"key"`
	Label     string  `json:"label"`
	Tokens    int     `json:"tokens"`
	Pct       float64 `json:"pct"`
	Color     string  `json:"color"`
	TextColor string  `json:"textColor"`
	Fixed     bool    `json:"fixed"`
	Source    string  `json:"source"` // "estimated" or "measured"
}

// Composition is the full context window breakdown for a session.
type Composition struct {
	Tool              string      `json:"tool"` // "claude" or "codex"
	Model             string      `json:"model"`
	ModelID           string      `json:"modelId"`
	ModelTier         string      `json:"modelTier"`
	ModelReasoning    string      `json:"modelReasoning"`
	IsFast            bool        `json:"isFast"`
	SessionID         string      `json:"sessionId"`
	ContextWindowSize int         `json:"contextWindowSize"`
	TotalUsedPct      float64     `json:"totalUsedPct"`
	TotalUsedTokens   int         `json:"totalUsedTokens"`
	APIMatchPct       float64     `json:"apiMatchPct,omitempty"`
	APIMatchTokens    int         `json:"apiMatchTokens,omitempty"`
	FreeTokens        int         `json:"freeTokens"`
	Components        []Component `json:"components"`

	CompactionEvents []CompactionEvent `json:"compactionEvents,omitempty"`
	Subagents        []SubagentSpawn   `json:"subagents,omitempty"`
	MCPServers       []MCPServerInfo   `json:"mcpServers,omitempty"`
	Skills           *SkillSummary     `json:"skills,omitempty"`
	MemoryFiles      []MemoryFile      `json:"memoryFiles,omitempty"`
	AgentFiles       []AgentFile       `json:"agentFiles,omitempty"`
	PlanUsage        interface{}       `json:"planUsage,omitempty"`
	ToolCalls        []ToolCall        `json:"toolCalls,omitempty"`
	Turns            []Turn            `json:"turns,omitempty"`
	Attachments      []Attachment      `json:"attachments,omitempty"`
	Timestamp        string            `json:"timestamp"`
}

// CompactionEvent records when context was compacted.
type CompactionEvent struct {
	Timestamp string `json:"timestamp,omitempty"`
	PreSize   int    `json:"preSize,omitempty"`
	PostSize  int    `json:"postSize,omitempty"`
}

// SubagentSpawn records a subagent/task creation.
type SubagentSpawn struct {
	Tool         string `json:"tool"`
	SubagentType string `json:"subagentType,omitempty"`
	Description  string `json:"description,omitempty"`
	Timestamp    string `json:"timestamp,omitempty"`
	ID           string `json:"id,omitempty"`
}

// MCPServerInfo describes an MCP server and its estimated token cost.
type MCPServerInfo struct {
	Name            string `json:"name"`
	ToolCount       int    `json:"toolCount"`
	EstimatedTokens int    `json:"estimatedTokens"`
}

// SkillSummary aggregates skill information.
type SkillSummary struct {
	Installed         int      `json:"installed"`
	Active            []string `json:"active"`
	FrontmatterTokens int      `json:"frontmatterTokens"`
	BodyTokens        int      `json:"bodyTokens"`
	Files             []SkillFile `json:"files,omitempty"`
}

// SkillFile describes a discovered skill.
type SkillFile struct {
	Name              string `json:"name"`
	Path              string `json:"path"`
	FrontmatterTokens int    `json:"frontmatterTokens"`
}

// MemoryFile describes a CLAUDE.md memory file.
type MemoryFile struct {
	Path   string `json:"path"`
	Chars  int    `json:"chars"`
	Tokens int    `json:"tokens"`
}

// AgentFile describes an agent definition file.
type AgentFile struct {
	Name   string `json:"name"`
	Path   string `json:"path"`
	Tokens int    `json:"tokens"`
}

// ToolCall records a tool invocation in the session.
type ToolCall struct {
	Name          string      `json:"name"`
	Input         interface{} `json:"input,omitempty"`
	ID            string      `json:"id,omitempty"`
	Timestamp     string      `json:"timestamp,omitempty"`
	TokenEstimate int         `json:"tokenEstimate"`
	FilePath      string      `json:"filePath,omitempty"`
}

// Turn represents a user turn boundary.
type Turn struct {
	Index        int    `json:"index"`
	Timestamp    string `json:"timestamp,omitempty"`
	Text         string `json:"text,omitempty"`
	Attachments  int    `json:"attachments,omitempty"`
	MessageIndex int    `json:"messageIndex,omitempty"`
}

// Attachment represents an image or file attachment.
type Attachment struct {
	Type      string `json:"type"`
	MediaType string `json:"mediaType,omitempty"`
	Name      string `json:"name,omitempty"`
	Tokens    int    `json:"tokens"`
	SizeBytes int    `json:"sizeBytes,omitempty"`
	Timestamp string `json:"timestamp,omitempty"`
}
