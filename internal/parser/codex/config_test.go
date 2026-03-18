package codex

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseConfig(t *testing.T) {
	// Build a temp directory that mimics a ~/.codex home structure.
	// ParseConfig reads from codexHome() which returns ~/.codex.
	// We instead call ParseConfig with a projectPath that contains
	// a .codex/config.toml, and separately test via a custom home.
	//
	// Since ParseConfig hard-codes codexHome() to ~/.codex, we set up
	// the config via a temp project directory using .codex/config.toml.

	tmpDir := t.TempDir()

	// Create .codex/config.toml inside the temp project dir.
	codexDir := filepath.Join(tmpDir, ".codex")
	if err := os.MkdirAll(codexDir, 0755); err != nil {
		t.Fatalf("creating .codex dir: %v", err)
	}

	// Copy fixture config.toml into the project .codex directory.
	fixtureConfig := readTestFile(t, testdataPath("config.toml"))
	if err := os.WriteFile(filepath.Join(codexDir, "config.toml"), []byte(fixtureConfig), 0644); err != nil {
		t.Fatalf("writing config.toml: %v", err)
	}

	// Copy AGENTS.md into a temp codex home and override the home via env.
	tmpHome := t.TempDir()
	agentsMd := readTestFile(t, testdataPath("AGENTS.md"))
	if err := os.WriteFile(filepath.Join(tmpHome, "AGENTS.md"), []byte(agentsMd), 0644); err != nil {
		t.Fatalf("writing AGENTS.md: %v", err)
	}
	// Also put config.toml in home (global) so codexHome() finds it.
	if err := os.WriteFile(filepath.Join(tmpHome, "config.toml"), []byte(fixtureConfig), 0644); err != nil {
		t.Fatalf("writing global config.toml: %v", err)
	}

	// Override CODEX_HOME so FindCodexHome() returns our temp dir.
	t.Setenv("CODEX_HOME", tmpHome)

	cfg, err := ParseConfig(tmpDir)
	if err != nil {
		t.Fatalf("ParseConfig returned error: %v", err)
	}

	if cfg.Model != "gpt-5.4" {
		t.Errorf("model: got %q, want %q", cfg.Model, "gpt-5.4")
	}

	if cfg.ReasoningEffort != "high" {
		t.Errorf("reasoningEffort: got %q, want %q", cfg.ReasoningEffort, "high")
	}

	// Verify MCP servers include "filesystem".
	hasFilesystem := false
	for _, srv := range cfg.MCP.Servers {
		if srv.Name == "filesystem" {
			hasFilesystem = true
			break
		}
	}
	if !hasFilesystem {
		names := make([]string, 0, len(cfg.MCP.Servers))
		for _, srv := range cfg.MCP.Servers {
			names = append(names, srv.Name)
		}
		t.Errorf("expected MCP server 'filesystem', got: %v", names)
	}

	// Verify agent definitions include "code_reviewer".
	hasCodeReviewer := false
	for _, agent := range cfg.Agents.Definitions {
		if agent.Name == "code_reviewer" {
			hasCodeReviewer = true
			break
		}
	}
	if !hasCodeReviewer {
		names := make([]string, 0, len(cfg.Agents.Definitions))
		for _, agent := range cfg.Agents.Definitions {
			names = append(names, agent.Name)
		}
		t.Errorf("expected agent definition 'code_reviewer', got: %v", names)
	}
}

func readTestFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading test file %s: %v", path, err)
	}
	return string(data)
}
