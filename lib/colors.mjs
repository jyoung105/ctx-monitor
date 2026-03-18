// Color definitions shared between terminal + HTML rendering
// From SPEC Section 9 — Color Palette Reference

export const COMPONENTS = {
  claude: [
    { key: 'system',      label: 'System prompt',      color: '#534AB7', textColor: '#EEEDFE', ansi: 56,  fixed: true,  defaultTokens: 3200 },
    { key: 'tools',       label: 'Built-in tools',     color: '#0F6E56', textColor: '#E1F5EE', ansi: 29,  fixed: true,  defaultTokens: 17000 },
    { key: 'mcp',         label: 'MCP tools',          color: '#D85A30', textColor: '#FAECE7', ansi: 166, fixed: false, defaultTokens: 0 },
    { key: 'agents',      label: 'Agents',             color: '#378ADD', textColor: '#E6F1FB', ansi: 33,  fixed: false, defaultTokens: 0 },
    { key: 'memory',      label: 'Memory (CLAUDE.md)', color: '#D4537E', textColor: '#FBEAF0', ansi: 162, fixed: false, defaultTokens: 0 },
    { key: 'skill_meta',  label: 'Skill frontmatter',  color: '#3B6D11', textColor: '#EAF3DE', ansi: 64,  fixed: false, defaultTokens: 0 },
    { key: 'skill_body',  label: 'Skill body (active)',color: '#97C459', textColor: '#C0DD97', ansi: 107, fixed: false, defaultTokens: 0 },
    { key: 'plan',        label: 'Plan / todo',        color: '#1D9E75', textColor: '#9FE1CB', ansi: 36,  fixed: false, defaultTokens: 0 },
    { key: 'user_msg',    label: 'User messages',      color: '#BA7517', textColor: '#FAEEDA', ansi: 136, fixed: false, defaultTokens: 0 },
    { key: 'tool_results',label: 'Tool call results',  color: '#EF9F27', textColor: '#FAC775', ansi: 214, fixed: false, defaultTokens: 0 },
    { key: 'responses',   label: 'AI responses',       color: '#854F0B', textColor: '#EF9F27', ansi: 94,  fixed: false, defaultTokens: 0 },
    { key: 'subagent',    label: 'Subagent summaries',color: '#85B7EB', textColor: '#B5D4F4', ansi: 110, fixed: false, defaultTokens: 0 },
    { key: 'buffer',      label: 'Compact buffer',     color: '#B4B2A9', textColor: '#D3D1C7', ansi: 249, fixed: true,  defaultTokens: 45000 },
  ],
  codex: [
    { key: 'instructions',label: 'Instructions',       color: '#534AB7', textColor: '#EEEDFE', ansi: 56,  fixed: true,  defaultTokens: 2500 },
    { key: 'tools',       label: 'Built-in tools',     color: '#0F6E56', textColor: '#E1F5EE', ansi: 29,  fixed: true,  defaultTokens: 8000 },
    { key: 'mcp',         label: 'MCP tools',          color: '#D85A30', textColor: '#FAECE7', ansi: 166, fixed: false, defaultTokens: 0 },
    { key: 'agents',      label: 'AGENTS.md / memories',color:'#378ADD', textColor: '#E6F1FB', ansi: 33,  fixed: false, defaultTokens: 0 },
    { key: 'skills',      label: 'Skills',             color: '#639922', textColor: '#EAF3DE', ansi: 64,  fixed: false, defaultTokens: 0 },
    { key: 'plan',        label: 'Plan (update_plan)', color: '#1D9E75', textColor: '#9FE1CB', ansi: 36,  fixed: false, defaultTokens: 0 },
    { key: 'user_msg',    label: 'User messages',      color: '#BA7517', textColor: '#FAEEDA', ansi: 136, fixed: false, defaultTokens: 0 },
    { key: 'tool_results',label: 'Tool call results',  color: '#EF9F27', textColor: '#FAC775', ansi: 214, fixed: false, defaultTokens: 0 },
    { key: 'responses',   label: 'Codex responses',    color: '#854F0B', textColor: '#EF9F27', ansi: 94,  fixed: false, defaultTokens: 0 },
    { key: 'subagent',    label: 'Subagent summaries', color: '#85B7EB', textColor: '#B5D4F4', ansi: 110, fixed: false, defaultTokens: 0 },
    { key: 'reasoning',   label: 'Reasoning tokens',   color: '#888780', textColor: '#D3D1C7', ansi: 245, fixed: false, defaultTokens: 0 },
    { key: 'free',        label: 'Free space',         color: '#E8E6DF', textColor: '#F1EFE8', ansi: 254, fixed: false, defaultTokens: 0 },
  ],
};

// ANSI terminal color helpers
export function ansiBg(ansi256) {
  return `\x1b[48;5;${ansi256}m`;
}

export function ansiFg(ansi256) {
  return `\x1b[38;5;${ansi256}m`;
}

export const RESET = '\x1b[0m';
export const BOLD = '\x1b[1m';
export const DIM = '\x1b[2m';

// Brand themes for HTML dashboard
export const THEMES = {
  claude: {
    name: 'Claude Code',
    brand500: '#D4603A',
    brand600: '#B84A28',
    surface0: '#FFFFFF',
    surface1: '#F8F7F5',
    surface2: '#EEEDEB',
    textPrimary: '#252422',
    textSecondary: '#6B6862',
    borderDefault: '#D8D6D2',
  },
  codex: {
    name: 'Codex CLI',
    brand500: '#10A37F',
    brand600: '#0D8A6A',
    surface0: '#FFFFFF',
    surface1: '#FAFAF9',
    surface2: '#F5F5F4',
    textPrimary: '#292524',
    textSecondary: '#78716C',
    borderDefault: '#E7E5E4',
  },
};

export function estimateTokens(text) {
  if (!text) return 0;
  if (typeof text !== 'string') text = JSON.stringify(text);
  return Math.ceil(text.length / 4);
}

// ── Model Registry ──────────────────────────────────────────────────
// Maps model IDs to display names, context windows, and capabilities.
// Both Claude and Codex models, including /fast variants.

export const MODEL_REGISTRY = {
  // ── Claude Code models ──
  // Opus 4.6 — flagship, deep reasoning
  'claude-opus-4-6':              { display: 'Claude Opus 4.6',           context: 200_000, family: 'claude', tier: 'opus',   reasoning: 'high' },
  'claude-opus-4-6[1m]':         { display: 'Claude Opus 4.6 (1M)',      context: 1_000_000, family: 'claude', tier: 'opus',   reasoning: 'high' },
  'claude-opus-4-6-20250610':    { display: 'Claude Opus 4.6',           context: 200_000, family: 'claude', tier: 'opus',   reasoning: 'high' },
  // Sonnet 4.6 — balanced performance
  'claude-sonnet-4-6':           { display: 'Claude Sonnet 4.6',         context: 200_000, family: 'claude', tier: 'sonnet', reasoning: 'medium' },
  'claude-sonnet-4-6-20250514':  { display: 'Claude Sonnet 4.6',         context: 200_000, family: 'claude', tier: 'sonnet', reasoning: 'medium' },
  // Haiku 4.5 — fast, lightweight
  'claude-haiku-4-5':            { display: 'Claude Haiku 4.5',          context: 200_000, family: 'claude', tier: 'haiku',  reasoning: 'low' },
  'claude-haiku-4-5-20251001':   { display: 'Claude Haiku 4.5',          context: 200_000, family: 'claude', tier: 'haiku',  reasoning: 'low' },
  // Legacy / older
  'claude-sonnet-4-20250514':    { display: 'Claude Sonnet 4',           context: 200_000, family: 'claude', tier: 'sonnet', reasoning: 'medium' },
  'claude-opus-4-20250514':      { display: 'Claude Opus 4',             context: 200_000, family: 'claude', tier: 'opus',   reasoning: 'high' },
  'claude-3-5-sonnet-20241022':  { display: 'Claude 3.5 Sonnet',         context: 200_000, family: 'claude', tier: 'sonnet', reasoning: 'medium' },
  'claude-3-5-haiku-20241022':   { display: 'Claude 3.5 Haiku',          context: 200_000, family: 'claude', tier: 'haiku',  reasoning: 'low' },
  'claude-3-opus-20240229':      { display: 'Claude 3 Opus',             context: 200_000, family: 'claude', tier: 'opus',   reasoning: 'high' },

  // ── Codex CLI / OpenAI models ──
  // GPT-5.4 — latest flagship
  'gpt-5.4':                     { display: 'GPT-5.4',                   context: 256_000, family: 'codex', tier: 'flagship', reasoning: 'high' },
  'gpt-5.4-mini':                { display: 'GPT-5.4 Mini',              context: 128_000, family: 'codex', tier: 'mini',     reasoning: 'medium' },
  // GPT-5.3 — codex optimized
  'gpt-5.3-codex':               { display: 'GPT-5.3 Codex',             context: 256_000, family: 'codex', tier: 'codex',    reasoning: 'high' },
  // GPT-5.2 — previous gen
  'gpt-5.2':                     { display: 'GPT-5.2',                   context: 200_000, family: 'codex', tier: 'flagship', reasoning: 'high' },
  'gpt-5.2-codex':               { display: 'GPT-5.2 Codex',             context: 200_000, family: 'codex', tier: 'codex',    reasoning: 'high' },
  // GPT-5.1 — codex variants
  'gpt-5.1-codex-max':           { display: 'GPT-5.1 Codex Max',         context: 256_000, family: 'codex', tier: 'max',      reasoning: 'xhigh' },
  'gpt-5.1-codex-mini':          { display: 'GPT-5.1 Codex Mini',        context: 128_000, family: 'codex', tier: 'mini',     reasoning: 'medium' },
  // Legacy OpenAI
  'o3':                          { display: 'o3',                         context: 200_000, family: 'codex', tier: 'reasoning', reasoning: 'xhigh' },
  'o3-mini':                     { display: 'o3 Mini',                    context: 200_000, family: 'codex', tier: 'mini',     reasoning: 'high' },
  'o4-mini':                     { display: 'o4 Mini',                    context: 200_000, family: 'codex', tier: 'mini',     reasoning: 'high' },
  'gpt-4.1':                     { display: 'GPT-4.1',                   context: 128_000, family: 'codex', tier: 'flagship', reasoning: 'medium' },
  'gpt-4.1-mini':                { display: 'GPT-4.1 Mini',              context: 128_000, family: 'codex', tier: 'mini',     reasoning: 'low' },
  'gpt-4o':                      { display: 'GPT-4o',                    context: 128_000, family: 'codex', tier: 'flagship', reasoning: 'medium' },
  'gpt-4o-mini':                 { display: 'GPT-4o Mini',               context: 128_000, family: 'codex', tier: 'mini',     reasoning: 'low' },
};

/**
 * Look up a model by its ID string, handling /fast suffix and partial matches.
 * Returns { display, context, family, tier, reasoning, isFast }.
 */
export function resolveModel(modelId) {
  if (!modelId) return { display: 'Unknown', context: 200_000, family: 'unknown', tier: 'unknown', reasoning: 'medium', isFast: false };

  // Handle /fast suffix
  const isFast = modelId.includes('/fast') || modelId.includes('-fast');
  const baseId = modelId.replace(/\/fast$/, '').replace(/-fast$/, '');

  // Strip [1m] for lookup but remember it
  const is1M = baseId.includes('[1m]') || baseId.includes('(1m)');
  const lookupId = baseId.replace(/\[1m\]/, '').replace(/\(1m\)/, '').trim();

  // Direct match
  if (MODEL_REGISTRY[baseId]) {
    const m = { ...MODEL_REGISTRY[baseId], isFast };
    if (isFast) m.display += ' (Fast)';
    return m;
  }

  // Match without [1m] then override context
  if (MODEL_REGISTRY[lookupId]) {
    const m = { ...MODEL_REGISTRY[lookupId], isFast };
    if (is1M) { m.context = 1_000_000; m.display += ' (1M)'; }
    if (isFast) m.display += ' (Fast)';
    return m;
  }

  // Prefix match — find longest matching prefix
  let bestMatch = null;
  let bestLen = 0;
  for (const [key, val] of Object.entries(MODEL_REGISTRY)) {
    if (lookupId.startsWith(key) && key.length > bestLen) {
      bestMatch = val;
      bestLen = key.length;
    }
  }
  if (bestMatch) {
    const m = { ...bestMatch, isFast };
    if (is1M) { m.context = 1_000_000; m.display += ' (1M)'; }
    if (isFast) m.display += ' (Fast)';
    return m;
  }

  // Fallback: infer family from name
  const family = lookupId.startsWith('claude') ? 'claude' : lookupId.startsWith('gpt') || lookupId.startsWith('o3') || lookupId.startsWith('o4') ? 'codex' : 'unknown';
  return {
    display: modelId,
    context: family === 'codex' ? 128_000 : 200_000,
    family,
    tier: 'unknown',
    reasoning: 'medium',
    isFast,
  };
}
