package claude

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/tonylee/ctx-monitor/internal/model"
)

const (
	TokensPerMCPTool      = 700
	DefaultToolsPerServer = 10
)

// knownMCPSizes maps well-known MCP server names to their estimated token costs.
var knownMCPSizes = map[string]int{
	"playwright": 14300,
	"sentry":     10000,
}

type memoryConfigCacheEntry struct {
	config     model.MemoryConfig
	expiresAt  time.Time
}

type skillFileCacheEntry struct {
	files      []model.SkillFile
	expiresAt  time.Time
}

var (
	claudeConfigParseNow = time.Now
	memoryConfigCacheMu  sync.Mutex
	memoryConfigCache    = map[string]memoryConfigCacheEntry{}
	memoryConfigCacheTTL = time.Second
	skillFileCacheMu     sync.Mutex
	skillFileCache       = map[string]skillFileCacheEntry{}
	skillFileCacheTTL    = time.Second
)

// ParseConfig reads Claude Code configuration from multiple sources and returns
// a unified ClaudeConfig. Returns nil on catastrophic failure, but tolerates
// individual file read errors gracefully.
func ParseConfig(projectPath string) *model.ClaudeConfig {
	home, err := os.UserHomeDir()
	if err != nil {
		home = ""
	}

	cfg := &model.ClaudeConfig{}

	// MCP
	cfg.MCP = parseMCPConfig(projectPath, home)

	// Memory files
	cfg.Memory = parseMemoryConfig(projectPath, home)

	// Agent files
	cfg.Agents = parseAgentsConfig(home)

	// Skill files
	cfg.Skills = parseSkillsConfig(projectPath, home)

	// Settings
	cfg.Settings = parseSettingsConfig(projectPath, home)

	// OAuth
	cfg.OAuth = parseOAuthConfig(home)

	return cfg
}

// ---------------------------------------------------------------------------
// MCP
// ---------------------------------------------------------------------------

type mcpSettingsFile struct {
	MCPServers map[string]json.RawMessage `json:"mcpServers"`
}

type mcpServerRaw struct {
	ToolCount int `json:"toolCount"`
}

func parseMCPConfig(projectPath, home string) model.MCPConfig {
	// Ordered candidate files: project sources first, then user global.
	candidates := []string{
		filepath.Join(projectPath, ".mcp.json"),
		filepath.Join(projectPath, ".claude", "settings.json"),
	}
	if home != "" {
		candidates = append(candidates,
			filepath.Join(home, ".claude", "settings.json"),
			filepath.Join(home, ".claude.json"),
		)
	}

	seen := map[string]bool{}
	var servers []model.MCPServerInfo

	for _, path := range candidates {
		parsed := readMCPSettingsFile(path)
		if parsed == nil {
			continue
		}
		for name, raw := range parsed.MCPServers {
			if seen[name] {
				continue
			}
			seen[name] = true

			tokens := estimateMCPTokens(name, raw)
			servers = append(servers, model.MCPServerInfo{
				Name:            name,
				EstimatedTokens: tokens,
			})
		}
	}

	total := 0
	for _, s := range servers {
		total += s.EstimatedTokens
	}

	return model.MCPConfig{
		Servers:     servers,
		TotalTokens: total,
	}
}

func estimateMCPTokens(name string, raw json.RawMessage) int {
	if known, ok := knownMCPSizes[strings.ToLower(name)]; ok {
		return known
	}

	var srv mcpServerRaw
	if err := json.Unmarshal(raw, &srv); err == nil && srv.ToolCount > 0 {
		return srv.ToolCount * TokensPerMCPTool
	}

	return DefaultToolsPerServer * TokensPerMCPTool
}

func readMCPSettingsFile(path string) *mcpSettingsFile {
	data := safeReadFile(path)
	if data == nil {
		return nil
	}
	var f mcpSettingsFile
	if err := json.Unmarshal(data, &f); err != nil {
		return nil
	}
	if len(f.MCPServers) == 0 {
		return nil
	}
	return &f
}

// ---------------------------------------------------------------------------
// Memory files
// ---------------------------------------------------------------------------

func parseMemoryConfig(projectPath, home string) model.MemoryConfig {
	cacheKey := home + "::" + projectPath
	now := claudeConfigParseNow()
	memoryConfigCacheMu.Lock()
	cached, ok := memoryConfigCache[cacheKey]
	memoryConfigCacheMu.Unlock()
	if ok && now.Before(cached.expiresAt) {
		files := append([]model.MemoryFile(nil), cached.config.Files...)
		return model.MemoryConfig{
			Files:       files,
			TotalTokens: cached.config.TotalTokens,
		}
	}

	var files []model.MemoryFile

	// Global CLAUDE.md
	if home != "" {
		if mf := readMemoryFile(filepath.Join(home, ".claude", "CLAUDE.md")); mf != nil {
			files = append(files, *mf)
		}
	}

	// Project CLAUDE.md
	if mf := readMemoryFile(filepath.Join(projectPath, "CLAUDE.md")); mf != nil {
		files = append(files, *mf)
	}

	// Subdirectory .claude/CLAUDE.md (skip node_modules)
	_ = filepath.WalkDir(projectPath, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			name := d.Name()
			if name == "node_modules" || name == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		// We want {projectPath}/**/.claude/CLAUDE.md but not the top-level one.
		if d.Name() == "CLAUDE.md" {
			parent := filepath.Base(filepath.Dir(path))
			if parent == ".claude" && path != filepath.Join(projectPath, "CLAUDE.md") {
				if mf := readMemoryFile(path); mf != nil {
					files = append(files, *mf)
				}
			}
		}
		return nil
	})

	total := 0
	for _, f := range files {
		total += f.Tokens
	}

	cfg := model.MemoryConfig{
		Files:       files,
		TotalTokens: total,
	}
	memoryConfigCacheMu.Lock()
	memoryConfigCache[cacheKey] = memoryConfigCacheEntry{
		config:    cfg,
		expiresAt: now.Add(memoryConfigCacheTTL),
	}
	memoryConfigCacheMu.Unlock()
	return cfg
}

func readMemoryFile(path string) *model.MemoryFile {
	data := safeReadFile(path)
	if data == nil {
		return nil
	}
	chars := len(data)
	return &model.MemoryFile{
		Path:   path,
		Chars:  chars,
		Tokens: chars / 4,
	}
}

// ---------------------------------------------------------------------------
// Agent files
// ---------------------------------------------------------------------------

func parseAgentsConfig(home string) model.AgentsConfig {
	if home == "" {
		return model.AgentsConfig{}
	}

	agentsDir := filepath.Join(home, ".claude", "agents")
	entries, err := os.ReadDir(agentsDir)
	if err != nil {
		return model.AgentsConfig{}
	}

	var files []model.AgentFile
	total := 0

	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		path := filepath.Join(agentsDir, e.Name())
		data := safeReadFile(path)
		if data == nil {
			continue
		}
		tokens := len(data) / 4
		total += tokens
		name := strings.TrimSuffix(e.Name(), ".md")
		files = append(files, model.AgentFile{
			Name:   name,
			Path:   path,
			Tokens: tokens,
		})
	}

	return model.AgentsConfig{
		Files:       files,
		TotalTokens: total,
	}
}

// ---------------------------------------------------------------------------
// Skill files
// ---------------------------------------------------------------------------

func parseSkillsConfig(projectPath, home string) model.SkillsConfig {
	var skills []model.SkillFile
	total := 0

	// Global skills
	if home != "" {
		globalSkills := findSkillFiles(filepath.Join(home, ".claude", "skills"))
		skills = append(skills, globalSkills...)
	}

	// Project skills
	projectSkills := findSkillFiles(filepath.Join(projectPath, ".claude", "skills"))
	skills = append(skills, projectSkills...)

	for _, s := range skills {
		total += s.FrontmatterTokens
	}

	return model.SkillsConfig{
		Installed:              skills,
		Count:                  len(skills),
		TotalFrontmatterTokens: total,
	}
}

func findSkillFiles(skillsDir string) []model.SkillFile {
	now := claudeConfigParseNow()
	skillFileCacheMu.Lock()
	cached, ok := skillFileCache[skillsDir]
	skillFileCacheMu.Unlock()
	if ok && now.Before(cached.expiresAt) {
		return append([]model.SkillFile(nil), cached.files...)
	}

	entries, err := os.ReadDir(skillsDir)
	if err != nil {
		return nil
	}

	var skills []model.SkillFile

	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		skillMD := filepath.Join(skillsDir, e.Name(), "SKILL.md")
		data := safeReadFile(skillMD)
		if data == nil {
			continue
		}
		fmTokens := parseFrontmatterTokens(data)
		skills = append(skills, model.SkillFile{
			Name:              e.Name(),
			Path:              skillMD,
			FrontmatterTokens: fmTokens,
		})
	}

	skillFileCacheMu.Lock()
	skillFileCache[skillsDir] = skillFileCacheEntry{
		files:     append([]model.SkillFile(nil), skills...),
		expiresAt: now.Add(skillFileCacheTTL),
	}
	skillFileCacheMu.Unlock()
	return skills
}

// parseFrontmatterTokens extracts the YAML frontmatter from a markdown file
// (content between the first pair of --- markers) and returns its token estimate.
func parseFrontmatterTokens(data []byte) int {
	content := string(data)
	if !strings.HasPrefix(content, "---") {
		return 0
	}
	rest := content[3:]
	// Find closing ---
	end := strings.Index(rest, "\n---")
	if end < 0 {
		return 0
	}
	frontmatter := rest[:end]
	return len(frontmatter) / 4
}

// ---------------------------------------------------------------------------
// Settings
// ---------------------------------------------------------------------------

type settingsFile struct {
	StatusLine  interface{}            `json:"statusLine"`
	Hooks       map[string]interface{} `json:"hooks"`
	Permissions struct {
		Allow []interface{} `json:"allow"`
		Deny  []interface{} `json:"deny"`
	} `json:"permissions"`
}

func parseSettingsConfig(projectPath, home string) model.SettingsConfig {
	var userSettings, projectSettings *settingsFile

	if home != "" {
		userSettings = readSettingsFile(filepath.Join(home, ".claude", "settings.json"))
	}
	projectSettings = readSettingsFile(filepath.Join(projectPath, ".claude", "settings.json"))

	cfg := model.SettingsConfig{
		Hooks: map[string]interface{}{},
		Permissions: model.PermissionsConfig{
			User:    []interface{}{},
			Project: []interface{}{},
		},
	}

	if userSettings != nil {
		if cfg.StatusLine == nil && userSettings.StatusLine != nil {
			cfg.StatusLine = userSettings.StatusLine
		}
		for k, v := range userSettings.Hooks {
			cfg.Hooks[k] = v
		}
		cfg.Permissions.User = userSettings.Permissions.Allow
	}

	if projectSettings != nil {
		if cfg.StatusLine == nil && projectSettings.StatusLine != nil {
			cfg.StatusLine = projectSettings.StatusLine
		}
		for k, v := range projectSettings.Hooks {
			cfg.Hooks[k] = v
		}
		cfg.Permissions.Project = projectSettings.Permissions.Allow
	}

	return cfg
}

func readSettingsFile(path string) *settingsFile {
	data := safeReadFile(path)
	if data == nil {
		return nil
	}
	var s settingsFile
	if err := json.Unmarshal(data, &s); err != nil {
		return nil
	}
	return &s
}

// ---------------------------------------------------------------------------
// OAuth
// ---------------------------------------------------------------------------

func parseOAuthConfig(home string) model.OAuthConfig {
	if home == "" {
		return model.OAuthConfig{}
	}
	data := safeReadFile(filepath.Join(home, ".claude.json"))
	if data == nil {
		return model.OAuthConfig{}
	}

	var raw map[string]interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		return model.OAuthConfig{}
	}

	tokenFields := []string{"oauthToken", "oauth_token", "accessToken", "access_token"}
	for _, field := range tokenFields {
		if v, ok := raw[field]; ok && v != nil && v != "" {
			return model.OAuthConfig{HasToken: true}
		}
	}

	return model.OAuthConfig{}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// safeReadFile reads a file and returns its contents, or nil on any error.
func safeReadFile(path string) []byte {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	return data
}
