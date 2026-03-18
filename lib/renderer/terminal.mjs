// ANSI terminal rendering for ctx-monitor
// Renders context composition data as colored terminal output

import { COMPONENTS, ansiBg, ansiFg, RESET, BOLD, DIM } from '../colors.mjs';

// ── Helper functions ────────────────────────────────────────────────

/**
 * Format token count as human-readable string: "3.2k", "104.6k", "1.2M"
 */
export function formatTokens(n) {
  if (n == null || isNaN(n)) return '0';
  if (n >= 1_000_000) {
    const val = n / 1_000_000;
    return val % 1 === 0 ? `${val}M` : `${val.toFixed(1)}M`;
  }
  if (n >= 1_000) {
    const val = n / 1_000;
    return val % 1 === 0 ? `${val}k` : `${val.toFixed(1)}k`;
  }
  return String(n);
}

/**
 * Format percentage: "52.3%"
 */
export function formatPct(n) {
  if (n == null || isNaN(n)) return '0.0%';
  return `${n.toFixed(1)}%`;
}

/**
 * Format number with commas: 104600 → "104,600"
 */
export function numberWithCommas(n) {
  if (n == null || isNaN(n)) return '0';
  return n.toLocaleString('en-US');
}

/**
 * Return ANSI color code based on usage percentage.
 * green < 50%, yellow 50-75%, red >= 75%
 */
export function pctColor(pct, noColor = false) {
  if (noColor) return '';
  if (pct >= 75) return '\x1b[31m'; // red
  if (pct >= 50) return '\x1b[33m'; // yellow
  return '\x1b[32m'; // green
}

/**
 * Strip all ANSI escape codes from a string.
 */
function stripAnsi(str) {
  return str.replace(/\x1b\[[0-9;]*m/g, '');
}

/**
 * Conditionally apply ANSI wrapper — returns empty strings when noColor is set.
 */
function c(code, noColor) {
  return noColor ? '' : code;
}

/**
 * Pad/truncate string to exact visible width (ignoring ANSI codes).
 */
function padEnd(str, width) {
  const visible = stripAnsi(str).length;
  if (visible >= width) return str;
  return str + ' '.repeat(width - visible);
}

function padStart(str, width) {
  const visible = stripAnsi(str).length;
  if (visible >= width) return str;
  return ' '.repeat(width - visible) + str;
}

// ── Composition data helpers ────────────────────────────────────────

/**
 * Get the component list for a composition, with calculated percentages.
 * Each item: { key, label, tokens, pct, color, ansi, fixed }
 */
function getComponents(composition) {
  const tool = composition.tool || 'claude';
  const defs = COMPONENTS[tool] || COMPONENTS.claude;
  const windowSize = composition.contextWindowSize || composition.windowSize || 200000;
  const components = composition.components || {};

  // If components is an array (from estimator), convert to a map by key
  let compMap = components;
  if (Array.isArray(components)) {
    compMap = {};
    for (const c of components) {
      compMap[c.key] = c.tokens;
    }
  }

  return defs
    .filter(d => d.key !== 'free')
    .map(d => {
      const tokens = compMap[d.key] ?? d.defaultTokens ?? 0;
      return {
        key: d.key,
        label: d.label,
        tokens,
        pct: windowSize > 0 ? (tokens / windowSize) * 100 : 0,
        color: d.color,
        ansi: d.ansi,
        fixed: d.fixed,
      };
    });
}

function getTotalUsed(composition) {
  // Prefer apiMatchTokens (excludes buffer, matches Claude Code HUD)
  if (composition.apiMatchTokens != null) return composition.apiMatchTokens;
  const comps = getComponents(composition);
  return comps.filter(c => c.key !== 'buffer').reduce((sum, c) => sum + c.tokens, 0);
}

function getWindowSize(composition) {
  return composition.contextWindowSize || composition.windowSize || 200000;
}

// ── 1. renderBar ────────────────────────────────────────────────────

/**
 * Render a full-width horizontal stacked bar using ANSI 256 colors.
 */
export function renderBar(composition, width, { noColor = false } = {}) {
  const termWidth = width || process.stdout.columns || 80;
  const comps = getComponents(composition).filter(c => c.tokens > 0);
  const windowSize = getWindowSize(composition);
  const totalUsed = comps.reduce((sum, c) => sum + c.tokens, 0);
  const usedPct = windowSize > 0 ? (totalUsed / windowSize) * 100 : 0;

  // Calculate character counts for each component
  const barWidth = termWidth;
  let segments = comps.map(comp => {
    const chars = Math.round((comp.tokens / windowSize) * barWidth);
    return { ...comp, chars };
  });

  // Ensure total allocated chars doesn't exceed barWidth
  const usedChars = segments.reduce((s, seg) => s + seg.chars, 0);
  const freeChars = Math.max(0, barWidth - usedChars);

  let bar = '';
  for (const seg of segments) {
    if (seg.chars <= 0) continue;
    const label = seg.pct > 5 ? formatPct(seg.pct) : '';
    const block = '█'.repeat(seg.chars);

    if (noColor) {
      // Embed label in the middle of the block
      if (label && seg.chars > label.length + 2) {
        const mid = Math.floor((seg.chars - label.length) / 2);
        bar += '█'.repeat(mid) + label + '█'.repeat(seg.chars - mid - label.length);
      } else {
        bar += block;
      }
    } else {
      if (label && seg.chars > label.length + 2) {
        const mid = Math.floor((seg.chars - label.length) / 2);
        bar += ansiBg(seg.ansi) + ansiFg(15) + '█'.repeat(mid) + label + '█'.repeat(seg.chars - mid - label.length) + RESET;
      } else {
        bar += ansiFg(seg.ansi) + block + RESET;
      }
    }
  }

  // Free space
  if (freeChars > 0) {
    if (noColor) {
      bar += '░'.repeat(freeChars);
    } else {
      bar += DIM + '░'.repeat(freeChars) + RESET;
    }
  }

  return bar;
}

// ── 2. renderTable ──────────────────────────────────────────────────

/**
 * Render a component breakdown table with colored dots and mini bars.
 */
export function renderTable(composition, { noColor = false } = {}) {
  const comps = getComponents(composition);
  const windowSize = getWindowSize(composition);
  const totalUsed = getTotalUsed(composition);
  const freeTokens = Math.max(0, windowSize - totalUsed);
  const usedPct = windowSize > 0 ? (totalUsed / windowSize) * 100 : 0;
  const freePct = windowSize > 0 ? (freeTokens / windowSize) * 100 : 0;
  const maxBarWidth = 20;

  const lines = [];
  const header = `  ${padEnd('Component', 22)} ${padStart('Tokens', 10)} ${padStart('%', 7)}  Bar`;
  lines.push(header);
  lines.push('  ' + '━'.repeat(52));

  for (const comp of comps) {
    if (comp.tokens === 0) continue;
    const dot = noColor ? '●' : `${ansiFg(comp.ansi)}●${RESET}`;
    const label = padEnd(comp.label, 20);
    const tokens = padStart(numberWithCommas(comp.tokens), 10);
    const pct = padStart(formatPct(comp.pct), 6);
    const barLen = Math.max(0, Math.round((comp.pct / 100) * maxBarWidth));
    const miniBar = noColor
      ? '█'.repeat(barLen)
      : `${ansiFg(comp.ansi)}${'█'.repeat(barLen)}${RESET}`;
    lines.push(`  ${dot} ${label} ${tokens} ${pct}  ${miniBar}`);
  }

  lines.push('  ' + '━'.repeat(52));

  // Total line
  const totalLabel = noColor
    ? padEnd('Total used', 22)
    : `${BOLD}${padEnd('Total used', 22)}${RESET}`;
  const totalTokensFmt = noColor
    ? padStart(numberWithCommas(totalUsed), 10)
    : `${BOLD}${padStart(numberWithCommas(totalUsed), 10)}${RESET}`;
  const totalPctFmt = padStart(formatPct(usedPct), 6);
  lines.push(`  ${totalLabel} ${totalTokensFmt} ${totalPctFmt}`);

  // Free line
  lines.push(`  ${padEnd('Free', 22)} ${padStart(numberWithCommas(freeTokens), 10)} ${padStart(formatPct(freePct), 6)}`);

  // Context window line
  lines.push(`  ${padEnd('Context window', 22)} ${padStart(numberWithCommas(windowSize), 10)}`);

  const dim = c(DIM, noColor);
  const bold = c(BOLD, noColor);
  const reset = c(RESET, noColor);

  // ── Memory Files detail ──
  const memoryFiles = composition.memoryFiles || [];
  if (memoryFiles.length > 0) {
    const memTotal = memoryFiles.reduce((s, f) => s + (f.tokens || 0), 0);
    lines.push('');
    lines.push(`  ${bold}Memory Files (${memoryFiles.length}, ~${formatTokens(memTotal)} total)${reset}`);
    lines.push('  ' + '─'.repeat(52));
    for (let i = 0; i < memoryFiles.length; i++) {
      const f = memoryFiles[i];
      const conn = i === memoryFiles.length - 1 ? '└' : '├';
      const fp = shortenPath(f.path || '');
      lines.push(`  ${conn} ${padEnd(fp, 40)} ${dim}~${formatTokens(f.tokens || 0)} (${numberWithCommas(f.chars || 0)} chars)${reset}`);
    }
  }

  // ── Agent Files detail ──
  const agentFiles = composition.agentFiles || [];
  if (agentFiles.length > 0) {
    const agentTotal = agentFiles.reduce((s, f) => s + (f.tokens || 0), 0);
    lines.push('');
    lines.push(`  ${bold}Agent Definitions (${agentFiles.length}, ~${formatTokens(agentTotal)} total)${reset}`);
    lines.push('  ' + '─'.repeat(52));
    for (let i = 0; i < agentFiles.length; i++) {
      const a = agentFiles[i];
      const conn = i === agentFiles.length - 1 ? '└' : '├';
      lines.push(`  ${conn} ${padEnd(a.name || 'unknown', 25)} ${dim}~${formatTokens(a.tokens || 0)}${reset}`);
    }
  }

  // ── MCP Servers detail ──
  const mcpServers = composition.mcpServers || [];
  if (mcpServers.length > 0) {
    const mcpTotal = mcpServers.reduce((s, srv) => s + (srv.estimatedTokens || 0), 0);
    lines.push('');
    lines.push(`  ${bold}MCP Servers (${mcpServers.length}, ~${formatTokens(mcpTotal)} total)${reset}`);
    lines.push('  ' + '─'.repeat(52));
    for (let i = 0; i < mcpServers.length; i++) {
      const srv = mcpServers[i];
      const conn = i === mcpServers.length - 1 ? '└' : '├';
      const name = padEnd(srv.name || 'unknown', 20);
      const tools = padStart(String(srv.toolCount || '?'), 3) + ' tools';
      const tokens = padStart('~' + formatTokens(srv.estimatedTokens || 0), 8);
      lines.push(`  ${conn} ${name} ${tools}  ${tokens}`);
    }
  }

  // ── Subagent Spawns detail ──
  const subagents = composition.subagents || [];
  if (subagents.length > 0) {
    lines.push('');
    lines.push(`  ${bold}Subagents / Agents (${subagents.length})${reset}`);
    lines.push('  ' + '─'.repeat(52));
    for (let i = 0; i < subagents.length; i++) {
      const sa = subagents[i];
      const conn = i === subagents.length - 1 ? '└' : '├';
      const type = sa.subagentType || sa.tool || 'agent';
      const desc = sa.description ? truncate(sa.description, 35) : '';
      const tok = sa.tokensReturned ? `[~${formatTokens(sa.tokensReturned)} returned]` : '';
      lines.push(`  ${conn} ${padEnd(type, 14)} ${padEnd(desc, 36)} ${dim}${tok}${reset}`);
    }
  }

  // ── Skill detail ──
  const skills = composition.skills || {};
  const activeSkills = skills.active || [];
  const skillFiles = skills.files || [];
  if (skillFiles.length > 0 || activeSkills.length > 0) {
    lines.push('');
    lines.push(`  ${bold}Skills (${skills.installed || 0} installed, ${activeSkills.length} active, ~${formatTokens(skills.frontmatterTokens || 0)} frontmatter)${reset}`);
    lines.push('  ' + '─'.repeat(52));
    if (activeSkills.length > 0) {
      for (const sk of activeSkills) {
        lines.push(`  ${noColor ? '●' : `\x1b[38;5;107m●${RESET}`} ${sk}  ${dim}[active in context]${reset}`);
      }
      if (skills.bodyTokens > 0) {
        lines.push(`  ${dim}  active body: ~${formatTokens(skills.bodyTokens)}${reset}`);
      }
      lines.push('  ' + '─'.repeat(30));
    }
    // Show individual skill frontmatter tokens
    for (let i = 0; i < skillFiles.length; i++) {
      const sf = skillFiles[i];
      const conn = i === skillFiles.length - 1 ? '└' : '├';
      const isActive = activeSkills.includes(sf.name);
      const marker = isActive ? ` ${dim}[active]${reset}` : '';
      lines.push(`  ${conn} ${padEnd(sf.name || '?', 30)} ${dim}~${formatTokens(sf.frontmatterTokens || 0)}${reset}${marker}`);
    }
  }

  // ── Tool Calls summary ──
  const toolCalls = composition.toolCalls || [];
  if (toolCalls.length > 0) {
    lines.push('');
    lines.push(renderToolCallsDetail(toolCalls, { noColor }));
  }

  return lines.join('\n');
}

/**
 * Truncate a string to maxLen, adding … if needed.
 */
function truncate(str, maxLen) {
  if (!str) return '';
  if (str.length <= maxLen) return str;
  return str.substring(0, maxLen - 1) + '…';
}

/**
 * Extract a short display target from a tool call's input.
 */
function toolCallDetail(call) {
  const name = call.name || call.tool || 'Unknown';
  const input = call.input || {};

  switch (name) {
    case 'Read': {
      const fp = shortenPath(input.file_path || '');
      const range = input.offset ? `:${input.offset}` + (input.limit ? `-${input.offset + input.limit}` : '') : '';
      return { target: fp + range, extra: '' };
    }
    case 'Edit':
    case 'MultiEdit':
      return { target: shortenPath(input.file_path || ''), extra: 'edit' };
    case 'Write':
      return { target: shortenPath(input.file_path || ''), extra: 'create/write' };
    case 'Grep':
      return { target: `"${truncate(input.pattern || '', 20)}" ${input.glob || input.path || ''}`, extra: '' };
    case 'Glob':
      return { target: input.pattern || '', extra: '' };
    case 'Bash': {
      const cmd = truncate(input.command || '', 45);
      return { target: cmd, extra: '' };
    }
    case 'Agent':
    case 'Task': {
      const type = input.subagent_type || '';
      const desc = truncate(input.description || input.prompt || '', 30);
      return { target: `${type} "${desc}"`, extra: 'isolated' };
    }
    case 'Skill':
      return { target: input.skill || input.skill_name || '', extra: 'skill loaded' };
    case 'TodoWrite':
    case 'TodoRead':
      return { target: 'plan/todo', extra: '' };
    case 'WebFetch':
      return { target: truncate(input.url || '', 40), extra: '' };
    case 'WebSearch':
      return { target: truncate(input.query || '', 40), extra: '' };
    default:
      // MCP tools: mcp__server__tool
      if (name.startsWith('mcp__')) {
        const parts = name.split('__');
        return { target: `${parts[1]}/${parts.slice(2).join('__')}`, extra: 'mcp' };
      }
      return { target: '', extra: '' };
  }
}

/**
 * Shorten a file path for display.
 */
function shortenPath(fp) {
  if (!fp) return '';
  // Remove common home prefix
  const home = process.env.HOME || '';
  if (home && fp.startsWith(home)) {
    fp = '~' + fp.slice(home.length);
  }
  // If still long, show just the last 2-3 segments
  if (fp.length > 50) {
    const parts = fp.split('/');
    if (parts.length > 3) {
      fp = '…/' + parts.slice(-3).join('/');
    }
  }
  return fp;
}

/**
 * Render detailed tool call log with file:line, aggregation, and per-call detail.
 */
function renderToolCallsDetail(toolCalls, { noColor = false } = {}) {
  const bold = c(BOLD, noColor);
  const dim = c(DIM, noColor);
  const reset = c(RESET, noColor);

  const lines = [];
  lines.push(`  ${bold}Tool Calls (${toolCalls.length} total)${reset}`);
  lines.push('  ' + '─'.repeat(52));

  // Aggregation by tool name
  const byName = {};
  const tokensByName = {};
  for (const tc of toolCalls) {
    const n = tc.name || tc.tool || 'Unknown';
    byName[n] = (byName[n] || 0) + 1;
    tokensByName[n] = (tokensByName[n] || 0) + (tc.tokenEstimate || tc.tokens || 0);
  }

  // Sort by count descending
  const sorted = Object.entries(byName).sort((a, b) => b[1] - a[1]);
  const maxCount = sorted[0]?.[1] || 1;
  const barScale = 20;

  for (const [name, count] of sorted) {
    const barLen = Math.max(1, Math.round((count / maxCount) * barScale));
    const bar = '█'.repeat(barLen);
    const tok = tokensByName[name] || 0;
    lines.push(`  ${padEnd(name, 10)} ${noColor ? bar : `${ansiFg(214)}${bar}${RESET}`} ${padStart(String(count), 3)} calls  ${dim}(${formatTokens(tok)} tokens)${reset}`);
  }

  lines.push('  ' + '─'.repeat(52));

  // Per-call detail (show all, grouped chronologically)
  for (let i = 0; i < toolCalls.length; i++) {
    const tc = toolCalls[i];
    const conn = i === toolCalls.length - 1 ? '└' : i === 0 ? '┌' : '├';
    const name = padEnd(tc.name || tc.tool || '?', 7);
    const { target, extra } = toolCallDetail(tc);
    const tokEst = tc.tokenEstimate || tc.tokens || 0;
    const tokStr = tokEst > 0 ? `[~${formatTokens(tokEst)}]` : '';
    const extraStr = extra ? ` ${dim}(${extra})${reset}` : '';

    lines.push(`  ${conn} ${name}${padEnd(truncate(target, 38), 38)} ${dim}${tokStr}${reset}${extraStr}`);
  }

  return lines.join('\n');
}

// ── 3. renderOrder ──────────────────────────────────────────────────

/**
 * Render context loading order diagram.
 */
export function renderOrder(composition, { noColor = false } = {}) {
  const comps = getComponents(composition);
  const windowSize = getWindowSize(composition);

  // Split into pre-message (fixed/config) and post-message (dynamic) components
  const preMessage = ['system', 'instructions', 'tools', 'mcp', 'agents', 'memory', 'skill_meta', 'skill_body', 'plan', 'skills'];
  const postMessage = ['user_msg', 'tool_results', 'responses', 'subagent', 'reasoning'];
  const bufferKeys = ['buffer'];

  const lines = [];
  const title = noColor ? '  Context Loading Order' : `  ${BOLD}Context Loading Order${RESET}`;
  lines.push(title);
  lines.push('  ' + '━'.repeat(22));

  let idx = 1;
  let insertedUserSep = false;
  let insertedCompactSep = false;

  for (const comp of comps) {
    if (comp.tokens === 0) continue;

    // Insert separator before first post-message component
    if (!insertedUserSep && postMessage.includes(comp.key)) {
      insertedUserSep = true;
      const sep = noColor
        ? '  ── user types first message ──'
        : `  ${DIM}── user types first message ──${RESET}`;
      lines.push(sep);
    }

    // Insert separator before buffer
    if (!insertedCompactSep && bufferKeys.includes(comp.key)) {
      insertedCompactSep = true;
      const triggerAt = Math.round(windowSize * 0.775);
      const sep = noColor
        ? `  ── compaction trigger at ~${formatTokens(triggerAt)} ──`
        : `  ${DIM}── compaction trigger at ~${formatTokens(triggerAt)} ──${RESET}`;
      lines.push(sep);
    }

    const fixedTag = comp.fixed ? ' (fixed)' : '';
    const num = String(idx).padStart(2, ' ');
    const label = padEnd(comp.label, 24);
    const tokens = `${numberWithCommas(comp.tokens)} tokens${fixedTag}`;
    lines.push(`  ${num}. ${label} ${tokens}`);
    idx++;
  }

  return lines.join('\n');
}

// ── 4. renderAgents ─────────────────────────────────────────────────

/**
 * Render subagent/team isolation diagram.
 */
export function renderAgents(composition, { noColor = false } = {}) {
  const windowSize = getWindowSize(composition);
  const totalUsed = getTotalUsed(composition);
  const usedPct = windowSize > 0 ? (totalUsed / windowSize) * 100 : 0;
  const agents = composition.agents || [];

  const bold = c(BOLD, noColor);
  const dim = c(DIM, noColor);
  const reset = c(RESET, noColor);

  const lines = [];
  lines.push(`  ${bold}Subagent Isolation${reset}`);
  lines.push('  ' + '━'.repeat(20));

  const boxW = 48;
  lines.push(`  ┌─ Main Session (${formatTokens(windowSize)} window) ${'─'.repeat(Math.max(0, boxW - 26 - formatTokens(windowSize).length))}┐`);
  lines.push(`  │  Context: ${numberWithCommas(totalUsed)} tokens (${formatPct(usedPct)})${' '.repeat(Math.max(1, boxW - 22 - numberWithCommas(totalUsed).length - formatPct(usedPct).length))}│`);
  lines.push(`  │${' '.repeat(boxW)}│`);

  if (agents.length === 0) {
    lines.push(`  │  ${dim}No active subagents${reset}${' '.repeat(Math.max(1, boxW - 22))}│`);
  } else {
    for (const agent of agents) {
      const name = agent.name || 'Unknown';
      const task = agent.task || '';
      const returned = agent.returnedTokens || 0;
      const innerW = 38;

      const headerText = task ? `${name} "${task}"` : name;
      const header = headerText.length > innerW - 4
        ? headerText.substring(0, innerW - 7) + '...'
        : headerText;

      lines.push(`  │  ┌─ ${padEnd(header, innerW - 4)} ─┐${' '.repeat(Math.max(1, boxW - innerW - 5))}│`);
      lines.push(`  │  │  Isolated ${formatTokens(windowSize)} window${' '.repeat(Math.max(1, innerW - 20 - formatTokens(windowSize).length))}│${' '.repeat(Math.max(1, boxW - innerW - 5))}│`);
      lines.push(`  │  │  Returned: ~${numberWithCommas(returned)} tokens summary${' '.repeat(Math.max(0, innerW - 30 - numberWithCommas(returned).length))}│${' '.repeat(Math.max(1, boxW - innerW - 5))}│`);
      lines.push(`  │  └${'─'.repeat(innerW - 2)}┘${' '.repeat(Math.max(1, boxW - innerW - 5))}│`);
      lines.push(`  │${' '.repeat(boxW)}│`);
    }
  }

  lines.push(`  └${'─'.repeat(boxW)}┘`);

  return lines.join('\n');
}

// ── 5. renderTimeline ───────────────────────────────────────────────

/**
 * Render context growth timeline chart.
 */
export function renderTimeline(composition, { noColor = false } = {}) {
  const windowSize = getWindowSize(composition);
  const snapshots = composition.timeline || [];

  const bold = c(BOLD, noColor);
  const dim = c(DIM, noColor);
  const reset = c(RESET, noColor);

  const lines = [];
  lines.push(`  ${bold}Context Growth Timeline${reset}`);
  lines.push('  ' + '━'.repeat(25));

  if (snapshots.length === 0) {
    lines.push(`  ${dim}No timeline data available${reset}`);
    return lines.join('\n');
  }

  // Build simple ASCII chart
  const chartHeight = 8;
  const chartWidth = Math.min(snapshots.length, 40);
  const step = Math.max(1, Math.floor(snapshots.length / chartWidth));

  const sampled = [];
  for (let i = 0; i < snapshots.length; i += step) {
    sampled.push(snapshots[i]);
  }

  const yLabels = [];
  const yStep = windowSize / 4;
  for (let i = 4; i >= 0; i--) {
    yLabels.push(formatTokens(Math.round(yStep * i)));
  }
  const yLabelWidth = Math.max(...yLabels.map(l => l.length)) + 1;

  // Normalize values to chart height
  const rows = [];
  for (let row = chartHeight; row >= 0; row--) {
    const threshold = (row / chartHeight) * windowSize;
    let line = padStart(row === chartHeight ? formatTokens(windowSize) : row === 0 ? '0' : row % 2 === 0 ? formatTokens(Math.round(threshold)) : '', yLabelWidth);
    line += ' ┤';

    for (let col = 0; col < sampled.length; col++) {
      const tokens = sampled[col].tokens || sampled[col].total || 0;
      const compaction = sampled[col].compaction || false;

      if (tokens >= threshold) {
        if (compaction && row === chartHeight) {
          line += '╮';
        } else {
          // Check if this is the "top" of the bar
          const nextThreshold = ((row + 1) / chartHeight) * windowSize;
          if (tokens < nextThreshold || row === chartHeight) {
            line += compaction ? '╭' : '╭';
          } else {
            line += '│';
          }
        }
      } else {
        line += ' ';
      }
    }
    rows.push(line);
  }

  // Build the graph using line drawing
  for (const row of rows) {
    lines.push(`  ${row}`);
  }

  // X-axis
  const xAxis = ' '.repeat(yLabelWidth) + ' └' + '─'.repeat(sampled.length);
  lines.push(`  ${xAxis}`);

  // Time labels
  if (sampled.length > 0 && sampled[0].time) {
    let timeLine = ' '.repeat(yLabelWidth + 2);
    const labelInterval = Math.max(1, Math.floor(sampled.length / 5));
    for (let i = 0; i < sampled.length; i++) {
      if (i % labelInterval === 0) {
        const t = sampled[i].time || '';
        const short = typeof t === 'string' ? t.substring(0, 5) : '';
        timeLine += short;
        i += short.length - 1;
      } else {
        timeLine += ' ';
      }
    }
    lines.push(`  ${dim}${timeLine}${reset}`);
  }

  return lines.join('\n');
}

// ── 6. renderToolCalls ──────────────────────────────────────────────

/**
 * Render tool call log with file:line detail.
 */
export function renderToolCalls(composition, { noColor = false } = {}) {
  const toolCalls = composition.toolCalls || [];

  const bold = c(BOLD, noColor);
  const dim = c(DIM, noColor);
  const reset = c(RESET, noColor);

  const lines = [];
  lines.push(`  ${bold}Tool Calls (${toolCalls.length} total)${reset}`);
  lines.push('  ' + '━'.repeat(23));

  if (toolCalls.length === 0) {
    lines.push(`  ${dim}No tool calls recorded${reset}`);
    return lines.join('\n');
  }

  for (let i = 0; i < toolCalls.length; i++) {
    const call = toolCalls[i];
    const connector = i === toolCalls.length - 1 ? '└' : i === 0 ? '┌' : '├';
    const name = padEnd(call.tool || call.name || 'Unknown', 6);
    const detail = call.detail || call.description || '';
    const tokensInfo = call.tokens
      ? `[~${formatTokens(call.tokens)} tokens${call.extra ? ', ' + call.extra : ''}]`
      : call.extra ? `[${call.extra}]` : '';

    const tokensFmt = noColor
      ? tokensInfo
      : `${dim}${tokensInfo}${reset}`;

    lines.push(`  ${connector} ${name} ${padEnd(detail, 30)} ${tokensFmt}`);
  }

  return lines.join('\n');
}

// ── 7. renderCompact ────────────────────────────────────────────────

/**
 * Single-line compact output.
 * Example: Claude Sonnet 4 │ 52.3% │ ████████████████████░░░░░░░░░░░░░░░░░░░░ │ 104.6k/200k
 */
export function renderCompact(composition, { noColor = false } = {}) {
  const windowSize = getWindowSize(composition);
  const totalUsed = getTotalUsed(composition);
  const usedPct = windowSize > 0 ? (totalUsed / windowSize) * 100 : 0;
  const model = composition.model || composition.tool || 'Claude';

  const barWidth = 40;
  const filledChars = Math.round((usedPct / 100) * barWidth);
  const emptyChars = barWidth - filledChars;

  const colorStart = noColor ? '' : pctColor(usedPct);
  const reset = c(RESET, noColor);

  const bar = `${colorStart}${'█'.repeat(filledChars)}${reset}${'░'.repeat(emptyChars)}`;
  const pct = formatPct(usedPct);
  const usage = `${formatTokens(totalUsed)}/${formatTokens(windowSize)}`;

  return `${model} │ ${pct} │ ${bar} │ ${usage}`;
}

// ── 8. renderStatusline ─────────────────────────────────────────────

/**
 * Ultra-compact for statusline bridge.
 * Example: 52% ████████████████████░░░░░░░░░░░░░░░░░░░░ 104.6k
 */
export function renderStatusline(composition, { noColor = false } = {}) {
  const windowSize = getWindowSize(composition);
  const totalUsed = getTotalUsed(composition);
  const usedPct = windowSize > 0 ? (totalUsed / windowSize) * 100 : 0;

  const barWidth = 40;
  const filledChars = Math.round((usedPct / 100) * barWidth);
  const emptyChars = barWidth - filledChars;

  const colorStart = noColor ? '' : pctColor(usedPct);
  const reset = c(RESET, noColor);

  const bar = `${colorStart}${'█'.repeat(filledChars)}${reset}${'░'.repeat(emptyChars)}`;
  return `${Math.round(usedPct)}% ${bar} ${formatTokens(totalUsed)}`;
}

// ── 9. renderFull ───────────────────────────────────────────────────

/**
 * Combines bar + table + relevant extras based on options.
 */
export function renderFull(composition, options = {}) {
  const {
    table = true,
    order = false,
    agents = false,
    timeline = false,
    toolCalls = false,
    compact = false,
    noColor = false,
  } = options;

  const styleOpts = { noColor };

  if (compact) {
    return renderCompact(composition, styleOpts);
  }

  const sections = [];

  // Always include the bar
  sections.push(renderBar(composition, undefined, styleOpts));

  if (table) {
    sections.push('');
    sections.push(renderTable(composition, styleOpts));
  }

  if (order) {
    sections.push('');
    sections.push(renderOrder(composition, styleOpts));
  }

  if (agents) {
    sections.push('');
    sections.push(renderAgents(composition, styleOpts));
  }

  if (timeline) {
    sections.push('');
    sections.push(renderTimeline(composition, styleOpts));
  }

  if (toolCalls) {
    sections.push('');
    sections.push(renderToolCalls(composition, styleOpts));
  }

  return sections.join('\n');
}

// ── 10. renderSetup ─────────────────────────────────────────────────

/**
 * Render setup instructions for the given tool.
 */
export function renderSetup(tool, { noColor = false } = {}) {
  const bold = c(BOLD, noColor);
  const dim = c(DIM, noColor);
  const reset = c(RESET, noColor);

  const lines = [];

  if (tool === 'codex') {
    lines.push(`${bold}ctx-monitor Setup — Codex CLI${reset}`);
    lines.push('━'.repeat(35));
    lines.push('');
    lines.push(`${bold}1. Install${reset}`);
    lines.push('   npm install -g ctx-monitor');
    lines.push('');
    lines.push(`${bold}2. Add to your shell config${reset} ${dim}(~/.zshrc or ~/.bashrc)${reset}`);
    lines.push('   export CTX_MONITOR_TOOL=codex');
    lines.push('');
    lines.push(`${bold}3. Run alongside Codex${reset}`);
    lines.push('   ctx-monitor watch');
    lines.push('');
    lines.push(`${bold}4. One-shot snapshot${reset}`);
    lines.push('   ctx-monitor snapshot');
    lines.push('');
    lines.push(`${dim}Codex CLI uses a different context structure than Claude Code.${reset}`);
    lines.push(`${dim}ctx-monitor auto-detects the tool from active sessions.${reset}`);
  } else {
    lines.push(`${bold}ctx-monitor Setup — Claude Code${reset}`);
    lines.push('━'.repeat(37));
    lines.push('');
    lines.push(`${bold}1. Install${reset}`);
    lines.push('   npm install -g ctx-monitor');
    lines.push('');
    lines.push(`${bold}2. Run alongside Claude Code${reset}`);
    lines.push('   ctx-monitor watch');
    lines.push('');
    lines.push(`${bold}3. One-shot snapshot${reset}`);
    lines.push('   ctx-monitor snapshot');
    lines.push('');
    lines.push(`${bold}4. Compact statusline output${reset}`);
    lines.push('   ctx-monitor status');
    lines.push('');
    lines.push(`${bold}5. HTML dashboard${reset}`);
    lines.push('   ctx-monitor dashboard');
    lines.push('');
    lines.push(`${dim}Claude Code uses a 200k token context window.${reset}`);
    lines.push(`${dim}ctx-monitor estimates usage from conversation structure.${reset}`);
  }

  return lines.join('\n');
}
