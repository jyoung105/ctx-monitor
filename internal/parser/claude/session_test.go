package claude

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/tonylee/ctx-monitor/internal/model"
)

func testdataPath(name string) string {
	_, filename, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(filename), "..", "..", "..", "testdata", "claude", name)
}

func TestParseSession_Simple(t *testing.T) {
	sess, err := ParseSession(testdataPath("session-simple.jsonl"))
	if err != nil {
		t.Fatalf("ParseSession returned error: %v", err)
	}

	if sess.SessionID != "abc-123" {
		t.Errorf("sessionId: got %q, want %q", sess.SessionID, "abc-123")
	}

	if !strings.Contains(sess.Model, "sonnet") {
		t.Errorf("model %q does not contain 'sonnet'", sess.Model)
	}

	if len(sess.Messages) < 4 {
		t.Errorf("message count: got %d, want >= 4", len(sess.Messages))
	}

	names := toolCallNameSet(sess.ToolCalls)
	if !names["Read"] {
		t.Errorf("expected tool call 'Read', got: %v", toolCallNameList(sess.ToolCalls))
	}
	if !names["Edit"] {
		t.Errorf("expected tool call 'Edit', got: %v", toolCallNameList(sess.ToolCalls))
	}

	hasUserTurn := false
	for _, msg := range sess.Messages {
		if msg.Role == "user" {
			hasUserTurn = true
			break
		}
	}
	if !hasUserTurn {
		t.Error("expected at least one user turn in messages")
	}

	if sess.TokenBuckets.UserMsg == 0 {
		t.Error("TokenBuckets.UserMsg should be non-zero")
	}
	if sess.TokenBuckets.Responses == 0 {
		t.Error("TokenBuckets.Responses should be non-zero")
	}
}

func TestParseSession_Compaction(t *testing.T) {
	sess, err := ParseSession(testdataPath("session-compaction.jsonl"))
	if err != nil {
		t.Fatalf("ParseSession returned error: %v", err)
	}

	if sess.CompactionEvents.Count < 1 {
		t.Errorf("compaction events count: got %d, want >= 1", sess.CompactionEvents.Count)
	}

	if len(sess.SkillActivations) < 1 {
		t.Errorf("skill activations: got %d, want >= 1", len(sess.SkillActivations))
	}

	if len(sess.SubagentSpawns) < 1 {
		t.Errorf("subagent spawns: got %d, want >= 1", len(sess.SubagentSpawns))
	}

	hasTodoWrite := false
	for _, pe := range sess.PlanUsage {
		if pe.Tool == "TodoWrite" {
			hasTodoWrite = true
			break
		}
	}
	if !hasTodoWrite {
		planTools := make([]string, 0, len(sess.PlanUsage))
		for _, pe := range sess.PlanUsage {
			planTools = append(planTools, pe.Tool)
		}
		t.Errorf("expected a TodoWrite plan event, got: %v", planTools)
	}

	if sess.ThinkingStats.Count < 1 {
		t.Errorf("thinking blocks count: got %d, want >= 1", sess.ThinkingStats.Count)
	}
}

func TestParseSession_Continuation(t *testing.T) {
	sess, err := ParseSession(testdataPath("session-continuation.jsonl"))
	if err != nil {
		t.Fatalf("ParseSession returned error: %v", err)
	}

	if !sess.Continuation {
		t.Error("expected isContinuation=true (sessionId in file differs from filename)")
	}
}

func TestParseSessionSummary_RetainsHotPathFields(t *testing.T) {
	sess, err := ParseSessionSummary(testdataPath("session-simple.jsonl"))
	if err != nil {
		t.Fatalf("ParseSessionSummary returned error: %v", err)
	}

	if len(sess.Messages) != 0 {
		t.Errorf("messages count: got %d, want 0 for summary parse", len(sess.Messages))
	}
	if len(sess.Attachments) != 0 {
		t.Errorf("attachments count: got %d, want 0 for summary parse", len(sess.Attachments))
	}
	if len(sess.Turns) == 0 {
		t.Fatal("expected turns to be retained in summary parse")
	}
	if len(sess.ToolCalls) == 0 {
		t.Fatal("expected tool calls to be retained in summary parse")
	}

	var edit *model.ToolCall
	for i := range sess.ToolCalls {
		if sess.ToolCalls[i].Name == "Edit" {
			edit = &sess.ToolCalls[i]
			break
		}
	}
	if edit == nil {
		t.Fatal("expected Edit tool call in summary parse")
	}
	if edit.FilePath != "/src/main.go" {
		t.Errorf("FilePath = %q, want %q", edit.FilePath, "/src/main.go")
	}
	inputMap, ok := edit.Input.(map[string]interface{})
	if !ok {
		t.Fatalf("Edit input type = %T, want map[string]interface{}", edit.Input)
	}
	if _, exists := inputMap["old_string"]; exists {
		t.Errorf("summary input unexpectedly retained old_string: %+v", inputMap)
	}
	if _, exists := inputMap["new_string"]; exists {
		t.Errorf("summary input unexpectedly retained new_string: %+v", inputMap)
	}
	if inputMap["file_path"] != "/src/main.go" {
		t.Errorf("summary input file_path = %v, want %q", inputMap["file_path"], "/src/main.go")
	}
}

func TestParseSessionTimeline_RetainsOnlyMessages(t *testing.T) {
	sess, err := ParseSessionTimeline(testdataPath("session-simple.jsonl"))
	if err != nil {
		t.Fatalf("ParseSessionTimeline returned error: %v", err)
	}

	if len(sess.Messages) == 0 {
		t.Fatal("expected timeline messages to be retained")
	}
	if len(sess.ToolCalls) != 0 {
		t.Errorf("tool calls count: got %d, want 0", len(sess.ToolCalls))
	}
	if len(sess.Attachments) != 0 {
		t.Errorf("attachments count: got %d, want 0", len(sess.Attachments))
	}
	if len(sess.Turns) != 0 {
		t.Errorf("turns count: got %d, want 0", len(sess.Turns))
	}
}

func TestFindProjectDir_UsesTTLCache(t *testing.T) {
	origTTL := projectDirCacheTTL
	origNow := claudeCacheNow
	origCache := projectDirCache
	projectDirCacheTTL = time.Hour
	currentTime := time.Date(2026, time.March, 22, 15, 0, 0, 0, time.UTC)
	claudeCacheNow = func() time.Time { return currentTime }
	projectDirCache = map[string]projectDirCacheEntry{}
	t.Cleanup(func() {
		projectDirCacheTTL = origTTL
		claudeCacheNow = origNow
		projectDirCache = origCache
	})

	home := t.TempDir()
	base := filepath.Join(home, ".claude", "projects")
	if err := os.MkdirAll(base, 0o755); err != nil {
		t.Fatalf("mkdir base: %v", err)
	}
	cwd := "/Users/tester/project-a"
	projectName := strings.ReplaceAll(cwd, "/", "-")
	resolved := filepath.Join(base, projectName)
	if err := os.MkdirAll(resolved, 0o755); err != nil {
		t.Fatalf("mkdir project dir: %v", err)
	}
	t.Setenv("HOME", home)

	first := FindProjectDir(cwd)
	if first != resolved {
		t.Fatalf("first result = %q, want %q", first, resolved)
	}

	if err := os.RemoveAll(resolved); err != nil {
		t.Fatalf("remove project dir: %v", err)
	}
	second := FindProjectDir(cwd)
	if second != resolved {
		t.Fatalf("cached result = %q, want %q within TTL", second, resolved)
	}

	currentTime = currentTime.Add(2 * time.Hour)
	third := FindProjectDir(cwd)
	if third != "" {
		t.Fatalf("post-TTL result = %q, want empty after removal", third)
	}
}

func TestFindAllSessions_UsesTTLCache(t *testing.T) {
	origTTL := sessionListCacheTTL
	origNow := claudeCacheNow
	origCache := sessionListCache
	sessionListCacheTTL = time.Hour
	currentTime := time.Date(2026, time.March, 22, 15, 30, 0, 0, time.UTC)
	claudeCacheNow = func() time.Time { return currentTime }
	sessionListCache = map[string]sessionListCacheEntry{}
	t.Cleanup(func() {
		sessionListCacheTTL = origTTL
		claudeCacheNow = origNow
		sessionListCache = origCache
	})

	projectDir := t.TempDir()
	writeSession := func(name string) {
		data, err := os.ReadFile(testdataPath("session-simple.jsonl"))
		if err != nil {
			t.Fatalf("read fixture: %v", err)
		}
		if err := os.WriteFile(filepath.Join(projectDir, name), data, 0o644); err != nil {
			t.Fatalf("write session: %v", err)
		}
	}

	writeSession("first.jsonl")
	first := FindAllSessions(projectDir)
	if len(first) != 1 {
		t.Fatalf("first count = %d, want 1", len(first))
	}

	writeSession("second.jsonl")
	second := FindAllSessions(projectDir)
	if len(second) != 1 {
		t.Fatalf("cached count = %d, want 1 within TTL", len(second))
	}

	currentTime = currentTime.Add(2 * time.Hour)
	third := FindAllSessions(projectDir)
	if len(third) != 2 {
		t.Fatalf("post-TTL count = %d, want 2", len(third))
	}
}

func toolCallNameSet(tcs []model.ToolCall) map[string]bool {
	set := make(map[string]bool, len(tcs))
	for _, tc := range tcs {
		set[tc.Name] = true
	}
	return set
}

func toolCallNameList(tcs []model.ToolCall) []string {
	names := make([]string, 0, len(tcs))
	for _, tc := range tcs {
		names = append(names, tc.Name)
	}
	return names
}
