#!/usr/bin/env node

// Claude Code statusline stdin→stdout bridge
// Reads JSON from stdin, outputs a compact colored status bar to stdout.
// Zero dependencies. Designed to run fast on every assistant message.

const chunks = [];
process.stdin.on('data', (chunk) => chunks.push(chunk));
process.stdin.on('end', () => {
  const raw = Buffer.concat(chunks).toString().trim();

  if (!raw) {
    process.stdout.write('ctx-monitor: no data\n');
    return;
  }

  let data;
  try {
    data = JSON.parse(raw);
  } catch {
    process.stdout.write('ctx-monitor: parse error\n');
    return;
  }

  const cw = data.context_window;
  if (!cw || cw.used_percentage == null) {
    process.stdout.write('ctx-monitor: waiting...\n');
    return;
  }

  const pct = cw.used_percentage;
  const totalSize = cw.context_window_size || 200000;
  const usage = data.current_usage;
  const cost = data.cost;

  // Determine bar color based on percentage thresholds
  let colorCode;
  if (pct >= 75) {
    colorCode = 31; // red
  } else if (pct >= 50) {
    colorCode = 33; // yellow
  } else {
    colorCode = 32; // green
  }

  const esc = (code, text) => `\x1b[${code}m${text}\x1b[0m`;

  // Build the bar
  const barWidth = 40;
  const filledCount = Math.round((pct / 100) * barWidth);
  const emptyCount = barWidth - filledCount;

  let bar;

  // If we have current_usage breakdown, show component segments
  if (usage) {
    const inputTokens = usage.input_tokens || 0;
    const cacheCreation = usage.cache_creation_input_tokens || 0;
    const cacheRead = usage.cache_read_input_tokens || 0;
    const totalInput = inputTokens + cacheCreation + cacheRead;

    if (totalInput > 0) {
      // Component colors (ANSI 256)
      const components = [
        { label: 'inp', tokens: inputTokens, color: 75 },      // blue
        { label: 'c_wr', tokens: cacheCreation, color: 214 },   // orange
        { label: 'c_rd', tokens: cacheRead, color: 114 },       // green
      ].filter(c => c.tokens > 0);

      let segments = '';
      let usedCells = 0;

      for (const comp of components) {
        const ratio = comp.tokens / totalInput;
        const cells = Math.max(ratio >= 0.01 ? 1 : 0, Math.round(ratio * filledCount));
        if (cells > 0) {
          segments += `\x1b[38;5;${comp.color}m${'█'.repeat(cells)}\x1b[0m`;
          usedCells += cells;
        }
      }

      // Adjust for rounding: fill or trim to match filledCount
      if (usedCells < filledCount) {
        const last = components[components.length - 1];
        segments += `\x1b[38;5;${last.color}m${'█'.repeat(filledCount - usedCells)}\x1b[0m`;
      }

      bar = segments + '░'.repeat(emptyCount);
    } else {
      bar = esc(colorCode, '█'.repeat(filledCount)) + '░'.repeat(emptyCount);
    }
  } else {
    bar = esc(colorCode, '█'.repeat(filledCount)) + '░'.repeat(emptyCount);
  }

  // Format token counts (e.g. 84000 → "84k")
  const fmtTokens = (n) => {
    if (n >= 1000000) return (n / 1000000).toFixed(1).replace(/\.0$/, '') + 'M';
    if (n >= 1000) return (n / 1000).toFixed(0) + 'k';
    return String(n);
  };

  // Total input tokens used
  let usedTokens;
  if (usage) {
    usedTokens = (usage.input_tokens || 0) +
                 (usage.cache_creation_input_tokens || 0) +
                 (usage.cache_read_input_tokens || 0);
  } else {
    usedTokens = cw.total_input_tokens || 0;
  }

  const pctStr = `${Math.round(pct)}%`;
  const tokenStr = `${fmtTokens(usedTokens)}/${fmtTokens(totalSize)}`;

  // Cost suffix
  let costStr = '';
  if (cost && cost.total_cost_usd != null) {
    costStr = ` $${cost.total_cost_usd.toFixed(2)}`;
  }

  process.stdout.write(`${pctStr} ${bar} ${tokenStr}${costStr}\n`);
});
