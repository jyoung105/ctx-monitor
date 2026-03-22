package claude

import (
	"path/filepath"
	"runtime"
	"strings"
	"testing"

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
