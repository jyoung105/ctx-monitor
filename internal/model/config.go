package model

// ClaudeConfig holds all parsed Claude Code configuration data.
type ClaudeConfig struct {
	MCP      MCPConfig      `json:"mcp"`
	Memory   MemoryConfig   `json:"memory"`
	Agents   AgentsConfig   `json:"agents"`
	Skills   SkillsConfig   `json:"skills"`
	Settings SettingsConfig `json:"settings"`
	OAuth    OAuthConfig    `json:"oauth"`
}

// CodexConfig holds all parsed Codex CLI configuration data.
type CodexConfig struct {
	Model               string            `json:"model"`
	ReasoningEffort     string            `json:"reasoningEffort"`
	CompactionThreshold interface{}       `json:"compactionThreshold"`
	MCP                 CodexMCPConfig    `json:"mcp"`
	Agents              CodexAgentsConfig `json:"agents"`
	Instructions        InstructionsInfo  `json:"instructions"`
	Skills              CodexSkillsConfig `json:"skills"`
}

// MCPConfig aggregates MCP server information from all config sources.
type MCPConfig struct {
	Servers     []MCPServerInfo `json:"servers"`
	TotalTokens int             `json:"totalTokens"`
}

// MemoryConfig aggregates CLAUDE.md memory file information.
type MemoryConfig struct {
	Files       []MemoryFile `json:"files"`
	TotalTokens int          `json:"totalTokens"`
}

// AgentsConfig aggregates agent definition file information.
type AgentsConfig struct {
	Files       []AgentFile `json:"files"`
	TotalTokens int         `json:"totalTokens"`
}

// SkillsConfig aggregates skill file information.
type SkillsConfig struct {
	Installed             []SkillFile `json:"installed"`
	Count                 int         `json:"count"`
	TotalFrontmatterTokens int        `json:"totalFrontmatterTokens"`
}

// SettingsConfig holds relevant settings from settings.json files.
type SettingsConfig struct {
	StatusLine  interface{}            `json:"statusLine"`
	Hooks       map[string]interface{} `json:"hooks"`
	Permissions PermissionsConfig      `json:"permissions"`
}

// PermissionsConfig holds user and project permission arrays.
type PermissionsConfig struct {
	User    []interface{} `json:"user"`
	Project []interface{} `json:"project"`
}

// OAuthConfig tracks OAuth token availability.
type OAuthConfig struct {
	HasToken bool `json:"hasToken"`
}

// CodexMCPConfig holds Codex MCP server information.
type CodexMCPConfig struct {
	Servers     []CodexMCPServer `json:"servers"`
	TotalTokens int              `json:"totalTokens"`
}

// CodexMCPServer describes a Codex MCP server.
type CodexMCPServer struct {
	Name            string   `json:"name"`
	URL             string   `json:"url,omitempty"`
	Command         string   `json:"command,omitempty"`
	Args            []string `json:"args,omitempty"`
	EnabledTools    []string `json:"enabledTools,omitempty"`
	DisabledTools   []string `json:"disabledTools,omitempty"`
	EstimatedTokens int      `json:"estimatedTokens"`
}

// CodexAgentsConfig holds Codex agent definitions.
type CodexAgentsConfig struct {
	Definitions []CodexAgentDef `json:"definitions"`
	TotalTokens int             `json:"totalTokens"`
}

// CodexAgentDef describes a Codex agent definition.
type CodexAgentDef struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Model       string `json:"model,omitempty"`
}

// InstructionsInfo holds information about the AGENTS.md instructions file.
type InstructionsInfo struct {
	Path   string `json:"path"`
	Chars  int    `json:"chars"`
	Tokens int    `json:"tokens"`
}

// CodexSkillsConfig holds Codex skill information.
type CodexSkillsConfig struct {
	Files       []CodexSkillFile `json:"files"`
	Count       int              `json:"count"`
	TotalTokens int              `json:"totalTokens"`
}

// CodexSkillFile describes a Codex skill file.
type CodexSkillFile struct {
	Name   string `json:"name"`
	Path   string `json:"path"`
	Chars  int    `json:"chars"`
	Tokens int    `json:"tokens"`
}
