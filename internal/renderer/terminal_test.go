package renderer

import (
	"strings"
	"testing"

	"github.com/tonylee/ctx-monitor/internal/model"
)

// makeComposition builds a minimal Composition for renderer tests.
func makeComposition(used, window int) *model.Composition {
	defs := model.ClaudeComponents()
	comps := make([]model.Component, len(defs))
	for i, d := range defs {
		comps[i] = model.Component{
			Key:    d.Key,
			Label:  d.Label,
			Tokens: d.DefaultTokens,
			Fixed:  d.Fixed,
			Color:  d.Color,
		}
	}
	return &model.Composition{
		Tool:              "claude",
		Model:             "Claude Sonnet 4.6",
		ContextWindowSize: window,
		TotalUsedTokens:   used,
		APIMatchTokens:    used,
		Components:        comps,
	}
}

// ---------------------------------------------------------------------------
// FormatTokens
// ---------------------------------------------------------------------------

func TestFormatTokens(t *testing.T) {
	cases := []struct {
		input int
		want  string
	}{
		{0, "0"},
		{500, "500"},
		{1000, "1k"},
		{1500, "1.5k"},
		{1000000, "1M"},
		{1500000, "1.5M"},
	}
	for _, tc := range cases {
		got := FormatTokens(tc.input)
		if got != tc.want {
			t.Errorf("FormatTokens(%d) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

// ---------------------------------------------------------------------------
// FormatPct
// ---------------------------------------------------------------------------

func TestFormatPct(t *testing.T) {
	cases := []struct {
		input float64
		want  string
	}{
		{0, "0.0%"},
		{52.3, "52.3%"},
		{100, "100.0%"},
	}
	for _, tc := range cases {
		got := FormatPct(tc.input)
		if got != tc.want {
			t.Errorf("FormatPct(%v) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

// ---------------------------------------------------------------------------
// NumberWithCommas
// ---------------------------------------------------------------------------

func TestNumberWithCommas(t *testing.T) {
	cases := []struct {
		input int
		want  string
	}{
		{0, "0"},
		{999, "999"},
		{1000, "1,000"},
		{1000000, "1,000,000"},
	}
	for _, tc := range cases {
		got := NumberWithCommas(tc.input)
		if got != tc.want {
			t.Errorf("NumberWithCommas(%d) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

// ---------------------------------------------------------------------------
// PctColor
// ---------------------------------------------------------------------------

func TestPctColorNoColor(t *testing.T) {
	got := PctColor(50, true)
	if got != "" {
		t.Errorf("PctColor(50, noColor=true) = %q, want \"\"", got)
	}
}

// ---------------------------------------------------------------------------
// StripAnsi
// ---------------------------------------------------------------------------

func TestStripAnsi(t *testing.T) {
	input := "\x1b[38;5;56mhello\x1b[0m world\x1b[1m!"
	got := StripAnsi(input)
	want := "hello world!"
	if got != want {
		t.Errorf("StripAnsi(%q) = %q, want %q", input, got, want)
	}
}

func TestStripAnsiNoEscapes(t *testing.T) {
	input := "plain text"
	got := StripAnsi(input)
	if got != input {
		t.Errorf("StripAnsi(%q) = %q, want unchanged", input, got)
	}
}

// ---------------------------------------------------------------------------
// RenderCompact
// ---------------------------------------------------------------------------

func TestRenderCompactContainsPipe(t *testing.T) {
	comp := makeComposition(50000, 200000)
	opts := RenderOpts{NoColor: true}
	got := RenderCompact(comp, opts)
	if !strings.Contains(got, "│") {
		t.Errorf("RenderCompact output does not contain '│': %q", got)
	}
}

// ---------------------------------------------------------------------------
// RenderStatusline
// ---------------------------------------------------------------------------

func TestRenderStatuslineContainsPct(t *testing.T) {
	comp := makeComposition(50000, 200000)
	opts := RenderOpts{NoColor: true}
	got := RenderStatusline(comp, opts)
	if !strings.Contains(got, "%") {
		t.Errorf("RenderStatusline output does not contain '%%': %q", got)
	}
}
