package codex

import (
	"path/filepath"
	"runtime"
	"testing"
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
