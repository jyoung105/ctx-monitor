package model

import "fmt"

// ComponentDef defines a context window component with display metadata.
type ComponentDef struct {
	Key           string
	Label         string
	Color         string // hex color for HTML
	TextColor     string // hex text color for HTML
	Ansi          int    // ANSI 256-color code
	Fixed         bool
	DefaultTokens int
}

// ClaudeComponents returns the 13 Claude Code context components.
func ClaudeComponents() []ComponentDef {
	return []ComponentDef{
		{Key: "system", Label: "System prompt", Color: "#534AB7", TextColor: "#EEEDFE", Ansi: 56, Fixed: true, DefaultTokens: 3200},
		{Key: "tools", Label: "Built-in tools", Color: "#0F6E56", TextColor: "#E1F5EE", Ansi: 29, Fixed: true, DefaultTokens: 17000},
		{Key: "mcp", Label: "MCP tools", Color: "#D85A30", TextColor: "#FAECE7", Ansi: 166, Fixed: false, DefaultTokens: 0},
		{Key: "agents", Label: "Agents", Color: "#378ADD", TextColor: "#E6F1FB", Ansi: 33, Fixed: false, DefaultTokens: 0},
		{Key: "memory", Label: "Memory (CLAUDE.md)", Color: "#D4537E", TextColor: "#FBEAF0", Ansi: 162, Fixed: false, DefaultTokens: 0},
		{Key: "skill_meta", Label: "Skill frontmatter", Color: "#3B6D11", TextColor: "#EAF3DE", Ansi: 64, Fixed: false, DefaultTokens: 0},
		{Key: "skill_body", Label: "Skill body (active)", Color: "#97C459", TextColor: "#C0DD97", Ansi: 107, Fixed: false, DefaultTokens: 0},
		{Key: "plan", Label: "Plan / todo", Color: "#1D9E75", TextColor: "#9FE1CB", Ansi: 36, Fixed: false, DefaultTokens: 0},
		{Key: "user_msg", Label: "User messages", Color: "#BA7517", TextColor: "#FAEEDA", Ansi: 136, Fixed: false, DefaultTokens: 0},
		{Key: "tool_results", Label: "Tool call results", Color: "#EF9F27", TextColor: "#FAC775", Ansi: 214, Fixed: false, DefaultTokens: 0},
		{Key: "responses", Label: "AI responses", Color: "#854F0B", TextColor: "#EF9F27", Ansi: 94, Fixed: false, DefaultTokens: 0},
		{Key: "subagent", Label: "Subagent summaries", Color: "#85B7EB", TextColor: "#B5D4F4", Ansi: 110, Fixed: false, DefaultTokens: 0},
		{Key: "buffer", Label: "Compact buffer", Color: "#B4B2A9", TextColor: "#D3D1C7", Ansi: 249, Fixed: true, DefaultTokens: 45000},
	}
}

// CodexComponents returns the 12 Codex CLI context components.
func CodexComponents() []ComponentDef {
	return []ComponentDef{
		{Key: "instructions", Label: "Instructions", Color: "#534AB7", TextColor: "#EEEDFE", Ansi: 56, Fixed: true, DefaultTokens: 2500},
		{Key: "tools", Label: "Built-in tools", Color: "#0F6E56", TextColor: "#E1F5EE", Ansi: 29, Fixed: true, DefaultTokens: 8000},
		{Key: "mcp", Label: "MCP tools", Color: "#D85A30", TextColor: "#FAECE7", Ansi: 166, Fixed: false, DefaultTokens: 0},
		{Key: "agents", Label: "AGENTS.md / memories", Color: "#378ADD", TextColor: "#E6F1FB", Ansi: 33, Fixed: false, DefaultTokens: 0},
		{Key: "skills", Label: "Skills", Color: "#639922", TextColor: "#EAF3DE", Ansi: 64, Fixed: false, DefaultTokens: 0},
		{Key: "plan", Label: "Plan (update_plan)", Color: "#1D9E75", TextColor: "#9FE1CB", Ansi: 36, Fixed: false, DefaultTokens: 0},
		{Key: "user_msg", Label: "User messages", Color: "#BA7517", TextColor: "#FAEEDA", Ansi: 136, Fixed: false, DefaultTokens: 0},
		{Key: "tool_results", Label: "Tool call results", Color: "#EF9F27", TextColor: "#FAC775", Ansi: 214, Fixed: false, DefaultTokens: 0},
		{Key: "responses", Label: "Codex responses", Color: "#854F0B", TextColor: "#EF9F27", Ansi: 94, Fixed: false, DefaultTokens: 0},
		{Key: "subagent", Label: "Subagent summaries", Color: "#85B7EB", TextColor: "#B5D4F4", Ansi: 110, Fixed: false, DefaultTokens: 0},
		{Key: "reasoning", Label: "Reasoning tokens", Color: "#888780", TextColor: "#D3D1C7", Ansi: 245, Fixed: false, DefaultTokens: 0},
		{Key: "free", Label: "Free space", Color: "#E8E6DF", TextColor: "#F1EFE8", Ansi: 254, Fixed: false, DefaultTokens: 0},
	}
}

// Theme defines brand colors for HTML dashboard rendering.
type Theme struct {
	Name          string
	Brand500      string
	Brand600      string
	Surface0      string
	Surface1      string
	Surface2      string
	TextPrimary   string
	TextSecondary string
	BorderDefault string
}

// ClaudeTheme returns the Claude Code brand theme.
func ClaudeTheme() Theme {
	return Theme{
		Name:          "Claude Code",
		Brand500:      "#D4603A",
		Brand600:      "#B84A28",
		Surface0:      "#FFFFFF",
		Surface1:      "#F8F7F5",
		Surface2:      "#EEEDEB",
		TextPrimary:   "#252422",
		TextSecondary: "#6B6862",
		BorderDefault: "#D8D6D2",
	}
}

// CodexTheme returns the Codex CLI brand theme.
func CodexTheme() Theme {
	return Theme{
		Name:          "Codex CLI",
		Brand500:      "#10A37F",
		Brand600:      "#0D8A6A",
		Surface0:      "#FFFFFF",
		Surface1:      "#FAFAF9",
		Surface2:      "#F5F5F4",
		TextPrimary:   "#292524",
		TextSecondary: "#78716C",
		BorderDefault: "#E7E5E4",
	}
}

// EstimateTokens estimates token count from text using the ~4 chars/token heuristic.
func EstimateTokens(text string) int {
	if len(text) == 0 {
		return 0
	}
	return (len(text) + 3) / 4 // ceil(len/4)
}

// ANSI escape code helpers.

const (
	Reset = "\x1b[0m"
	Bold  = "\x1b[1m"
	Dim   = "\x1b[2m"
)

// AnsiBg returns an ANSI 256-color background escape sequence.
func AnsiBg(code int) string {
	return fmt.Sprintf("\x1b[48;5;%dm", code)
}

// AnsiFg returns an ANSI 256-color foreground escape sequence.
func AnsiFg(code int) string {
	return fmt.Sprintf("\x1b[38;5;%dm", code)
}
