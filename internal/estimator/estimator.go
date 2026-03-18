package estimator

import (
	"math"
	"time"

	"github.com/tonylee/ctx-monitor/internal/model"
)

// safeNum safely converts interface{} to int.
func safeNum(v interface{}) int {
	if v == nil {
		return 0
	}
	switch n := v.(type) {
	case int:
		return n
	case int64:
		return int(n)
	case float64:
		return int(n)
	case float32:
		return int(n)
	}
	return 0
}

// pct returns a percentage rounded to 2 decimal places.
func pct(tokens, windowSize int) float64 {
	if windowSize == 0 {
		return 0
	}
	raw := float64(tokens) / float64(windowSize) * 100
	return math.Round(raw*100) / 100
}

// buildComponent constructs a Component from a ComponentDef and token count.
func buildComponent(def model.ComponentDef, tokens int, source string) model.Component {
	return model.Component{
		Key:       def.Key,
		Label:     def.Label,
		Tokens:    tokens,
		Pct:       0, // finalized later
		Color:     def.Color,
		TextColor: def.TextColor,
		Fixed:     def.Fixed,
		Source:    source,
	}
}

// finalizePcts sets Pct on each component based on windowSize.
func finalizePcts(components []model.Component, windowSize int) {
	for i := range components {
		components[i].Pct = pct(components[i].Tokens, windowSize)
	}
}

// EstimateClaudeContext builds a Composition for a Claude Code session.
func EstimateClaudeContext(session *model.ClaudeSession, config *model.ClaudeConfig, statusline map[string]interface{}) *model.Composition {
	defs := model.ClaudeComponents()

	// Nil-safe defaults
	if session == nil {
		session = &model.ClaudeSession{}
	}
	if config == nil {
		config = &model.ClaudeConfig{}
	}

	// --- Detect context window size ---
	windowSize := 0

	// 1. Check statusline.context_window.context_window_size
	if statusline != nil {
		if cw, ok := statusline["context_window"]; ok {
			if cwMap, ok := cw.(map[string]interface{}); ok {
				windowSize = safeNum(cwMap["context_window_size"])
			}
		}
	}

	// 2. Resolve from model
	modelID := session.Model
	if modelID == "" && statusline != nil {
		if m, ok := statusline["model"]; ok {
			if mMap, ok := m.(map[string]interface{}); ok {
				modelID = stringVal(mMap["id"])
			}
		}
	}
	resolved := model.ResolveModel(modelID)
	if windowSize == 0 {
		windowSize = resolved.Context
	}

	// --- Build 13 components ---
	// Indices: 0=system, 1=tools, 2=mcp, 3=agents, 4=memory, 5=skill_meta,
	//          6=skill_body, 7=plan, 8=user_msg, 9=tool_results, 10=responses,
	//          11=subagent, 12=buffer
	buckets := session.TokenBuckets

	componentTokens := [13]int{
		defs[0].DefaultTokens,              // system (fixed)
		defs[1].DefaultTokens,              // tools (fixed)
		config.MCP.TotalTokens,             // mcp
		config.Agents.TotalTokens,          // agents
		config.Memory.TotalTokens,          // memory
		config.Skills.TotalFrontmatterTokens, // skill_meta
		buckets.SkillBody,                  // skill_body
		buckets.Plan,                       // plan
		buckets.UserMsg,                    // user_msg
		buckets.ToolResults,                // tool_results
		buckets.Responses,                  // responses
		buckets.Subagent,                   // subagent
		defs[12].DefaultTokens,             // buffer (fixed)
	}

	components := make([]model.Component, 13)
	for i, def := range defs {
		source := "estimated"
		if def.Fixed {
			source = "fixed"
		}
		components[i] = buildComponent(def, componentTokens[i], source)
	}

	// --- Compute estimated total (excluding buffer) ---
	estimatedTotal := 0
	for i, c := range components {
		if i != 12 { // exclude buffer
			estimatedTotal += c.Tokens
		}
	}

	// --- Reconcile with measured data ---
	measuredInput := 0
	if statusline != nil {
		if cw, ok := statusline["context_window"]; ok {
			if cwMap, ok := cw.(map[string]interface{}); ok {
				measuredInput = safeNum(cwMap["total_input_tokens"])
			}
		}
	}
	if measuredInput == 0 && session.Usage != nil {
		measuredInput = session.Usage.InputTokens +
			session.Usage.CacheCreationInputTokens +
			session.Usage.CacheReadInputTokens
	}

	// Bump window size if measured input exceeds registry value
	if measuredInput > windowSize {
		windowSize = 1_000_000
	}

	if measuredInput > 0 {
		diff := measuredInput - estimatedTotal
		if diff != 0 {
			// Distribute diff proportionally across non-fixed, non-buffer estimated components
			scalable := []int{}
			scalableTotal := 0
			for i, c := range components {
				if i != 12 && !c.Fixed {
					scalable = append(scalable, i)
					scalableTotal += c.Tokens
				}
			}
			if scalableTotal > 0 {
				for _, idx := range scalable {
					share := float64(components[idx].Tokens) / float64(scalableTotal)
					components[idx].Tokens += int(math.Round(share * float64(diff)))
					components[idx].Source = "reconciled"
				}
			} else if len(scalable) > 0 {
				// Distribute evenly
				perComp := diff / len(scalable)
				for _, idx := range scalable {
					components[idx].Tokens += perComp
					components[idx].Source = "reconciled"
				}
			}
		}
	}

	// Ensure no negative tokens
	for i := range components {
		if components[i].Tokens < 0 {
			components[i].Tokens = 0
		}
	}

	// --- Compute totals ---
	totalUsed := 0
	for _, c := range components {
		totalUsed += c.Tokens
	}

	// apiMatchTokens = total minus buffer
	apiMatchTokens := totalUsed - components[12].Tokens
	if apiMatchTokens < 0 {
		apiMatchTokens = 0
	}

	freeTokens := windowSize - totalUsed
	if freeTokens < 0 {
		freeTokens = 0
	}

	finalizePcts(components, windowSize)

	totalUsedPct := pct(totalUsed, windowSize)
	apiMatchPct := pct(apiMatchTokens, windowSize)

	// --- Session ID from statusline or session ---
	sessionID := session.SessionID
	if sessionID == "" && statusline != nil {
		sessionID = stringVal(statusline["session_id"])
	}

	// --- Ancillary info ---
	var compactionEvents []model.CompactionEvent
	for _, ts := range session.CompactionEvents.Timestamps {
		compactionEvents = append(compactionEvents, model.CompactionEvent{Timestamp: ts})
	}

	var subagents []model.SubagentSpawn
	for _, s := range session.SubagentSpawns {
		subagents = append(subagents, model.SubagentSpawn{
			Tool:         "claude",
			SubagentType: s.SubagentType,
			Description:  s.Description,
			Timestamp:    s.Timestamp,
			ID:           s.ID,
		})
	}

	var mcpServers []model.MCPServerInfo
	mcpServers = config.MCP.Servers

	var skills *model.SkillSummary
	if len(config.Skills.Installed) > 0 || config.Skills.TotalFrontmatterTokens > 0 {
		var activeSkills []string
		for _, sa := range session.SkillActivations {
			activeSkills = append(activeSkills, sa.Skill)
		}
		skills = &model.SkillSummary{
			Installed:         config.Skills.Count,
			Active:            activeSkills,
			FrontmatterTokens: config.Skills.TotalFrontmatterTokens,
			BodyTokens:        buckets.SkillBody,
			Files:             config.Skills.Installed,
		}
	}

	var planUsage interface{}
	if len(session.PlanUsage) > 0 {
		planUsage = session.PlanUsage
	}

	return &model.Composition{
		Tool:            "claude",
		Model:           resolved.Display,
		ModelID:         modelID,
		ModelTier:       resolved.Tier,
		ModelReasoning:  resolved.Reasoning,
		IsFast:          resolved.IsFast,
		SessionID:       sessionID,
		ContextWindowSize: windowSize,
		TotalUsedPct:    totalUsedPct,
		TotalUsedTokens: totalUsed,
		APIMatchPct:     apiMatchPct,
		APIMatchTokens:  apiMatchTokens,
		FreeTokens:      freeTokens,
		Components:      components,
		CompactionEvents: compactionEvents,
		Subagents:       subagents,
		MCPServers:      mcpServers,
		Skills:          skills,
		MemoryFiles:     config.Memory.Files,
		AgentFiles:      config.Agents.Files,
		PlanUsage:       planUsage,
		ToolCalls:       session.ToolCalls,
		Turns:           session.Turns,
		Attachments:     session.Attachments,
		Timestamp:       time.Now().UTC().Format(time.RFC3339),
	}
}

// EstimateCodexContext builds a Composition for a Codex CLI session.
func EstimateCodexContext(session *model.CodexSession, config *model.CodexConfig) *model.Composition {
	defs := model.CodexComponents()

	// Nil-safe defaults
	if session == nil {
		session = &model.CodexSession{}
	}
	if config == nil {
		config = &model.CodexConfig{}
	}

	// --- Resolve model ---
	modelID := session.Model
	if modelID == "" {
		modelID = config.Model
	}
	resolved := model.ResolveModel(modelID)

	windowSize := session.ContextWindowSize
	if windowSize == 0 {
		windowSize = resolved.Context
	}

	// --- Build 12 components ---
	// Indices: 0=instructions, 1=tools, 2=mcp, 3=agents, 4=skills, 5=plan,
	//          6=user_msg, 7=tool_results, 8=responses, 9=subagent, 10=reasoning, 11=free
	buckets := session.TokenBuckets

	agentsTokens := config.Instructions.Tokens + config.Agents.TotalTokens

	componentTokens := [12]int{
		defs[0].DefaultTokens,     // instructions (fixed)
		defs[1].DefaultTokens,     // tools (fixed)
		config.MCP.TotalTokens,    // mcp
		agentsTokens,              // agents (AGENTS.md + agent defs)
		config.Skills.TotalTokens, // skills
		buckets.Plan,              // plan
		buckets.UserMsg,           // user_msg
		buckets.ToolResults,       // tool_results
		buckets.Responses,         // responses
		buckets.Subagent,          // subagent
		buckets.Reasoning,         // reasoning
		0,                         // free (computed below)
	}

	// Compute used tokens (all except free slot)
	usedTokens := 0
	for i := 0; i < 11; i++ {
		usedTokens += componentTokens[i]
	}

	// Component 12 (index 11) = free
	freeTokens := windowSize - usedTokens
	if freeTokens < 0 {
		freeTokens = 0
	}
	componentTokens[11] = freeTokens

	components := make([]model.Component, 12)
	for i, def := range defs {
		source := "estimated"
		if def.Fixed {
			source = "fixed"
		}
		if def.Key == "free" {
			source = "computed"
		}
		components[i] = buildComponent(def, componentTokens[i], source)
	}

	// --- Reconcile with measured token usage ---
	measuredInput := 0
	if session.LastTokenUsage.Input > 0 {
		measuredInput = session.LastTokenUsage.Input
	} else if session.TokenUsage.Input > 0 {
		measuredInput = session.TokenUsage.Input
	}

	if measuredInput > windowSize {
		windowSize = 1_000_000
	}

	if measuredInput > 0 {
		// Reconcile non-free components
		estimatedUsed := 0
		for i := 0; i < 11; i++ {
			estimatedUsed += components[i].Tokens
		}
		diff := measuredInput - estimatedUsed
		if diff != 0 {
			scalable := []int{}
			scalableTotal := 0
			for i := 0; i < 11; i++ {
				if !components[i].Fixed {
					scalable = append(scalable, i)
					scalableTotal += components[i].Tokens
				}
			}
			if scalableTotal > 0 {
				for _, idx := range scalable {
					share := float64(components[idx].Tokens) / float64(scalableTotal)
					components[idx].Tokens += int(math.Round(share * float64(diff)))
					components[idx].Source = "reconciled"
				}
			} else if len(scalable) > 0 {
				perComp := diff / len(scalable)
				for _, idx := range scalable {
					components[idx].Tokens += perComp
					components[idx].Source = "reconciled"
				}
			}
		}

		// Recompute free
		newUsed := 0
		for i := 0; i < 11; i++ {
			if components[i].Tokens < 0 {
				components[i].Tokens = 0
			}
			newUsed += components[i].Tokens
		}
		newFree := windowSize - newUsed
		if newFree < 0 {
			newFree = 0
		}
		components[11].Tokens = newFree
		usedTokens = newUsed
	}

	// Ensure no negatives
	for i := range components {
		if components[i].Tokens < 0 {
			components[i].Tokens = 0
		}
	}

	totalUsed := 0
	for i := 0; i < 11; i++ {
		totalUsed += components[i].Tokens
	}
	components[11].Tokens = windowSize - totalUsed
	if components[11].Tokens < 0 {
		components[11].Tokens = 0
	}
	totalWithFree := totalUsed + components[11].Tokens

	finalizePcts(components, windowSize)

	totalUsedPct := pct(totalUsed, windowSize)
	_ = totalWithFree

	// --- Ancillary info ---
	var subagents []model.SubagentSpawn
	for _, tc := range session.SubagentSpawns {
		subagents = append(subagents, model.SubagentSpawn{
			Tool:      "codex",
			ID:        tc.ID,
			Timestamp: tc.Timestamp,
		})
	}

	var planUsage interface{}
	if len(session.PlanUsage) > 0 {
		planUsage = session.PlanUsage
	}

	var mcpServers []model.MCPServerInfo
	for _, s := range config.MCP.Servers {
		mcpServers = append(mcpServers, model.MCPServerInfo{
			Name:            s.Name,
			EstimatedTokens: s.EstimatedTokens,
		})
	}

	return &model.Composition{
		Tool:              "codex",
		Model:             resolved.Display,
		ModelID:           modelID,
		ModelTier:         resolved.Tier,
		ModelReasoning:    resolved.Reasoning,
		IsFast:            resolved.IsFast,
		SessionID:         session.SessionID,
		ContextWindowSize: windowSize,
		TotalUsedPct:      totalUsedPct,
		TotalUsedTokens:   totalUsed,
		FreeTokens:        components[11].Tokens,
		Components:        components,
		Subagents:         subagents,
		MCPServers:        mcpServers,
		PlanUsage:         planUsage,
		Timestamp:         time.Now().UTC().Format(time.RFC3339),
	}
}

// SimulateUsage scales non-fixed components so totalUsedPct equals targetPct.
func SimulateUsage(comp *model.Composition, targetPct float64) *model.Composition {
	windowSize := comp.ContextWindowSize
	targetTokens := int(math.Round(float64(windowSize) * targetPct / 100))

	// Deep-clone components
	cloned := make([]model.Component, len(comp.Components))
	copy(cloned, comp.Components)

	// Separate "free" component index (codex only)
	freeIdx := -1
	for i, c := range cloned {
		if c.Key == "free" {
			freeIdx = i
			break
		}
	}

	// Gather fixed and scalable indices (exclude free)
	fixedTokens := 0
	scalable := []int{}
	scalableTokens := 0
	for i, c := range cloned {
		if i == freeIdx {
			continue
		}
		if c.Fixed {
			fixedTokens += c.Tokens
		} else {
			scalable = append(scalable, i)
			scalableTokens += c.Tokens
		}
	}

	scalableTarget := targetTokens - fixedTokens
	if scalableTarget < 0 {
		scalableTarget = 0
	}

	if scalableTokens > 0 {
		// Scale proportionally
		for _, idx := range scalable {
			share := float64(cloned[idx].Tokens) / float64(scalableTokens)
			cloned[idx].Tokens = int(math.Round(share * float64(scalableTarget)))
		}
	} else if len(scalable) > 0 {
		// Distribute evenly
		perComp := scalableTarget / len(scalable)
		for _, idx := range scalable {
			cloned[idx].Tokens = perComp
		}
	}

	// Ensure no negatives
	for i := range cloned {
		if cloned[i].Tokens < 0 {
			cloned[i].Tokens = 0
		}
	}

	// Recompute free for codex
	if freeIdx >= 0 {
		used := 0
		for i, c := range cloned {
			if i != freeIdx {
				used += c.Tokens
			}
		}
		free := windowSize - used
		if free < 0 {
			free = 0
		}
		cloned[freeIdx].Tokens = free
	}

	// Recompute totals
	totalUsed := 0
	for i, c := range cloned {
		if i != freeIdx {
			totalUsed += c.Tokens
		}
	}

	finalizePcts(cloned, windowSize)

	freeTokens := windowSize - totalUsed
	if freeTokens < 0 {
		freeTokens = 0
	}

	// Build new composition (shallow copy of non-component fields)
	newComp := *comp
	newComp.Components = cloned
	newComp.TotalUsedTokens = totalUsed
	newComp.TotalUsedPct = pct(totalUsed, windowSize)
	newComp.FreeTokens = freeTokens
	if freeIdx >= 0 {
		newComp.FreeTokens = cloned[freeIdx].Tokens
	}

	apiMatch := totalUsed
	// For claude, subtract buffer (last fixed component)
	if comp.Tool == "claude" {
		for i := len(cloned) - 1; i >= 0; i-- {
			if cloned[i].Key == "buffer" {
				apiMatch = totalUsed - cloned[i].Tokens
				if apiMatch < 0 {
					apiMatch = 0
				}
				break
			}
		}
	}
	newComp.APIMatchTokens = apiMatch
	newComp.APIMatchPct = pct(apiMatch, windowSize)
	newComp.Timestamp = time.Now().UTC().Format(time.RFC3339)

	return &newComp
}

// stringVal safely extracts a string from an interface{}.
func stringVal(v interface{}) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}
