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

func TestParseCodexSessionForArgs_AppendsIncrementally(t *testing.T) {
	codexSessionCacheMu.Lock()
	codexSessionCache = map[sessionCacheKey]*model.CodexSession{}
	codexSessionCacheMu.Unlock()

	path := copyFixtureToTemp(t, benchmarkRepoPath("testdata", "codex", "rollout-simple.jsonl"))

	first, err := parseCodexSessionForArgs(path, cliArgs{})
	if err != nil {
		t.Fatalf("first parse: %v", err)
	}
	firstUserMsg := first.TokenBuckets.UserMsg
	firstLines := first.RawStats.LineCount

	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatalf("open for append: %v", err)
	}
	appendLines := "\n" +
		`{"type":"event_msg","payload":{"type":"user_message","content":"Follow up request","timestamp":"2025-03-19T10:00:10Z"}}` + "\n" +
		`{"type":"event_msg","payload":{"type":"token_count","input_tokens":200,"output_tokens":30,"total_tokens":230,"timestamp":"2025-03-19T10:00:11Z"}}`
	if _, err := f.WriteString(appendLines); err != nil {
		_ = f.Close()
		t.Fatalf("append lines: %v", err)
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
	if second.TokenBuckets.UserMsg <= firstUserMsg {
		t.Fatalf("user msg tokens = %d, want > %d after append", second.TokenBuckets.UserMsg, firstUserMsg)
	}
	if second.RawStats.LineCount != firstLines+3 {
		t.Fatalf("line count = %d, want %d", second.RawStats.LineCount, firstLines+3)
	}
	if second.TokenUsage.Total <= first.TokenUsage.Total {
		t.Fatalf("total tokens = %d, want > %d after append", second.TokenUsage.Total, first.TokenUsage.Total)
	}

	codexSessionCacheMu.Lock()
	defer codexSessionCacheMu.Unlock()
	if len(codexSessionCache) != 1 {
		t.Fatalf("codex session cache size = %d, want 1 after append merge", len(codexSessionCache))
	}
}

func TestParseCodexConfigCached_InvalidatesOnConfigChange(t *testing.T) {
	codexConfigCacheMu.Lock()
	codexConfigCache = map[codexConfigCacheKey]*model.CodexConfig{}
	codexConfigCacheMu.Unlock()

	home := t.TempDir()
	projectDir := filepath.Join(home, "project")
	if err := os.MkdirAll(filepath.Join(home, ".codex"), 0o755); err != nil {
		t.Fatalf("mkdir codex home: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(home, ".agents", "skills", "demo-skill"), 0o755); err != nil {
		t.Fatalf("mkdir skill dir: %v", err)
	}
	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		t.Fatalf("mkdir project dir: %v", err)
	}

	copyPairs := map[string]string{
		benchmarkRepoPath("testdata", "codex", "config.toml"): filepath.Join(home, ".codex", "config.toml"),
		benchmarkRepoPath("testdata", "codex", "AGENTS.md"):   filepath.Join(home, ".codex", "AGENTS.md"),
	}
	for src, dst := range copyPairs {
		data, err := os.ReadFile(src)
		if err != nil {
			t.Fatalf("read %s: %v", src, err)
		}
		if err := os.WriteFile(dst, data, 0o644); err != nil {
			t.Fatalf("write %s: %v", dst, err)
		}
	}
	if err := os.WriteFile(filepath.Join(home, ".agents", "skills", "demo-skill", "SKILL.md"), []byte("---\nname: demo\n---\n"), 0o644); err != nil {
		t.Fatalf("write skill: %v", err)
	}

	t.Setenv("HOME", home)

	first, err := parseCodexConfigCached(projectDir)
	if err != nil {
		t.Fatalf("first config parse: %v", err)
	}
	second, err := parseCodexConfigCached(projectDir)
	if err != nil {
		t.Fatalf("second config parse: %v", err)
	}
	if first != second {
		t.Fatal("expected cached config pointer on repeated parse")
	}

	cfgPath := filepath.Join(home, ".codex", "config.toml")
	f, err := os.OpenFile(cfgPath, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatalf("open config: %v", err)
	}
	if _, err := f.WriteString("\nmodel = \"gpt-5.4-mini\"\n"); err != nil {
		_ = f.Close()
		t.Fatalf("append config: %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("close config: %v", err)
	}
	now := time.Now().Add(time.Second)
	if err := os.Chtimes(cfgPath, now, now); err != nil {
		t.Fatalf("chtimes config: %v", err)
	}

	third, err := parseCodexConfigCached(projectDir)
	if err != nil {
		t.Fatalf("third config parse: %v", err)
	}
	if third == second {
		t.Fatal("expected config cache invalidation after config change")
	}
}

func TestParseClaudeConfigCached_UsesTTL(t *testing.T) {
	claudeConfigCacheMu.Lock()
	claudeConfigCache = map[string]claudeConfigCacheEntry{}
	claudeConfigCacheMu.Unlock()

	origTTL := claudeConfigCacheTTL
	origNow := claudeConfigNow
	claudeConfigCacheTTL = time.Hour
	currentTime := time.Date(2026, time.March, 22, 14, 0, 0, 0, time.UTC)
	claudeConfigNow = func() time.Time { return currentTime }
	t.Cleanup(func() {
		claudeConfigCacheTTL = origTTL
		claudeConfigNow = origNow
	})

	home := t.TempDir()
	projectDir := filepath.Join(home, "project")
	if err := os.MkdirAll(filepath.Join(home, ".claude"), 0o755); err != nil {
		t.Fatalf("mkdir claude home: %v", err)
	}
	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		t.Fatalf("mkdir project dir: %v", err)
	}

	settingsPath := filepath.Join(home, ".claude", "settings.json")
	if err := os.WriteFile(settingsPath, []byte(`{"statusLine":"compact"}`), 0o644); err != nil {
		t.Fatalf("write settings: %v", err)
	}
	t.Setenv("HOME", home)

	first := parseClaudeConfigCached(projectDir)
	second := parseClaudeConfigCached(projectDir)
	if first != second {
		t.Fatal("expected cached Claude config pointer within TTL")
	}

	if err := os.WriteFile(settingsPath, []byte(`{"statusLine":"full"}`), 0o644); err != nil {
		t.Fatalf("rewrite settings: %v", err)
	}
	third := parseClaudeConfigCached(projectDir)
	if third != second {
		t.Fatal("expected TTL cache to reuse config before expiry")
	}

	currentTime = currentTime.Add(2 * time.Hour)
	fourth := parseClaudeConfigCached(projectDir)
	if fourth == third {
		t.Fatal("expected TTL expiry to refresh Claude config")
	}
}

func TestBuildClaudeTimelineCached_InvalidatesOnFileChange(t *testing.T) {
	timelineCacheMu.Lock()
	timelineCache = map[timelineCacheKey]interface{}{}
	timelineCacheMu.Unlock()

	path := copyFixtureToTemp(t, benchmarkRepoPath("testdata", "claude", "session-simple.jsonl"))

	first, err := buildClaudeTimelineCached(path, "timeline-1")
	if err != nil {
		t.Fatalf("first timeline build: %v", err)
	}
	if first == nil {
		t.Fatal("expected first timeline payload")
	}
	second, err := buildClaudeTimelineCached(path, "timeline-1")
	if err != nil {
		t.Fatalf("second timeline build: %v", err)
	}
	if second == nil {
		t.Fatal("expected second timeline payload")
	}
	timelineCacheMu.Lock()
	if len(timelineCache) != 1 {
		timelineCacheMu.Unlock()
		t.Fatalf("timeline cache size = %d, want 1 after cache hit", len(timelineCache))
	}
	timelineCacheMu.Unlock()

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
		t.Fatalf("chtimes timeline file: %v", err)
	}

	third, err := buildClaudeTimelineCached(path, "timeline-1")
	if err != nil {
		t.Fatalf("third timeline build: %v", err)
	}
	if third == nil {
		t.Fatal("expected third timeline payload")
	}
	timelineCacheMu.Lock()
	defer timelineCacheMu.Unlock()
	if len(timelineCache) != 2 {
		t.Fatalf("timeline cache size = %d, want 2 after invalidation", len(timelineCache))
	}
}
