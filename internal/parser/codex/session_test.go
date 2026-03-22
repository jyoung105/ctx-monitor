package codex

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func testdataPath(name string) string {
	_, filename, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(filename), "..", "..", "..", "testdata", "codex", name)
}

func TestParseSession_Simple(t *testing.T) {
	sess, err := ParseSession(testdataPath("rollout-simple.jsonl"))
	if err != nil {
		t.Fatalf("ParseSession returned error: %v", err)
	}

	if sess.SessionID != "codex-sess-1" {
		t.Errorf("sessionId: got %q, want %q", sess.SessionID, "codex-sess-1")
	}

	if sess.Model != "gpt-5.4" {
		t.Errorf("model: got %q, want %q", sess.Model, "gpt-5.4")
	}

	if sess.ContextWindowSize != 256000 {
		t.Errorf("contextWindowSize: got %d, want %d", sess.ContextWindowSize, 256000)
	}

	// Verify apply_patch tool call is present.
	hasPatch := false
	for _, tc := range sess.ToolCalls {
		if tc.Name == "apply_patch" {
			hasPatch = true
			break
		}
	}
	if !hasPatch {
		names := make([]string, 0, len(sess.ToolCalls))
		for _, tc := range sess.ToolCalls {
			names = append(names, tc.Name)
		}
		t.Errorf("expected tool call 'apply_patch', got: %v", names)
	}

	if sess.TokenUsage.Input == 0 && sess.TokenUsage.Output == 0 && sess.TokenUsage.Total == 0 {
		t.Error("tokenUsage should have non-zero values")
	}

	if sess.TokenBuckets.Responses == 0 && sess.TokenBuckets.UserMsg == 0 && sess.TokenBuckets.ToolResults == 0 {
		t.Error("token buckets should have non-zero values")
	}
}

func TestParseSessionSummary_SkipsHeavySlices(t *testing.T) {
	sess, err := ParseSessionSummary(testdataPath("rollout-simple.jsonl"))
	if err != nil {
		t.Fatalf("ParseSessionSummary returned error: %v", err)
	}

	if len(sess.ToolCalls) != 0 {
		t.Errorf("tool calls count: got %d, want 0 for summary parse", len(sess.ToolCalls))
	}
	if len(sess.ToolResults) != 0 {
		t.Errorf("tool results count: got %d, want 0 for summary parse", len(sess.ToolResults))
	}
	if len(sess.Turns) != 0 {
		t.Errorf("turns count: got %d, want 0 for summary parse", len(sess.Turns))
	}
	if sess.TokenBuckets.ToolResults == 0 {
		t.Error("expected tool result tokens to still be counted in summary parse")
	}
}

func TestFindAllSessions_UsesTTLCache(t *testing.T) {
	origTTL := sessionDiscoveryTTL
	origNow := sessionDiscoveryNow
	origCache := sessionDiscoveryCache
	sessionDiscoveryTTL = time.Hour
	currentTime := time.Date(2026, time.March, 22, 13, 0, 0, 0, time.UTC)
	sessionDiscoveryNow = func() time.Time { return currentTime }
	sessionDiscoveryCache = map[string]sessionDiscoveryCacheEntry{}
	t.Cleanup(func() {
		sessionDiscoveryTTL = origTTL
		sessionDiscoveryNow = origNow
		sessionDiscoveryCache = origCache
	})

	home := t.TempDir()
	t.Setenv("CODEX_HOME", home)

	writeSessionFile := func(rel string) {
		path := filepath.Join(home, "sessions", rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir session dir: %v", err)
		}
		data, err := os.ReadFile(testdataPath("rollout-simple.jsonl"))
		if err != nil {
			t.Fatalf("read fixture: %v", err)
		}
		if err := os.WriteFile(path, data, 0o644); err != nil {
			t.Fatalf("write session file: %v", err)
		}
	}

	writeSessionFile(filepath.Join("2026", "03", "22", "first.jsonl"))
	first, err := FindAllSessions()
	if err != nil {
		t.Fatalf("FindAllSessions first call: %v", err)
	}
	if len(first) != 1 {
		t.Fatalf("first count = %d, want 1", len(first))
	}

	writeSessionFile(filepath.Join("2026", "03", "22", "second.jsonl"))
	second, err := FindAllSessions()
	if err != nil {
		t.Fatalf("FindAllSessions cached call: %v", err)
	}
	if len(second) != 1 {
		t.Fatalf("cached count = %d, want 1 within TTL", len(second))
	}

	currentTime = currentTime.Add(2 * time.Hour)
	third, err := FindAllSessions()
	if err != nil {
		t.Fatalf("FindAllSessions after TTL: %v", err)
	}
	if len(third) != 2 {
		t.Fatalf("post-TTL count = %d, want 2", len(third))
	}
}

func TestParseSessionSummaryFromOffset_AppendsDelta(t *testing.T) {
	path := filepath.Join(t.TempDir(), "rollout.jsonl")
	data, err := os.ReadFile(testdataPath("rollout-simple.jsonl"))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write temp session: %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat session: %v", err)
	}

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

	delta, err := ParseSessionSummaryFromOffset(path, info.Size())
	if err != nil {
		t.Fatalf("ParseSessionSummaryFromOffset returned error: %v", err)
	}

	if delta.RawStats.LineCount != 3 {
		t.Fatalf("delta line count = %d, want 3", delta.RawStats.LineCount)
	}
	if delta.TokenBuckets.UserMsg == 0 {
		t.Fatal("expected appended user message tokens in delta")
	}
	if delta.TokenUsage.Total != 230 {
		t.Fatalf("delta total tokens = %d, want 230", delta.TokenUsage.Total)
	}
}
