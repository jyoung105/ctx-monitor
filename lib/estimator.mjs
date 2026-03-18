/**
 * estimator.mjs — Context Composition Estimation Engine.
 *
 * Takes parsed session data and config data from the parsers and computes
 * a full context composition breakdown for Claude Code and Codex CLI.
 *
 * Zero npm dependencies; Node.js built-ins only.
 */

import { COMPONENTS, estimateTokens, resolveModel } from './colors.mjs';

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function safeNum(v, fallback = 0) {
  if (v == null || v === '') return fallback;
  const n = Number(v);
  return Number.isFinite(n) ? n : fallback;
}

function pct(tokens, windowSize) {
  if (!windowSize) return 0;
  return Math.round((tokens / windowSize) * 10000) / 100; // two decimals
}

function buildComponent(def, tokens, source = 'estimated') {
  return {
    key: def.key,
    label: def.label,
    tokens,
    pct: 0, // filled in after all components are assembled
    color: def.color,
    textColor: def.textColor,
    fixed: def.fixed,
    source,
  };
}

function finalizePcts(components, windowSize) {
  for (const c of components) {
    c.pct = pct(c.tokens, windowSize);
  }
}

/**
 * When we have a measured total from the API, distribute the difference
 * between measured and estimated totals proportionally across non-fixed
 * estimated components.
 */
function reconcileWithMeasured(components, measuredTotal) {
  const estimatedTotal = components.reduce((s, c) => s + c.tokens, 0);
  const diff = measuredTotal - estimatedTotal;

  if (Math.abs(diff) < 10) return; // close enough

  // Only adjust non-fixed, estimated components
  const adjustable = components.filter((c) => !c.fixed && c.source === 'estimated');
  const adjustableSum = adjustable.reduce((s, c) => s + c.tokens, 0);

  if (adjustableSum === 0) return; // nothing to scale

  for (const c of adjustable) {
    const share = c.tokens / adjustableSum;
    c.tokens = Math.max(0, Math.round(c.tokens + diff * share));
  }
}

// ---------------------------------------------------------------------------
// Claude Code estimation
// ---------------------------------------------------------------------------

function detectContextWindowSize(session, statusline) {
  // 1. Statusline provides the definitive value
  if (statusline?.context_window?.context_window_size) {
    return statusline.context_window.context_window_size;
  }
  // 2. Use model registry
  const modelId = session?.model || statusline?.model?.id || '';
  const modelInfo = resolveModel(modelId);
  let windowSize = modelInfo.context;
  // 3. If measured input already exceeds the registry value, bump up
  if (session?.usage) {
    const u = session.usage;
    const total = (u.input_tokens || 0) + (u.cache_creation_input_tokens || 0) + (u.cache_read_input_tokens || 0);
    if (total > windowSize) windowSize = 1_000_000;
  }
  return windowSize;
}

/**
 * Estimate context composition for a Claude Code session.
 *
 * @param {object} params
 * @param {object|null} params.session — Parsed session data (messages, token buckets, etc.)
 * @param {object|null} params.config  — Parsed config data from parseClaudeConfig()
 * @param {object|null} params.statusline — Live statusline JSON from stdin (optional)
 * @returns {object} Context composition breakdown
 */
export function estimateClaudeContext({ session = null, config = null, statusline = null } = {}) {
  const defs = COMPONENTS.claude;
  const contextWindowSize = detectContextWindowSize(session, statusline);

  const buckets = session?.tokenBuckets ?? {};
  const components = [];

  // 1. System prompt (fixed)
  components.push(buildComponent(defs[0], defs[0].defaultTokens));

  // 2. Built-in tools (fixed)
  components.push(buildComponent(defs[1], defs[1].defaultTokens));

  // 3. MCP tools
  components.push(buildComponent(defs[2], safeNum(config?.mcp?.totalTokens)));

  // 4. Agents
  components.push(buildComponent(defs[3], safeNum(config?.agents?.totalTokens)));

  // 5. Memory (CLAUDE.md)
  components.push(buildComponent(defs[4], safeNum(config?.memory?.totalTokens)));

  // 6. Skill frontmatter
  components.push(buildComponent(defs[5], safeNum(config?.skills?.totalFrontmatterTokens)));

  // 7. Skill body (active) — from session
  components.push(buildComponent(defs[6], safeNum(buckets.skill_body)));

  // 8. Plan / todo
  components.push(buildComponent(defs[7], safeNum(buckets.plan)));

  // 9. User messages
  components.push(buildComponent(defs[8], safeNum(buckets.user_msg)));

  // 10. Tool call results
  components.push(buildComponent(defs[9], safeNum(buckets.tool_results)));

  // 11. AI responses
  components.push(buildComponent(defs[10], safeNum(buckets.responses)));

  // 12. Subagent summaries
  components.push(buildComponent(defs[11], safeNum(buckets.subagent)));

  // 13. Compact buffer (fixed)
  components.push(buildComponent(defs[12], defs[12].defaultTokens));

  // --- Reconcile with measured data ---
  // The API's measured input tokens = everything in context EXCEPT the buffer.
  // Buffer is a reserved concept (Claude Code reserves ~45k for compaction),
  // not an actual token count reported by the API.
  // So we reconcile non-buffer components against the measured total.

  let measuredSource = 'estimated';
  let measuredTotal = 0;

  if (statusline?.context_window?.total_input_tokens != null) {
    measuredTotal = safeNum(statusline.context_window.total_input_tokens);
    measuredSource = 'measured';
  }

  if (session?.usage) {
    const u = session.usage;
    const fromUsage =
      safeNum(u.input_tokens) + safeNum(u.cache_creation_input_tokens) + safeNum(u.cache_read_input_tokens);
    if (fromUsage > 0) {
      measuredTotal = fromUsage;
      measuredSource = 'measured';
    }
  }

  if (measuredTotal > 0) {
    // Reconcile only non-buffer components against measured total
    const nonBufferComponents = components.filter(c => c.key !== 'buffer');
    const estimatedNonBuffer = nonBufferComponents.reduce((s, c) => s + c.tokens, 0);
    const diff = measuredTotal - estimatedNonBuffer;

    if (Math.abs(diff) > 10) {
      const adjustable = nonBufferComponents.filter(c => !c.fixed && c.source === 'estimated');
      const adjustableSum = adjustable.reduce((s, c) => s + c.tokens, 0);
      if (adjustableSum > 0) {
        for (const c of adjustable) {
          const share = c.tokens / adjustableSum;
          c.tokens = Math.max(0, Math.round(c.tokens + diff * share));
        }
      }
    }
  }

  // Compute percentages and totals
  // Total used = measured input tokens (what Claude Code shows) + buffer reservation
  finalizePcts(components, contextWindowSize);

  const totalUsedTokens = components.reduce((s, c) => s + c.tokens, 0);
  const totalUsedPct = pct(totalUsedTokens, contextWindowSize);
  // Also compute the "API-matching" percentage (without buffer) for display
  const apiMatchTokens = components.filter(c => c.key !== 'buffer').reduce((s, c) => s + c.tokens, 0);
  const apiMatchPct = pct(apiMatchTokens, contextWindowSize);
  const freeTokens = Math.max(0, contextWindowSize - totalUsedTokens);

  // --- Collect ancillary info ---

  const compactionEvents = session?.compactionEvents ?? [];
  const subagents = session?.subagentSpawns ?? [];
  const mcpServers = config?.mcp?.servers ?? [];
  const skills = {
    installed: config?.skills?.count ?? 0,
    active: session?.skillActivations?.map(s => s.skill) ?? [],
    frontmatterTokens: safeNum(config?.skills?.totalFrontmatterTokens),
    bodyTokens: safeNum(buckets.skill_body),
    files: config?.skills?.installed ?? [],
  };
  const memoryFiles = config?.memory?.files ?? [];
  const agentFiles = config?.agents?.files ?? [];
  const planUsage = session?.planUsage ?? null;
  const toolCalls = session?.toolCalls ?? [];
  const turns = session?.turns ?? [];
  const attachments = session?.attachments ?? [];

  // Model from session or statusline — resolve to display name
  const rawModel = session?.model ?? statusline?.model?.id ?? statusline?.model ?? '';
  const modelInfo = resolveModel(rawModel);
  const model = modelInfo.display;

  const sessionId = session?.sessionId ?? statusline?.session_id ?? 'unknown';

  return {
    tool: 'claude',
    model,
    modelId: rawModel,
    modelTier: modelInfo.tier,
    modelReasoning: modelInfo.reasoning,
    isFast: modelInfo.isFast || false,
    sessionId,
    contextWindowSize,
    totalUsedPct,       // includes buffer reservation
    totalUsedTokens,    // includes buffer reservation
    apiMatchPct,        // matches Claude Code's HUD (excludes buffer)
    apiMatchTokens,     // matches Claude Code's HUD (excludes buffer)
    freeTokens,
    components,
    compactionEvents,
    subagents,
    mcpServers,
    skills,
    memoryFiles,
    agentFiles,
    planUsage,
    toolCalls,
    turns,
    attachments,
    timestamp: new Date().toISOString(),
  };
}

// ---------------------------------------------------------------------------
// Codex CLI estimation
// ---------------------------------------------------------------------------

/**
 * Estimate context composition for a Codex CLI session.
 *
 * @param {object} params
 * @param {object|null} params.session — Parsed session data
 * @param {object|null} params.config  — Parsed config data
 * @returns {object} Context composition breakdown
 */
export function estimateCodexContext({ session = null, config = null } = {}) {
  const defs = COMPONENTS.codex;
  const rawModel = session?.model || config?.model || '';
  const modelInfo = resolveModel(rawModel);
  const contextWindowSize = safeNum(session?.contextWindowSize, modelInfo.context);

  const buckets = session?.tokenBuckets ?? {};
  const tokenUsage = session?.tokenUsage ?? {};
  const lastTokenUsage = session?.lastTokenUsage ?? {};
  const components = [];

  // 1. Instructions (fixed)
  components.push(buildComponent(defs[0], defs[0].defaultTokens));

  // 2. Built-in tools (fixed)
  components.push(buildComponent(defs[1], defs[1].defaultTokens));

  // 3. MCP tools
  components.push(buildComponent(defs[2], safeNum(config?.mcp?.totalTokens)));

  // 4. AGENTS.md / memories
  const agentMemTokens = safeNum(config?.instructions?.tokens) + safeNum(config?.agents?.totalTokens);
  components.push(buildComponent(defs[3], agentMemTokens));

  // 5. Skills
  components.push(buildComponent(defs[4], safeNum(config?.skills?.totalTokens)));

  // 6. Plan
  components.push(buildComponent(defs[5], safeNum(buckets.plan)));

  // 7. User messages
  components.push(buildComponent(defs[6], safeNum(buckets.user_msg)));

  // 8. Tool call results
  components.push(buildComponent(defs[7], safeNum(buckets.tool_results)));

  // 9. Codex responses
  components.push(buildComponent(defs[8], safeNum(buckets.responses)));

  // 10. Subagent summaries
  components.push(buildComponent(defs[9], safeNum(buckets.subagent)));

  // 11. Reasoning tokens
  const reasoningTokens = safeNum(tokenUsage.reasoning);
  components.push(buildComponent(defs[10], reasoningTokens));

  // Sum of the 11 content components (everything except free space)
  let usedTokens = components.reduce((s, c) => s + c.tokens, 0);

  // 12. Free space
  const freeSpaceTokens = Math.max(0, contextWindowSize - usedTokens);
  components.push(buildComponent(defs[11], freeSpaceTokens));

  // --- Reconcile with measured data ---

  if (session?.tokenUsage) {
    const u = session.tokenUsage;
    const lastMeasuredInput = safeNum(lastTokenUsage.input, null);
    const measuredTotal = safeNum(u.total, null) ?? safeNum(u.total_tokens, null);
    const measuredInput =
      safeNum(u.input, null) ??
      safeNum(u.input_tokens, null) ??
      (safeNum(u.cached, 0) + safeNum(u.output, 0));
    const measuredContent = lastMeasuredInput || measuredInput || measuredTotal;
    if (measuredContent > 0 && measuredContent <= contextWindowSize) {
      // Reconcile only the non-free-space components
      const contentComponents = components.slice(0, -1);
      reconcileWithMeasured(contentComponents, measuredContent);

      // Recompute free space
      usedTokens = contentComponents.reduce((s, c) => s + c.tokens, 0);
      components[components.length - 1].tokens = Math.max(0, contextWindowSize - usedTokens);

      // Mark as measured
      for (const c of contentComponents) {
        if (!c.fixed) c.source = 'measured';
      }
    }
  }

  // Compute percentages
  finalizePcts(components, contextWindowSize);

  const totalUsedTokens = usedTokens; // excludes free space
  const totalUsedPct = pct(totalUsedTokens, contextWindowSize);
  const freeTokens = Math.max(0, contextWindowSize - totalUsedTokens);

  // --- Collect ancillary info ---

  const compactionEvents = session?.compactionEvents ?? [];
  const subagents = session?.subagentSpawns ?? [];
  const mcpServers = config?.mcp?.servers ?? [];
  const skills = {
    installed: config?.skills?.count ?? 0,
    active: session?.skillActivations?.map(s => s.skill) ?? [],
    frontmatterTokens: safeNum(config?.skills?.totalFrontmatterTokens),
    bodyTokens: safeNum(config?.skills?.totalTokens),
  };
  const planUsage = session?.planUsage ?? null;
  const toolCalls = session?.toolCalls ?? [];
  const turns = session?.turns ?? [];

  const model = modelInfo.display;
  const sessionId = session?.sessionId ?? 'unknown';

  return {
    tool: 'codex',
    model,
    modelId: rawModel,
    modelTier: modelInfo.tier,
    modelReasoning: modelInfo.reasoning,
    isFast: modelInfo.isFast || false,
    sessionId,
    contextWindowSize,
    totalUsedPct,
    totalUsedTokens,
    freeTokens,
    components,
    compactionEvents,
    subagents,
    mcpServers,
    skills,
    planUsage,
    toolCalls,
    turns,
    timestamp: new Date().toISOString(),
  };
}

// ---------------------------------------------------------------------------
// simulateUsage
// ---------------------------------------------------------------------------

/**
 * Scale non-fixed components of an existing composition so that
 * totalUsedPct equals targetPct. Returns a new composition object.
 *
 * @param {object} composition — A composition from estimateClaudeContext or estimateCodexContext
 * @param {number} targetPct   — Target usage percentage (0–100)
 * @returns {object} New composition with scaled components
 */
export function simulateUsage(composition, targetPct) {
  if (targetPct < 0 || targetPct > 100) {
    throw new RangeError(`targetPct must be between 0 and 100, got ${targetPct}`);
  }

  const { contextWindowSize, components: origComponents, tool } = composition;
  const targetTokens = Math.round((targetPct / 100) * contextWindowSize);

  // Deep-clone components
  const components = origComponents.map((c) => ({ ...c }));

  // For Codex, separate the "free" component from content components
  const isFreeKey = (key) => key === 'free';
  const contentComponents = components.filter((c) => !isFreeKey(c.key));
  const freeComponent = components.find((c) => isFreeKey(c.key));

  // Sum of fixed components (cannot be scaled)
  const fixedTokens = contentComponents
    .filter((c) => c.fixed)
    .reduce((s, c) => s + c.tokens, 0);

  // Non-fixed, non-free components
  const scalable = contentComponents.filter((c) => !c.fixed);
  const scalableSum = scalable.reduce((s, c) => s + c.tokens, 0);

  // Target tokens for scalable portion
  const scalableTarget = Math.max(0, targetTokens - fixedTokens);

  if (scalableSum > 0) {
    const factor = scalableTarget / scalableSum;
    for (const c of scalable) {
      c.tokens = Math.round(c.tokens * factor);
      c.source = 'estimated'; // simulated values are estimates
    }
  } else if (scalableTarget > 0 && scalable.length > 0) {
    // All scalable components are zero — distribute evenly
    const perComponent = Math.round(scalableTarget / scalable.length);
    for (const c of scalable) {
      c.tokens = perComponent;
      c.source = 'estimated';
    }
  }

  // Recompute free space for Codex
  if (freeComponent) {
    const contentUsed = contentComponents.reduce((s, c) => s + c.tokens, 0);
    freeComponent.tokens = Math.max(0, contextWindowSize - contentUsed);
  }

  // Recompute percentages
  finalizePcts(components, contextWindowSize);

  const totalUsedTokens = contentComponents.reduce((s, c) => s + c.tokens, 0);
  const totalUsedPct = pct(totalUsedTokens, contextWindowSize);
  const freeTokens = Math.max(0, contextWindowSize - totalUsedTokens);

  return {
    ...composition,
    totalUsedPct,
    totalUsedTokens,
    freeTokens,
    components,
    timestamp: new Date().toISOString(),
  };
}
