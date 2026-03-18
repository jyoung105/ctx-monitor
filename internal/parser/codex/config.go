package codex

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/tonylee/ctx-monitor/internal/model"
	"github.com/tonylee/ctx-monitor/internal/parser/toml"
)

// codexHome returns the Codex CLI home directory (~/.codex).
func codexHome() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".codex")
}

// readFile reads a file and returns its contents, or empty string on error.
func readFile(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return string(data)
}

// mergeConfigs merges project config over global config.
// Top-level keys are overridden by project values. mcp_servers and agents
// tables are merged at the server/agent level.
func mergeConfigs(global, project map[string]interface{}) map[string]interface{} {
	merged := make(map[string]interface{})

	// Copy global first.
	for k, v := range global {
		merged[k] = v
	}

	// Override/merge with project values.
	for k, v := range project {
		switch k {
		case "mcp_servers", "agents":
			// Merge at the server/agent level.
			globalTable, _ := merged[k].(map[string]interface{})
			projectTable, _ := v.(map[string]interface{})
			mergedTable := make(map[string]interface{})
			for sk, sv := range globalTable {
				mergedTable[sk] = sv
			}
			for sk, sv := range projectTable {
				mergedTable[sk] = sv
			}
			merged[k] = mergedTable
		default:
			merged[k] = v
		}
	}

	return merged
}

// extractStringSlice extracts a []string from an interface{} value.
func extractStringSlice(v interface{}) []string {
	arr, ok := v.([]interface{})
	if !ok {
		return nil
	}
	result := make([]string, 0, len(arr))
	for _, item := range arr {
		if s, ok := item.(string); ok {
			result = append(result, s)
		}
	}
	return result
}

// extractString extracts a string from an interface{} value.
func extractString(m map[string]interface{}, key string) string {
	v, ok := m[key]
	if !ok {
		return ""
	}
	s, _ := v.(string)
	return s
}

// serverToJSON returns a JSON representation of a server map for token estimation.
func serverToJSON(serverDef map[string]interface{}) string {
	data, err := json.Marshal(serverDef)
	if err != nil {
		return ""
	}
	return string(data)
}

// ParseConfig parses Codex CLI configuration from global and project paths,
// returning a populated *model.CodexConfig.
func ParseConfig(projectPath string) (*model.CodexConfig, error) {
	home := codexHome()

	// Read global and project TOML configs.
	var globalCfg map[string]interface{}
	var projectCfg map[string]interface{}

	if home != "" {
		globalText := readFile(filepath.Join(home, "config.toml"))
		if globalText != "" {
			globalCfg = toml.Parse(globalText)
		}
	}

	if projectPath != "" {
		projectText := readFile(filepath.Join(projectPath, ".codex", "config.toml"))
		if projectText != "" {
			projectCfg = toml.Parse(projectText)
		}
	}

	if globalCfg == nil {
		globalCfg = make(map[string]interface{})
	}
	if projectCfg == nil {
		projectCfg = make(map[string]interface{})
	}

	merged := mergeConfigs(globalCfg, projectCfg)

	cfg := &model.CodexConfig{}

	// Extract top-level config values.
	cfg.Model = extractString(merged, "model")
	cfg.ReasoningEffort = extractString(merged, "reasoning_effort")
	if v, ok := merged["compaction_threshold"]; ok {
		cfg.CompactionThreshold = v
	}

	// Extract MCP servers.
	var mcpServers []model.CodexMCPServer
	mcpTotalTokens := 0
	if mcpTable, ok := merged["mcp_servers"].(map[string]interface{}); ok {
		for name, serverVal := range mcpTable {
			serverDef, ok := serverVal.(map[string]interface{})
			if !ok {
				continue
			}
			server := model.CodexMCPServer{
				Name:    name,
				URL:     extractString(serverDef, "url"),
				Command: extractString(serverDef, "command"),
			}
			if v, ok := serverDef["args"]; ok {
				server.Args = extractStringSlice(v)
			}
			if v, ok := serverDef["enabled_tools"]; ok {
				server.EnabledTools = extractStringSlice(v)
			}
			if v, ok := serverDef["disabled_tools"]; ok {
				server.DisabledTools = extractStringSlice(v)
			}
			server.EstimatedTokens = model.EstimateTokens(serverToJSON(serverDef))
			mcpTotalTokens += server.EstimatedTokens
			mcpServers = append(mcpServers, server)
		}
	}
	cfg.MCP = model.CodexMCPConfig{
		Servers:     mcpServers,
		TotalTokens: mcpTotalTokens,
	}

	// Extract agents.
	var agentDefs []model.CodexAgentDef
	agentsTotalTokens := 0
	if agentsTable, ok := merged["agents"].(map[string]interface{}); ok {
		for name, agentVal := range agentsTable {
			agentDef, ok := agentVal.(map[string]interface{})
			if !ok {
				continue
			}
			agent := model.CodexAgentDef{
				Name:        name,
				Description: extractString(agentDef, "description"),
				Model:       extractString(agentDef, "model"),
			}
			agentJSON, _ := json.Marshal(agentDef)
			agentsTotalTokens += model.EstimateTokens(string(agentJSON))
			agentDefs = append(agentDefs, agent)
		}
	}
	cfg.Agents = model.CodexAgentsConfig{
		Definitions: agentDefs,
		TotalTokens: agentsTotalTokens,
	}

	// Read AGENTS.md instructions.
	if home != "" {
		agentsMdPath := filepath.Join(home, "AGENTS.md")
		content := readFile(agentsMdPath)
		if content != "" {
			cfg.Instructions = model.InstructionsInfo{
				Path:   agentsMdPath,
				Chars:  len(content),
				Tokens: model.EstimateTokens(content),
			}
		}
	}

	// Discover skills from ~/.agents/skills/*/SKILL.md and {projectPath}/.agents/skills/*/SKILL.md
	var skillFiles []model.CodexSkillFile
	skillsTotalTokens := 0

	skillDirs := []string{}
	if home != "" {
		userHome, err := os.UserHomeDir()
		if err == nil {
			skillDirs = append(skillDirs, filepath.Join(userHome, ".agents", "skills"))
		}
	}
	if projectPath != "" {
		skillDirs = append(skillDirs, filepath.Join(projectPath, ".agents", "skills"))
	}

	for _, skillsDir := range skillDirs {
		entries, err := os.ReadDir(skillsDir)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			skillMdPath := filepath.Join(skillsDir, entry.Name(), "SKILL.md")
			content := readFile(skillMdPath)
			if content == "" {
				continue
			}
			tokens := model.EstimateTokens(content)
			skillFiles = append(skillFiles, model.CodexSkillFile{
				Name:   entry.Name(),
				Path:   skillMdPath,
				Chars:  len(content),
				Tokens: tokens,
			})
			skillsTotalTokens += tokens
		}
	}

	cfg.Skills = model.CodexSkillsConfig{
		Files:       skillFiles,
		Count:       len(skillFiles),
		TotalTokens: skillsTotalTokens,
	}

	return cfg, nil
}
