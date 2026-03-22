package model

import (
	"strings"
	"testing"
)

func TestClaudeComponentsCount(t *testing.T) {
	comps := ClaudeComponents()
	if len(comps) != 13 {
		t.Errorf("ClaudeComponents() returned %d components, want 13", len(comps))
	}
}

func TestCodexComponentsCount(t *testing.T) {
	comps := CodexComponents()
	if len(comps) != 12 {
		t.Errorf("CodexComponents() returned %d components, want 12", len(comps))
	}
}

func TestEstimateTokensHelloWorld(t *testing.T) {
	// "hello world" is 11 chars; ceil(11/4) = 3
	got := EstimateTokens("hello world")
	if got != 3 {
		t.Errorf("EstimateTokens(\"hello world\") = %d, want 3", got)
	}
}

func TestEstimateTokensEmpty(t *testing.T) {
	got := EstimateTokens("")
	if got != 0 {
		t.Errorf("EstimateTokens(\"\") = %d, want 0", got)
	}
}

func TestEstimateTokens100Chars(t *testing.T) {
	s := strings.Repeat("a", 100)
	got := EstimateTokens(s)
	if got != 25 {
		t.Errorf("EstimateTokens(100-char string) = %d, want 25", got)
	}
}

func TestAnsiFg(t *testing.T) {
	got := AnsiFg(56)
	want := "\x1b[38;5;56m"
	if got != want {
		t.Errorf("AnsiFg(56) = %q, want %q", got, want)
	}
}

func TestAnsiBg(t *testing.T) {
	got := AnsiBg(214)
	want := "\x1b[48;5;214m"
	if got != want {
		t.Errorf("AnsiBg(214) = %q, want %q", got, want)
	}
}

func TestClaudeComponentsKeys(t *testing.T) {
	comps := ClaudeComponents()
	wantKeys := []string{
		"system", "tools", "mcp", "agents", "memory", "skill_meta",
		"skill_body", "plan", "user_msg", "tool_results", "responses",
		"subagent", "buffer",
	}
	for i, c := range comps {
		if c.Key != wantKeys[i] {
			t.Errorf("ClaudeComponents()[%d].Key = %q, want %q", i, c.Key, wantKeys[i])
		}
	}
}

func TestCodexComponentsKeys(t *testing.T) {
	comps := CodexComponents()
	wantKeys := []string{
		"instructions", "tools", "mcp", "agents", "skills", "plan",
		"user_msg", "tool_results", "responses", "subagent", "reasoning", "free",
	}
	for i, c := range comps {
		if c.Key != wantKeys[i] {
			t.Errorf("CodexComponents()[%d].Key = %q, want %q", i, c.Key, wantKeys[i])
		}
	}
}
