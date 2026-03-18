#!/usr/bin/env node

/**
 * ctx-monitor — Context Composition Monitor for Claude Code & Codex CLI.
 *
 * Main CLI entry point. Zero npm dependencies; Node.js built-ins only.
 */

import { existsSync, statSync } from 'node:fs';
import { join } from 'node:path';
import { homedir } from 'node:os';

import { parseSession as parseClaudeSession, findLatestSession as findLatestClaudeSession, findProjectDir, getSessionDir } from '../lib/parsers/claude-session.mjs';
import { parseClaudeConfig } from '../lib/parsers/claude-config.mjs';
import { fetchPlanUsage } from '../lib/parsers/claude-usage-api.mjs';
import { parseSession as parseCodexSession, findLatestSession as findLatestCodexSession } from '../lib/parsers/codex-session.mjs';
import { parseCodexConfig } from '../lib/parsers/codex-config.mjs';
import { estimateClaudeContext, estimateCodexContext, simulateUsage } from '../lib/estimator.mjs';
import { startServer } from '../lib/server.mjs';

// ---------------------------------------------------------------------------
// Lazy-loaded renderer (may not exist yet)
// ---------------------------------------------------------------------------

let _renderer = null;

async function getRenderer() {
  if (_renderer) return _renderer;
  try {
    _renderer = await import('../lib/renderer/terminal.mjs');
  } catch {
    _renderer = {};
  }
  return _renderer;
}

// ---------------------------------------------------------------------------
// Argument parsing
// ---------------------------------------------------------------------------

function parseArgs(argv) {
  const args = {
    mode: null,        // 'claude' | 'codex' | null (auto-detect)
    watch: null,       // null = no watch, number = seconds
    pct: null,         // simulate N% usage
    session: null,     // target session UUID
    project: null,     // target project path
    serve: null,       // null = no serve, number = port

    // Views
    table: false,
    order: false,
    agents: false,
    setup: false,
    timeline: false,
    diff: false,

    // Statusline
    statusline: false,
    statuslineFull: false,

    // Output
    json: false,
    noColor: false,
    compact: false,
    help: false,
  };

  let i = 0;
  while (i < argv.length) {
    const arg = argv[i];

    switch (arg) {
      case '--claude':
      case '-c':
        args.mode = 'claude';
        break;

      case '--codex':
      case '-x':
        args.mode = 'codex';
        break;

      case '--watch':
      case '-w': {
        const next = argv[i + 1];
        if (next && !next.startsWith('-') && /^\d+$/.test(next)) {
          args.watch = parseInt(next, 10);
          i++;
        } else {
          args.watch = 5;
        }
        break;
      }

      case '--pct':
      case '-p': {
        const next = argv[i + 1];
        if (next != null) {
          const n = parseFloat(next);
          if (Number.isFinite(n) && n >= 0 && n <= 100) {
            args.pct = n;
            i++;
          } else {
            process.stderr.write(`Error: --pct requires a number between 0 and 100\n`);
            process.exit(2);
          }
        } else {
          process.stderr.write(`Error: --pct requires a value\n`);
          process.exit(2);
        }
        break;
      }

      case '--session': {
        const next = argv[i + 1];
        if (next && !next.startsWith('-')) {
          args.session = next;
          i++;
        } else {
          process.stderr.write(`Error: --session requires a session ID\n`);
          process.exit(2);
        }
        break;
      }

      case '--project': {
        const next = argv[i + 1];
        if (next && !next.startsWith('-')) {
          args.project = next;
          i++;
        } else {
          process.stderr.write(`Error: --project requires a path\n`);
          process.exit(2);
        }
        break;
      }

      case '--serve': {
        const next = argv[i + 1];
        if (next && !next.startsWith('-') && /^\d+$/.test(next)) {
          args.serve = parseInt(next, 10);
          i++;
        } else {
          args.serve = 3456;
        }
        break;
      }

      case '--table':
      case '-t':
        args.table = true;
        break;

      case '--order':
      case '-o':
        args.order = true;
        break;

      case '--agents':
      case '-a':
        args.agents = true;
        break;

      case '--setup':
      case '-s':
        args.setup = true;
        break;

      case '--timeline':
        args.timeline = true;
        break;

      case '--diff':
        args.diff = true;
        break;

      case '--statusline':
        args.statusline = true;
        break;

      case '--statusline-full':
        args.statuslineFull = true;
        break;

      case '--json':
        args.json = true;
        break;

      case '--no-color':
        args.noColor = true;
        break;

      case '--compact':
        args.compact = true;
        break;

      case '--help':
      case '-h':
        args.help = true;
        break;

      default:
        if (arg.startsWith('-')) {
          process.stderr.write(`Unknown option: ${arg}\n`);
          process.exit(2);
        }
        break;
    }
    i++;
  }

  return args;
}

// ---------------------------------------------------------------------------
// Help text
// ---------------------------------------------------------------------------

const HELP_TEXT = `
ctx-monitor — Context Composition Monitor for Claude Code & Codex CLI

Usage: ctx-monitor [options]

Options:
  --claude, -c          Force Claude Code mode
  --codex, -x           Force Codex CLI mode
  --watch [N], -w [N]   Re-render every N seconds (default: 5)
  --pct <N>, -p <N>     Simulate N% context usage
  --session <id>        Target specific session by UUID
  --project <path>      Target specific project directory
  --serve [port]        Start HTTP server with browser dashboard (default: 3456)

Views:
  --table, -t           Component breakdown table only
  --order, -o           Context loading order diagram
  --agents, -a          Subagent/team isolation diagram
  --setup, -s           Setup instructions
  --timeline            Show context growth timeline
  --diff                Compare two sessions side-by-side

Statusline integration:
  --statusline          Read Claude Code JSON from stdin, output compact bar
  --statusline-full     Read stdin, output multi-line component breakdown

Output:
  --json                Output structured JSON
  --no-color            Disable ANSI colors
  --compact             Single-line output mode
  --help, -h            Show help

Exit codes:
  0  Success
  1  No session found
  2  Invalid arguments
  3  Parse error
`.trimStart();

// ---------------------------------------------------------------------------
// Mode detection
// ---------------------------------------------------------------------------

function detectMode() {
  const home = homedir();
  const claudeDir = join(home, '.claude');
  const codexDir = join(home, '.codex');

  if (existsSync(claudeDir)) return 'claude';
  if (existsSync(codexDir)) return 'codex';
  return 'claude'; // default
}

// ---------------------------------------------------------------------------
// Stdin reader (for --statusline)
// ---------------------------------------------------------------------------

function readStdin() {
  return new Promise((resolve, reject) => {
    const chunks = [];
    process.stdin.setEncoding('utf8');
    process.stdin.on('data', (chunk) => chunks.push(chunk));
    process.stdin.on('end', () => resolve(chunks.join('')));
    process.stdin.on('error', reject);

    // If stdin is a TTY (no pipe), resolve immediately with empty string
    if (process.stdin.isTTY) {
      resolve('');
    }
  });
}

// ---------------------------------------------------------------------------
// Session discovery helpers
// ---------------------------------------------------------------------------

function resolveProjectDir(args) {
  if (args.project) {
    const projDir = findProjectDir(args.project);
    if (projDir) return { projectPath: args.project, sessionDir: projDir };
    // If findProjectDir fails, the project path itself might be the session dir
    return { projectPath: args.project, sessionDir: null };
  }
  // Use current working directory
  const cwd = process.cwd();
  const projDir = findProjectDir(cwd);
  return { projectPath: cwd, sessionDir: projDir };
}

async function findAndParseClaudeSession(args) {
  const { projectPath, sessionDir } = resolveProjectDir(args);

  if (!sessionDir) {
    return { session: null, projectPath };
  }

  let sessionFile;
  if (args.session) {
    // Look for specific session by ID
    const filePath = join(sessionDir, `${args.session}.jsonl`);
    if (existsSync(filePath)) {
      sessionFile = { id: args.session, filePath };
    } else {
      process.stderr.write(`Session not found: ${args.session}\n`);
      return { session: null, projectPath };
    }
  } else {
    sessionFile = findLatestClaudeSession(sessionDir);
  }

  if (!sessionFile) {
    return { session: null, projectPath };
  }

  try {
    const session = await parseClaudeSession(sessionFile.filePath);
    return { session, projectPath };
  } catch (err) {
    process.stderr.write(`Parse error: ${err.message}\n`);
    process.exit(3);
  }
}

async function findAndParseCodexSession(args) {
  let sessionFile;
  if (args.session) {
    // For codex, session ID might be a path or a name
    if (existsSync(args.session)) {
      sessionFile = { path: args.session };
    } else {
      process.stderr.write(`Session not found: ${args.session}\n`);
      return { session: null, projectPath: args.project || process.cwd() };
    }
  } else {
    sessionFile = await findLatestCodexSession();
  }

  if (!sessionFile) {
    return { session: null, projectPath: args.project || process.cwd() };
  }

  try {
    const session = await parseCodexSession(sessionFile.path);
    return { session, projectPath: args.project || process.cwd() };
  } catch (err) {
    process.stderr.write(`Parse error: ${err.message}\n`);
    process.exit(3);
  }
}

// ---------------------------------------------------------------------------
// Composition builder
// ---------------------------------------------------------------------------

async function buildComposition(mode, args) {
  if (mode === 'claude') {
    const { session, projectPath } = await findAndParseClaudeSession(args);
    if (!session) {
      process.stderr.write('No Claude Code session found.\n');
      process.exit(1);
    }
    const config = parseClaudeConfig(projectPath);

    // Try to fetch plan usage (non-blocking)
    let planUsage = null;
    try {
      planUsage = await fetchPlanUsage();
    } catch {
      // Plan usage is optional — ignore failures
    }

    let composition = estimateClaudeContext({ session, config });
    if (planUsage) {
      composition.planUsage = planUsage;
    }

    if (args.pct != null) {
      composition = simulateUsage(composition, args.pct);
    }

    return composition;
  }

  if (mode === 'codex') {
    const { session, projectPath } = await findAndParseCodexSession(args);
    if (!session) {
      process.stderr.write('No Codex CLI session found.\n');
      process.exit(1);
    }
    const config = await parseCodexConfig(projectPath);

    let composition = estimateCodexContext({ session, config });

    if (args.pct != null) {
      composition = simulateUsage(composition, args.pct);
    }

    return composition;
  }

  process.stderr.write(`Unknown mode: ${mode}\n`);
  process.exit(2);
}

// ---------------------------------------------------------------------------
// Watch mode — track file mtime and only re-parse when changed
// ---------------------------------------------------------------------------

function startWatchMode(mode, args, renderFn) {
  let lastMtime = 0;
  const intervalSec = args.watch || 5;

  async function tick() {
    try {
      // Check if the session file has changed
      let sessionFilePath = null;
      if (mode === 'claude') {
        const { sessionDir } = resolveProjectDir(args);
        if (sessionDir) {
          if (args.session) {
            sessionFilePath = join(sessionDir, `${args.session}.jsonl`);
          } else {
            const latest = findLatestClaudeSession(sessionDir);
            if (latest) sessionFilePath = latest.filePath;
          }
        }
      } else if (mode === 'codex') {
        if (args.session && existsSync(args.session)) {
          sessionFilePath = args.session;
        } else {
          const latest = await findLatestCodexSession();
          if (latest) sessionFilePath = latest.path;
        }
      }

      if (!sessionFilePath || !existsSync(sessionFilePath)) return;

      const mtime = statSync(sessionFilePath).mtimeMs;
      if (mtime <= lastMtime) return; // File hasn't changed
      lastMtime = mtime;

      const composition = await buildComposition(mode, args);
      await renderFn(composition);
    } catch (err) {
      process.stderr.write(`Watch error: ${err.message}\n`);
    }
  }

  // Initial render
  tick();

  // Set interval
  const handle = setInterval(tick, intervalSec * 1000);

  // Clean exit
  process.on('SIGINT', () => {
    clearInterval(handle);
    process.exit(0);
  });
  process.on('SIGTERM', () => {
    clearInterval(handle);
    process.exit(0);
  });
}

// ---------------------------------------------------------------------------
// Render dispatch
// ---------------------------------------------------------------------------

async function render(composition, args) {
  const R = await getRenderer();
  const opts = { noColor: args.noColor };

  if (args.table && R.renderTable) {
    process.stdout.write(R.renderTable(composition, opts) + '\n');
  } else if (args.order && R.renderOrder) {
    process.stdout.write(R.renderOrder(composition, opts) + '\n');
  } else if (args.agents && R.renderAgents) {
    process.stdout.write(R.renderAgents(composition, opts) + '\n');
  } else if (args.timeline && R.renderTimeline) {
    process.stdout.write(R.renderTimeline(composition, opts) + '\n');
  } else if (args.compact && R.renderCompact) {
    process.stdout.write(R.renderCompact(composition, opts) + '\n');
  } else if (R.renderFull) {
    process.stdout.write(R.renderFull(composition, opts) + '\n');
  } else {
    // Fallback: JSON output if no renderer available
    process.stdout.write(JSON.stringify(composition, null, 2) + '\n');
  }
}

// ---------------------------------------------------------------------------
// Server mode helpers
// ---------------------------------------------------------------------------

function startServeMode(mode, args, composition) {
  startServer({
    port: args.serve,

    getComposition: async () => {
      try {
        return await buildComposition(mode, args);
      } catch {
        return composition; // Return last known composition on error
      }
    },

    getSessions: async () => {
      try {
        if (mode === 'claude') {
          const { findAllSessions } = await import('../lib/parsers/claude-session.mjs');
          const { sessionDir } = resolveProjectDir(args);
          if (!sessionDir) return [];
          return findAllSessions(sessionDir).map(s => ({
            id: s.id,
            mtime: s.mtime.toISOString(),
            size: s.size,
          }));
        } else {
          const { findAllSessions } = await import('../lib/parsers/codex-session.mjs');
          const sessions = await findAllSessions();
          return sessions.map(s => ({
            id: s.name
              .replace(/^rollout-\d{4}-\d{2}-\d{2}T\d{2}-\d{2}-\d{2}-/, '')
              .replace(/\.jsonl$/, ''),
            path: s.path,
            mtime: s.mtime.toISOString(),
            size: s.size,
          }));
        }
      } catch {
        return [];
      }
    },

    getSessionById: async (id) => {
      try {
        if (mode === 'claude') {
          const { sessionDir, projectPath } = resolveProjectDir(args);
          if (!sessionDir) return null;
          const filePath = join(sessionDir, `${id}.jsonl`);
          if (!existsSync(filePath)) return null;
          const session = await parseClaudeSession(filePath);
          const config = parseClaudeConfig(projectPath || args.project || process.cwd());
          return estimateClaudeContext({ session, config });
        } else {
          const { findAllSessions } = await import('../lib/parsers/codex-session.mjs');
          const sessions = await findAllSessions();
          const match = sessions.find(s => s.name.includes(id));
          if (!match) return null;
          const session = await parseCodexSession(match.path);
          const config = await parseCodexConfig(args.project || process.cwd());
          return estimateCodexContext({ session, config });
        }
      } catch {
        return null;
      }
    },

    getTimeline: async (id) => {
      try {
        // Build a timeline by parsing the session and extracting per-message token growth
        let session;
        if (mode === 'claude') {
          const { sessionDir } = resolveProjectDir(args);
          if (!sessionDir) return null;
          const filePath = join(sessionDir, `${id}.jsonl`);
          if (!existsSync(filePath)) return null;
          session = await parseClaudeSession(filePath);
        } else {
          const { findAllSessions } = await import('../lib/parsers/codex-session.mjs');
          const sessions = await findAllSessions();
          const match = sessions.find(s => s.name.includes(id));
          if (!match) return null;
          session = await parseCodexSession(match.path);
        }

        if (!session) return null;

        // Build timeline from messages
        let runningTotal = 0;
        const events = (session.messages || session.turns || []).map((msg, i) => {
          const tokens = msg.tokenEstimate || 0;
          runningTotal += tokens;
          return {
            index: i,
            role: msg.role || msg.status || 'unknown',
            timestamp: msg.timestamp || msg.startTime || null,
            tokens,
            cumulativeTokens: runningTotal,
            toolCalls: msg.toolCallCount || 0,
          };
        });

        return {
          sessionId: id,
          eventCount: events.length,
          totalTokens: runningTotal,
          events,
        };
      } catch {
        return null;
      }
    },
  });
}

// ---------------------------------------------------------------------------
// Main
// ---------------------------------------------------------------------------

async function main() {
  const argv = process.argv.slice(2);
  const args = parseArgs(argv);

  // --help
  if (args.help) {
    process.stdout.write(HELP_TEXT);
    process.exit(0);
  }

  // Detect mode
  const mode = args.mode || detectMode();

  // --statusline: read stdin, compute, render compact bar, exit
  if (args.statusline || args.statuslineFull) {
    const raw = await readStdin();
    if (!raw.trim()) {
      process.stdout.write('');
      process.exit(0);
    }

    let statusline;
    try {
      statusline = JSON.parse(raw);
    } catch {
      process.stderr.write('Error: Invalid JSON on stdin\n');
      process.exit(3);
    }

    const composition = estimateClaudeContext({ statusline });

    if (args.pct != null) {
      const adjusted = simulateUsage(composition, args.pct);
      const R = await getRenderer();
      if (args.statuslineFull && R.renderTable) {
        process.stdout.write(R.renderTable(adjusted, { noColor: args.noColor }) + '\n');
      } else if (R.renderStatusline) {
        process.stdout.write(R.renderStatusline(adjusted, { noColor: args.noColor }));
      } else if (R.renderBar) {
        process.stdout.write(R.renderBar(adjusted, { noColor: args.noColor }));
      } else {
        process.stdout.write(JSON.stringify(adjusted) + '\n');
      }
    } else {
      const R = await getRenderer();
      if (args.statuslineFull && R.renderTable) {
        process.stdout.write(R.renderTable(composition, { noColor: args.noColor }) + '\n');
      } else if (R.renderStatusline) {
        process.stdout.write(R.renderStatusline(composition, { noColor: args.noColor }));
      } else if (R.renderBar) {
        process.stdout.write(R.renderBar(composition, { noColor: args.noColor }));
      } else {
        process.stdout.write(JSON.stringify(composition) + '\n');
      }
    }
    process.exit(0);
  }

  // --setup: show setup instructions, exit
  if (args.setup) {
    const R = await getRenderer();
    if (R.renderSetup) {
      process.stdout.write(R.renderSetup({ mode, noColor: args.noColor }) + '\n');
    } else {
      process.stdout.write(`Setup instructions for ${mode} mode are not yet available.\n`);
      process.stdout.write(`Ensure ~/.${mode === 'claude' ? 'claude' : 'codex'} directory exists.\n`);
    }
    process.exit(0);
  }

  // Build composition
  const composition = await buildComposition(mode, args);

  // --json: output JSON and exit
  if (args.json) {
    process.stdout.write(JSON.stringify(composition, null, 2) + '\n');
    process.exit(0);
  }

  // --serve: start HTTP server
  if (args.serve != null) {
    startServeMode(mode, args, composition);

    // If also watching, set up watch alongside server
    if (args.watch != null) {
      // In serve mode, watch is handled by the server's getComposition callback
      process.stderr.write(`Watching for changes every ${args.watch || 5}s (via API refresh)\n`);
    }
    return; // Keep process alive for server
  }

  // --watch: re-render on interval
  if (args.watch != null) {
    startWatchMode(mode, args, async (comp) => {
      // Clear screen for clean re-render
      process.stdout.write('\x1b[2J\x1b[H');
      await render(comp, args);
    });
    return; // Keep process alive for watch
  }

  // Single render and exit
  await render(composition, args);
  process.exit(0);
}

// Run
main().catch((err) => {
  process.stderr.write(`Fatal error: ${err.message}\n`);
  process.exit(3);
});
