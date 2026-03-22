package estimator

import (
	"testing"

	"github.com/tonylee/ctx-monitor/internal/model"
)

func nilClaudeSession() *model.ClaudeSession {
	return &model.ClaudeSession{}
}

func nilClaudeConfig() *model.ClaudeConfig {
	return &model.ClaudeConfig{}
}

func nilCodexSession() *model.CodexSession {
	return &model.CodexSession{}
}

func nilCodexConfig() *model.CodexConfig {
	return &model.CodexConfig{}
}

// TestEstimateClaudeContextNilInputs verifies the function returns a valid
// Composition with 13 components and tool="claude" when all inputs are empty.
func TestEstimateClaudeContextNilInputs(t *testing.T) {
	comp := EstimateClaudeContext(nilClaudeSession(), nilClaudeConfig(), nil)
	if comp == nil {
		t.Fatal("EstimateClaudeContext returned nil")
	}
	if comp.Tool != "claude" {
		t.Errorf("Tool = %q, want %q", comp.Tool, "claude")
	}
	if len(comp.Components) != 13 {
		t.Errorf("Components count = %d, want 13", len(comp.Components))
	}
	if comp.ContextWindowSize <= 0 {
		t.Errorf("ContextWindowSize = %d, want > 0", comp.ContextWindowSize)
	}
	if comp.Timestamp == "" {
		t.Error("Timestamp is empty")
	}
}

// TestEstimateClaudeContextWithBuckets verifies that token bucket values flow
// through to the correct components.
func TestEstimateClaudeContextWithBuckets(t *testing.T) {
	session := &model.ClaudeSession{
		Model: "claude-sonnet-4-6",
		TokenBuckets: model.TokenBuckets{
			UserMsg:     5000,
			ToolResults: 3000,
			Responses:   8000,
		},
	}
	comp := EstimateClaudeContext(session, nilClaudeConfig(), nil)
	if comp == nil {
		t.Fatal("EstimateClaudeContext returned nil")
	}

	byKey := make(map[string]model.Component)
	for _, c := range comp.Components {
		byKey[c.Key] = c
	}

	if byKey["user_msg"].Tokens != 5000 {
		t.Errorf("user_msg tokens = %d, want 5000", byKey["user_msg"].Tokens)
	}
	if byKey["tool_results"].Tokens != 3000 {
		t.Errorf("tool_results tokens = %d, want 3000", byKey["tool_results"].Tokens)
	}
	if byKey["responses"].Tokens != 8000 {
		t.Errorf("responses tokens = %d, want 8000", byKey["responses"].Tokens)
	}
}

// TestEstimateCodexContextNilInputs verifies 12 components and tool="codex".
func TestEstimateCodexContextNilInputs(t *testing.T) {
	comp := EstimateCodexContext(nilCodexSession(), nilCodexConfig())
	if comp == nil {
		t.Fatal("EstimateCodexContext returned nil")
	}
	if comp.Tool != "codex" {
		t.Errorf("Tool = %q, want %q", comp.Tool, "codex")
	}
	if len(comp.Components) != 12 {
		t.Errorf("Components count = %d, want 12", len(comp.Components))
	}
	if comp.ContextWindowSize <= 0 {
		t.Errorf("ContextWindowSize = %d, want > 0", comp.ContextWindowSize)
	}
}

// TestSimulateUsageZero checks that at 0%, all scalable components are zero.
func TestSimulateUsageZero(t *testing.T) {
	base := EstimateClaudeContext(nilClaudeSession(), nilClaudeConfig(), nil)
	sim := SimulateUsage(base, 0)
	if sim == nil {
		t.Fatal("SimulateUsage returned nil")
	}
	for _, c := range sim.Components {
		if !c.Fixed && c.Key != "buffer" && c.Tokens != 0 {
			t.Errorf("component %q: tokens = %d, want 0 at 0%% usage", c.Key, c.Tokens)
		}
	}
}

// TestSimulateUsage50 checks that TotalUsedPct is approximately 50.
func TestSimulateUsage50(t *testing.T) {
	base := EstimateClaudeContext(nilClaudeSession(), nilClaudeConfig(), nil)
	// Give scalable components some tokens so proportional scaling works.
	session := &model.ClaudeSession{
		Model: "claude-sonnet-4-6",
		TokenBuckets: model.TokenBuckets{
			UserMsg:     10000,
			ToolResults: 5000,
			Responses:   15000,
		},
	}
	base = EstimateClaudeContext(session, nilClaudeConfig(), nil)
	sim := SimulateUsage(base, 50)
	if sim == nil {
		t.Fatal("SimulateUsage returned nil")
	}
	if sim.TotalUsedPct < 45 || sim.TotalUsedPct > 55 {
		t.Errorf("TotalUsedPct = %.2f, want ~50", sim.TotalUsedPct)
	}
}

// TestSimulateUsage100 checks that TotalUsedTokens is close to ContextWindowSize.
func TestSimulateUsage100(t *testing.T) {
	session := &model.ClaudeSession{
		Model: "claude-sonnet-4-6",
		TokenBuckets: model.TokenBuckets{
			UserMsg:     10000,
			ToolResults: 5000,
			Responses:   15000,
		},
	}
	base := EstimateClaudeContext(session, nilClaudeConfig(), nil)
	sim := SimulateUsage(base, 100)
	if sim == nil {
		t.Fatal("SimulateUsage returned nil")
	}
	windowSize := sim.ContextWindowSize
	// Allow a small rounding tolerance of 1%
	tolerance := windowSize / 100
	if sim.TotalUsedTokens < windowSize-tolerance {
		t.Errorf("TotalUsedTokens = %d, want close to %d", sim.TotalUsedTokens, windowSize)
	}
}

// TestComponentPercentagesSumToAtMost100 verifies component pcts don't exceed window.
func TestComponentPercentagesSumToAtMost100(t *testing.T) {
	session := &model.ClaudeSession{
		Model: "claude-sonnet-4-6",
		TokenBuckets: model.TokenBuckets{
			UserMsg:     10000,
			ToolResults: 5000,
			Responses:   15000,
		},
	}
	comp := EstimateClaudeContext(session, nilClaudeConfig(), nil)
	var sum float64
	for _, c := range comp.Components {
		sum += c.Pct
	}
	if sum > 101.0 { // allow small floating-point rounding
		t.Errorf("Component percentages sum = %.2f, want <= 100", sum)
	}
}
