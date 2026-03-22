package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/tonylee/ctx-monitor/internal/model"
)

func copyFixtureToTemp(t *testing.T, srcPath string) string {
	t.Helper()

	data, err := os.ReadFile(srcPath)
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	path := filepath.Join(t.TempDir(), filepath.Base(srcPath))
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	return path
}

func TestParseClaudeSessionForArgs_UsesCache(t *testing.T) {
	claudeSessionCacheMu.Lock()
	claudeSessionCache = map[sessionCacheKey]*model.ClaudeSession{}
	claudeSessionCacheMu.Unlock()

	path := copyFixtureToTemp(t, benchmarkRepoPath("testdata", "claude", "session-simple.jsonl"))

	first, err := parseClaudeSessionForArgs(path, cliArgs{})
	if err != nil {
		t.Fatalf("first parse: %v", err)
	}
	second, err := parseClaudeSessionForArgs(path, cliArgs{})
	if err != nil {
		t.Fatalf("second parse: %v", err)
	}
	if first != second {
		t.Fatal("expected cached Claude session pointer on repeated parse")
	}
}

func TestParseCodexSessionForArgs_InvalidatesOnFileChange(t *testing.T) {
	codexSessionCacheMu.Lock()
	codexSessionCache = map[sessionCacheKey]*model.CodexSession{}
	codexSessionCacheMu.Unlock()

	path := copyFixtureToTemp(t, benchmarkRepoPath("testdata", "codex", "rollout-simple.jsonl"))

	first, err := parseCodexSessionForArgs(path, cliArgs{})
	if err != nil {
		t.Fatalf("first parse: %v", err)
	}

	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatalf("open for append: %v", err)
	}
	if _, err := f.WriteString("\n"); err != nil {
		_ = f.Close()
		t.Fatalf("append newline: %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("close file: %v", err)
	}
	now := time.Now().Add(time.Second)
	if err := os.Chtimes(path, now, now); err != nil {
		t.Fatalf("chtimes: %v", err)
	}

	second, err := parseCodexSessionForArgs(path, cliArgs{})
	if err != nil {
		t.Fatalf("second parse: %v", err)
	}
	if first == second {
		t.Fatal("expected cache invalidation after file change")
	}
}
