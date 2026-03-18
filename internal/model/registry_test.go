package model

import (
	"strings"
	"testing"
)

func TestResolveModelSonnet(t *testing.T) {
	m := ResolveModel("claude-sonnet-4-6")
	if m.Display != "Claude Sonnet 4.6" {
		t.Errorf("Display = %q, want %q", m.Display, "Claude Sonnet 4.6")
	}
	if m.Context != 200000 {
		t.Errorf("Context = %d, want 200000", m.Context)
	}
	if m.Family != "claude" {
		t.Errorf("Family = %q, want %q", m.Family, "claude")
	}
}

func TestResolveModelOpus1M(t *testing.T) {
	m := ResolveModel("claude-opus-4-6[1m]")
	if m.Context != 1000000 {
		t.Errorf("Context = %d, want 1000000", m.Context)
	}
}

func TestResolveModelFast(t *testing.T) {
	m := ResolveModel("claude-sonnet-4-6/fast")
	if !m.IsFast {
		t.Errorf("IsFast = false, want true")
	}
	if !strings.Contains(m.Display, "Fast") {
		t.Errorf("Display %q does not contain \"Fast\"", m.Display)
	}
}

func TestResolveModelGPT54(t *testing.T) {
	m := ResolveModel("gpt-5.4")
	if m.Family != "codex" {
		t.Errorf("Family = %q, want %q", m.Family, "codex")
	}
	if m.Context != 256000 {
		t.Errorf("Context = %d, want 256000", m.Context)
	}
}

func TestResolveModelEmpty(t *testing.T) {
	m := ResolveModel("")
	if m.Display != "Unknown" {
		t.Errorf("Display = %q, want %q", m.Display, "Unknown")
	}
}

func TestResolveModelUnknown(t *testing.T) {
	m := ResolveModel("some-unknown-model")
	if m.Family != "unknown" {
		t.Errorf("Family = %q, want %q", m.Family, "unknown")
	}
}

func TestResolveModelExactMatch(t *testing.T) {
	m := ResolveModel("claude-sonnet-4-6-20250514")
	if m.Display != "Claude Sonnet 4.6" {
		t.Errorf("Display = %q, want %q", m.Display, "Claude Sonnet 4.6")
	}
	if m.Family != "claude" {
		t.Errorf("Family = %q, want %q", m.Family, "claude")
	}
}

func TestResolveModelIsFastFalseByDefault(t *testing.T) {
	m := ResolveModel("claude-sonnet-4-6")
	if m.IsFast {
		t.Errorf("IsFast = true, want false for non-fast model")
	}
}
