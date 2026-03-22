package model

import "strings"

// ModelInfo describes a model's capabilities and context window.
type ModelInfo struct {
	Display   string
	Context   int
	Family    string // "claude" or "codex"
	Tier      string // "opus", "sonnet", "haiku", "flagship", "mini", etc.
	Reasoning string // "low", "medium", "high", "xhigh"
	IsFast    bool
}

var modelRegistry = map[string]ModelInfo{
	// Claude Code models
	"claude-opus-4-6":             {Display: "Claude Opus 4.6", Context: 200_000, Family: "claude", Tier: "opus", Reasoning: "high"},
	"claude-opus-4-6[1m]":         {Display: "Claude Opus 4.6 (1M)", Context: 1_000_000, Family: "claude", Tier: "opus", Reasoning: "high"},
	"claude-opus-4-6-20250610":    {Display: "Claude Opus 4.6", Context: 200_000, Family: "claude", Tier: "opus", Reasoning: "high"},
	"claude-sonnet-4-6":           {Display: "Claude Sonnet 4.6", Context: 200_000, Family: "claude", Tier: "sonnet", Reasoning: "medium"},
	"claude-sonnet-4-6-20250514":  {Display: "Claude Sonnet 4.6", Context: 200_000, Family: "claude", Tier: "sonnet", Reasoning: "medium"},
	"claude-haiku-4-5":            {Display: "Claude Haiku 4.5", Context: 200_000, Family: "claude", Tier: "haiku", Reasoning: "low"},
	"claude-haiku-4-5-20251001":   {Display: "Claude Haiku 4.5", Context: 200_000, Family: "claude", Tier: "haiku", Reasoning: "low"},
	"claude-sonnet-4-20250514":    {Display: "Claude Sonnet 4", Context: 200_000, Family: "claude", Tier: "sonnet", Reasoning: "medium"},
	"claude-opus-4-20250514":      {Display: "Claude Opus 4", Context: 200_000, Family: "claude", Tier: "opus", Reasoning: "high"},
	"claude-3-5-sonnet-20241022":  {Display: "Claude 3.5 Sonnet", Context: 200_000, Family: "claude", Tier: "sonnet", Reasoning: "medium"},
	"claude-3-5-haiku-20241022":   {Display: "Claude 3.5 Haiku", Context: 200_000, Family: "claude", Tier: "haiku", Reasoning: "low"},
	"claude-3-opus-20240229":      {Display: "Claude 3 Opus", Context: 200_000, Family: "claude", Tier: "opus", Reasoning: "high"},

	// Codex CLI / OpenAI models
	"gpt-5.4":            {Display: "GPT-5.4", Context: 256_000, Family: "codex", Tier: "flagship", Reasoning: "high"},
	"gpt-5.4-mini":       {Display: "GPT-5.4 Mini", Context: 128_000, Family: "codex", Tier: "mini", Reasoning: "medium"},
	"gpt-5.3-codex":      {Display: "GPT-5.3 Codex", Context: 256_000, Family: "codex", Tier: "codex", Reasoning: "high"},
	"gpt-5.2":            {Display: "GPT-5.2", Context: 200_000, Family: "codex", Tier: "flagship", Reasoning: "high"},
	"gpt-5.2-codex":      {Display: "GPT-5.2 Codex", Context: 200_000, Family: "codex", Tier: "codex", Reasoning: "high"},
	"gpt-5.1-codex-max":  {Display: "GPT-5.1 Codex Max", Context: 256_000, Family: "codex", Tier: "max", Reasoning: "xhigh"},
	"gpt-5.1-codex-mini": {Display: "GPT-5.1 Codex Mini", Context: 128_000, Family: "codex", Tier: "mini", Reasoning: "medium"},
	"o3":                  {Display: "o3", Context: 200_000, Family: "codex", Tier: "reasoning", Reasoning: "xhigh"},
	"o3-mini":             {Display: "o3 Mini", Context: 200_000, Family: "codex", Tier: "mini", Reasoning: "high"},
	"o4-mini":             {Display: "o4 Mini", Context: 200_000, Family: "codex", Tier: "mini", Reasoning: "high"},
	"gpt-4.1":             {Display: "GPT-4.1", Context: 128_000, Family: "codex", Tier: "flagship", Reasoning: "medium"},
	"gpt-4.1-mini":        {Display: "GPT-4.1 Mini", Context: 128_000, Family: "codex", Tier: "mini", Reasoning: "low"},
	"gpt-4o":              {Display: "GPT-4o", Context: 128_000, Family: "codex", Tier: "flagship", Reasoning: "medium"},
	"gpt-4o-mini":         {Display: "GPT-4o Mini", Context: 128_000, Family: "codex", Tier: "mini", Reasoning: "low"},
}

// ResolveModel looks up a model by ID, handling /fast suffix and [1m] variants.
func ResolveModel(modelID string) ModelInfo {
	if modelID == "" {
		return ModelInfo{Display: "Unknown", Context: 200_000, Family: "unknown", Tier: "unknown", Reasoning: "medium"}
	}

	// Handle /fast suffix
	isFast := strings.Contains(modelID, "/fast") || strings.HasSuffix(modelID, "-fast")
	baseID := strings.TrimSuffix(strings.TrimSuffix(modelID, "/fast"), "-fast")

	// Strip [1m] for lookup but remember it
	is1M := strings.Contains(baseID, "[1m]") || strings.Contains(baseID, "(1m)")
	lookupID := strings.TrimSpace(strings.ReplaceAll(strings.ReplaceAll(baseID, "[1m]", ""), "(1m)", ""))

	// Direct match
	if m, ok := modelRegistry[baseID]; ok {
		m.IsFast = isFast
		if isFast {
			m.Display += " (Fast)"
		}
		return m
	}

	// Match without [1m] then override context
	if m, ok := modelRegistry[lookupID]; ok {
		m.IsFast = isFast
		if is1M {
			m.Context = 1_000_000
			m.Display += " (1M)"
		}
		if isFast {
			m.Display += " (Fast)"
		}
		return m
	}

	// Prefix match — find longest matching prefix
	var bestMatch *ModelInfo
	bestLen := 0
	for key, val := range modelRegistry {
		if strings.HasPrefix(lookupID, key) && len(key) > bestLen {
			v := val
			bestMatch = &v
			bestLen = len(key)
		}
	}
	if bestMatch != nil {
		bestMatch.IsFast = isFast
		if is1M {
			bestMatch.Context = 1_000_000
			bestMatch.Display += " (1M)"
		}
		if isFast {
			bestMatch.Display += " (Fast)"
		}
		return *bestMatch
	}

	// Fallback: infer family from name
	family := "unknown"
	ctx := 200_000
	if strings.HasPrefix(lookupID, "claude") {
		family = "claude"
	} else if strings.HasPrefix(lookupID, "gpt") || strings.HasPrefix(lookupID, "o3") || strings.HasPrefix(lookupID, "o4") {
		family = "codex"
		ctx = 128_000
	}

	return ModelInfo{
		Display:   modelID,
		Context:   ctx,
		Family:    family,
		Tier:      "unknown",
		Reasoning: "medium",
		IsFast:    isFast,
	}
}
