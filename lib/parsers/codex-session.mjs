// Codex CLI JSONL session/rollout parser
// Zero npm dependencies — Node.js built-ins only

import { createReadStream } from 'node:fs';
import { readdir, stat } from 'node:fs/promises';
import { createInterface } from 'node:readline';
import { join, basename } from 'node:path';
import { homedir } from 'node:os';

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function estimateTokens(text) {
  if (!text) return 0;
  if (typeof text !== 'string') text = JSON.stringify(text);
  return Math.ceil(text.length / 4);
}

function truncate(str, max = 500) {
  if (!str) return str;
  if (typeof str !== 'string') str = JSON.stringify(str);
  return str.length > max ? str.slice(0, max) + '…' : str;
}

/** Parse `*** filepath` and `@@@ -s,c +s,c @@@` from apply_patch content. */
function parseApplyPatch(content) {
  if (!content || typeof content !== 'string') return { files: [], hunks: [] };
  const files = [];
  const hunks = [];
  for (const line of content.split('\n')) {
    const fileMatch = line.match(/^\*\*\*\s+(.+)$/);
    if (fileMatch) {
      files.push(fileMatch[1].trim());
      continue;
    }
    const hunkMatch = line.match(/^@@@\s+-(\d+),(\d+)\s+\+(\d+),(\d+)\s+@@@/);
    if (hunkMatch) {
      hunks.push({
        oldStart: parseInt(hunkMatch[1], 10),
        oldCount: parseInt(hunkMatch[2], 10),
        newStart: parseInt(hunkMatch[3], 10),
        newCount: parseInt(hunkMatch[4], 10),
      });
    }
  }
  return { files, hunks };
}

// ---------------------------------------------------------------------------
// CODEX_HOME discovery
// ---------------------------------------------------------------------------

export function findCodexHome() {
  if (process.env.CODEX_HOME) return process.env.CODEX_HOME;
  const home = homedir();
  // Primary location
  const primary = join(home, '.codex');
  // Fallback
  const fallback = join(home, '.codex_home');
  // We return primary by default; callers that need the sessions dir should
  // check both if primary has no sessions/.
  return primary;
}

async function resolveSessionsDir() {
  const primary = join(findCodexHome(), 'sessions');
  try {
    const s = await stat(primary);
    if (s.isDirectory()) return primary;
  } catch { /* ignore */ }

  const fallback = join(homedir(), '.codex_home', 'sessions');
  try {
    const s = await stat(fallback);
    if (s.isDirectory()) return fallback;
  } catch { /* ignore */ }

  return primary; // return primary even if missing so errors are clear
}

// ---------------------------------------------------------------------------
// Session discovery
// ---------------------------------------------------------------------------

/** Recursively collect all *.jsonl files under a directory. */
async function collectJsonl(dir) {
  const results = [];
  let entries;
  try {
    entries = await readdir(dir, { withFileTypes: true });
  } catch {
    return results;
  }
  for (const entry of entries) {
    const full = join(dir, entry.name);
    if (entry.isDirectory()) {
      results.push(...await collectJsonl(full));
    } else if (entry.isFile() && entry.name.endsWith('.jsonl')) {
      try {
        const s = await stat(full);
        results.push({ path: full, name: entry.name, mtime: s.mtime, size: s.size });
      } catch { /* skip */ }
    }
  }
  return results;
}

/**
 * Find the most recent rollout JSONL file.
 * @returns {{ path: string, name: string, mtime: Date, size: number } | null}
 */
export async function findLatestSession() {
  const sessionsDir = await resolveSessionsDir();
  const all = await collectJsonl(sessionsDir);
  if (all.length === 0) return null;
  all.sort((a, b) => b.mtime.getTime() - a.mtime.getTime());
  return all[0];
}

/**
 * List all sessions with basic metadata.
 * @returns {Array<{ path: string, name: string, mtime: Date, size: number }>}
 */
export async function findAllSessions() {
  const sessionsDir = await resolveSessionsDir();
  const all = await collectJsonl(sessionsDir);
  all.sort((a, b) => b.mtime.getTime() - a.mtime.getTime());
  return all;
}

// ---------------------------------------------------------------------------
// Session parsing
// ---------------------------------------------------------------------------

const CODEX_TOOLS = new Set([
  'shell', 'apply_patch', 'file_read', 'update_plan',
  'spawn_agent', 'wait_agent', 'send_input', 'web_search',
  'spawn_agents_on_csv',
]);

/**
 * Parse a Codex CLI JSONL session file.
 *
 * Uses streaming readline so arbitrarily large files stay memory-friendly.
 *
 * @param {string} filePath  Absolute path to a rollout-*.jsonl file
 * @returns {Promise<Object>} Parsed session data
 */
export async function parseSession(filePath) {
  const session = {
    file: filePath,
    sessionId: basename(filePath).replace(/^rollout-/, '').replace(/\.jsonl$/, ''),
    model: null,
    contextWindowSize: null,
    reasoningEffort: null,

    tokenUsage: { total: 0, input: 0, cached: 0, output: 0, reasoning: 0 },
    lastTokenUsage: { total: 0, input: 0, cached: 0, output: 0, reasoning: 0 },

    toolCalls: [],
    toolResults: [],
    compactionEvents: [],
    subagentSpawns: [],
    planUsage: [],
    turns: [],

    tokenBuckets: {
      user_msg: 0,
      tool_results: 0,
      responses: 0,
      subagent: 0,
      reasoning: 0,
      plan: 0,
    },

    _raw: { lineCount: 0, parseErrors: 0 },
  };

  // Track turns being built
  const activeTurns = new Map(); // turnId -> turn object

  const rl = createInterface({
    input: createReadStream(filePath, { encoding: 'utf8' }),
    crlfDelay: Infinity,
  });

  let lineNum = 0;
  for await (const line of rl) {
    lineNum++;
    session._raw.lineCount = lineNum;
    if (!line.trim()) continue;

    let obj;
    try {
      obj = JSON.parse(line);
    } catch {
      session._raw.parseErrors++;
      continue;
    }

    // ---------------------------------------------------------------
    // Top-level session metadata
    // ---------------------------------------------------------------
    if (obj.type === 'session_meta') {
      const meta = obj.payload || {};
      if (meta.id) session.sessionId = meta.id;
      if (meta.model) session.model = meta.model;
      if (meta.model_context_window != null) session.contextWindowSize = meta.model_context_window;
      continue;
    }

    // ---------------------------------------------------------------
    // Top-level turn_context (not wrapped in event_msg)
    // ---------------------------------------------------------------
    if (obj.turn_context) {
      const tc = obj.turn_context;
      if (tc.model) session.model = tc.model;
      if (tc.reasoning_effort) session.reasoningEffort = tc.reasoning_effort;
      if (tc.model_context_window != null) session.contextWindowSize = tc.model_context_window;
    }

    // ---------------------------------------------------------------
    // Top-level response items (current Codex rollout format)
    // ---------------------------------------------------------------
    if (obj.type === 'response_item') {
      const item = obj.payload || {};
      const ts = obj.timestamp || item.timestamp || null;

      if (item.type === 'message') {
        const blocks = Array.isArray(item.content) ? item.content : [];
        const text = blocks
          .filter((block) => block && typeof block.text === 'string')
          .map((block) => block.text)
          .join('\n');
        const tokenEst = estimateTokens(text);

        if (item.role === 'user') {
          session.tokenBuckets.user_msg += tokenEst;
          session.turns.push({
            index: session.turns.length,
            timestamp: ts,
            text: truncate(text, 240),
            attachments: 0,
          });
        } else if (item.role === 'assistant') {
          session.tokenBuckets.responses += tokenEst;
        }
      } else if (item.type === 'function_call') {
        const toolName = item.name || item.function?.name || 'unknown';
        const args = item.arguments || item.function?.arguments || null;
        const call = {
          name: toolName,
          arguments: args,
          id: item.call_id || item.id || `fc_${lineNum}`,
          timestamp: ts,
          tokenEstimate: estimateTokens(args),
        };

        if (toolName === 'apply_patch') {
          const patchContent = typeof args === 'string' ? args : '';
          call.patchInfo = parseApplyPatch(patchContent);
        }

        session.toolCalls.push(call);
        if (toolName === 'spawn_agent') {
          session.subagentSpawns.push({ ...call });
          session.tokenBuckets.subagent += call.tokenEstimate;
        }
        if (toolName === 'update_plan') {
          session.planUsage.push({ ...call });
          session.tokenBuckets.plan += call.tokenEstimate;
        }
      } else if (item.type === 'function_call_output') {
        const content = item.output || item.content || '';
        const tokenEst = estimateTokens(content);
        session.toolResults.push({
          toolCallId: item.call_id || item.id || null,
          content: truncate(content),
          tokenEstimate: tokenEst,
        });
        session.tokenBuckets.tool_results += tokenEst;
      } else if (item.type === 'reasoning') {
        session.tokenBuckets.reasoning = Math.max(
          session.tokenBuckets.reasoning,
          session.tokenUsage.reasoning || 0,
        );
      }
      continue;
    }

    // ---------------------------------------------------------------
    // event_msg wrapper
    // ---------------------------------------------------------------
    const payload = obj.type === 'event_msg' ? (obj.payload || obj) : obj;
    const ptype = payload.type;
    const ts = obj.timestamp || payload.timestamp || null;

    switch (ptype) {
      // --- Token counts -------------------------------------------
      case 'token_count': {
        const usage = payload.info?.total_token_usage || payload.total_token_usage || {};
        const lastUsage = payload.info?.last_token_usage || payload.last_token_usage || {};
        if (usage.total_tokens != null) session.tokenUsage.total = usage.total_tokens;
        if (payload.total_token_usage != null && typeof payload.total_token_usage === 'number') {
          session.tokenUsage.total = payload.total_token_usage;
        }
        if (usage.input_tokens != null) session.tokenUsage.input = usage.input_tokens;
        if (payload.input_tokens != null) session.tokenUsage.input = payload.input_tokens;
        if (usage.cached_input_tokens != null) session.tokenUsage.cached = usage.cached_input_tokens;
        if (payload.cached_input_tokens != null) session.tokenUsage.cached = payload.cached_input_tokens;
        if (usage.output_tokens != null) session.tokenUsage.output = usage.output_tokens;
        if (payload.output_tokens != null) session.tokenUsage.output = payload.output_tokens;
        const reasoningTokens = usage.reasoning_output_tokens ?? payload.reasoning_tokens;
        if (reasoningTokens != null) {
          session.tokenUsage.reasoning = reasoningTokens;
          session.tokenBuckets.reasoning = reasoningTokens;
        }
        if (lastUsage.total_tokens != null) session.lastTokenUsage.total = lastUsage.total_tokens;
        if (lastUsage.input_tokens != null) session.lastTokenUsage.input = lastUsage.input_tokens;
        if (lastUsage.cached_input_tokens != null) session.lastTokenUsage.cached = lastUsage.cached_input_tokens;
        if (lastUsage.output_tokens != null) session.lastTokenUsage.output = lastUsage.output_tokens;
        if (lastUsage.reasoning_output_tokens != null) session.lastTokenUsage.reasoning = lastUsage.reasoning_output_tokens;
        const modelWindow = payload.info?.model_context_window ?? payload.model_context_window;
        if (modelWindow != null) session.contextWindowSize = modelWindow;
        if (payload.last_token_usage != null) {
          // last_token_usage is the delta for the most recent request
        }
        break;
      }

      case 'task_started': {
        if (payload.model_context_window != null) session.contextWindowSize = payload.model_context_window;
        break;
      }

      // --- Turns --------------------------------------------------
      case 'turn_started': {
        const turnId = payload.turn_id || payload.id || `turn_${lineNum}`;
        const turn = {
          id: turnId,
          model: payload.model || session.model,
          startTime: ts,
          endTime: null,
          status: 'running',
        };
        activeTurns.set(turnId, turn);
        session.turns.push(turn);
        if (payload.model) session.model = payload.model;
        break;
      }

      case 'turn_completed': {
        const turnId = payload.turn_id || payload.id;
        const turn = turnId ? activeTurns.get(turnId) : session.turns[session.turns.length - 1];
        if (turn) {
          turn.endTime = ts;
          turn.status = payload.status || 'completed';
          if (turnId) activeTurns.delete(turnId);
        }
        break;
      }

      // --- Context compaction -------------------------------------
      case 'context_compacted': {
        session.compactionEvents.push({
          timestamp: ts,
          preSize: payload.pre_size ?? payload.preSize ?? null,
          postSize: payload.post_size ?? payload.postSize ?? null,
        });
        break;
      }

      // --- Tool calls ---------------------------------------------
      case 'tool_call': {
        const toolName = payload.tool_name || payload.name || 'unknown';
        const args = payload.arguments || payload.args || null;
        const callId = payload.tool_call_id || payload.id || `tc_${lineNum}`;

        const call = {
          name: toolName,
          arguments: args,
          id: callId,
          timestamp: ts,
          tokenEstimate: estimateTokens(args),
        };

        // apply_patch: extract file paths and hunks
        if (toolName === 'apply_patch') {
          const patchContent = typeof args === 'string' ? args
            : (args && typeof args === 'object' ? (args.patch || args.content || JSON.stringify(args)) : '');
          call.patchInfo = parseApplyPatch(patchContent);
        }

        session.toolCalls.push(call);

        // Bucket tracking
        if (toolName === 'spawn_agent') {
          session.subagentSpawns.push({ ...call });
          session.tokenBuckets.subagent += call.tokenEstimate;
        }
        if (toolName === 'update_plan') {
          session.planUsage.push({ ...call });
          session.tokenBuckets.plan += call.tokenEstimate;
        }
        break;
      }

      // --- Tool results -------------------------------------------
      case 'tool_result': {
        const content = payload.content || payload.output || '';
        const tokenEst = estimateTokens(content);
        session.toolResults.push({
          toolCallId: payload.tool_call_id || payload.id || null,
          content: truncate(content),
          tokenEstimate: tokenEst,
        });
        session.tokenBuckets.tool_results += tokenEst;
        break;
      }

      // --- Response items -----------------------------------------
      case 'response_item': {
        const content = payload.text || payload.content || '';
        if (content) {
          session.tokenBuckets.responses += estimateTokens(content);
        }
        // Function calls embedded in response items
        if (payload.function_call || payload.tool_calls) {
          const calls = payload.tool_calls || [payload.function_call];
          for (const fc of calls) {
            if (!fc) continue;
            const toolName = fc.name || fc.function?.name || 'unknown';
            const args = fc.arguments || fc.function?.arguments || null;
            const callEntry = {
              name: toolName,
              arguments: args,
              id: fc.id || fc.call_id || `fc_${lineNum}`,
              timestamp: ts,
              tokenEstimate: estimateTokens(args),
            };
            if (toolName === 'apply_patch') {
              const patchContent = typeof args === 'string' ? args : '';
              callEntry.patchInfo = parseApplyPatch(patchContent);
            }
            session.toolCalls.push(callEntry);
            if (toolName === 'spawn_agent') {
              session.subagentSpawns.push({ ...callEntry });
              session.tokenBuckets.subagent += callEntry.tokenEstimate;
            }
            if (toolName === 'update_plan') {
              session.planUsage.push({ ...callEntry });
              session.tokenBuckets.plan += callEntry.tokenEstimate;
            }
          }
        }
        break;
      }

      // --- Session metadata ---------------------------------------
      case 'session_meta': {
        const meta = payload.payload || payload;
        if (meta.id) session.sessionId = meta.id;
        if (meta.model_context_window != null) session.contextWindowSize = meta.model_context_window;
        if (meta.model) session.model = meta.model;
        break;
      }

      // --- User messages (for bucket tracking) --------------------
      case 'user_message':
      case 'input_text': {
        const text = payload.text || payload.content || '';
        session.tokenBuckets.user_msg += estimateTokens(text);
        break;
      }

      default:
        // Ignore unknown event types
        break;
    }
  }

  return session;
}
