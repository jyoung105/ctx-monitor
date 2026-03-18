// Claude Code JSONL session parser
// Streaming line-by-line parsing for performance (files can be 50MB+)
// Zero npm dependencies — Node.js built-ins only

import { createReadStream, readdirSync, statSync, existsSync } from 'fs';
import { createInterface } from 'readline';
import { join, basename } from 'path';
import { homedir } from 'os';

// ---------------------------------------------------------------------------
// Token estimation
// ---------------------------------------------------------------------------

function estimateTokens(text) {
  if (!text) return 0;
  if (typeof text !== 'string') text = JSON.stringify(text);
  return Math.ceil(text.length / 4);
}

// ---------------------------------------------------------------------------
// Path helpers
// ---------------------------------------------------------------------------

/**
 * Returns the base ~/.claude/projects/ directory.
 */
export function getSessionDir() {
  return join(homedir(), '.claude', 'projects');
}

/**
 * Finds the Claude project directory for a given working directory.
 * Claude encodes the absolute path as a URL-encoded segment under ~/.claude/projects/.
 */
export function findProjectDir(cwd) {
  const base = getSessionDir();
  if (!existsSync(base)) return null;

  // Claude Code encodes project paths by replacing '/' with '-'
  // e.g. /Users/tonylee/ctx-monitor → -Users-tonylee-ctx-monitor
  const dashEncoded = cwd.replace(/\//g, '-');
  const dir = join(base, dashEncoded);
  if (existsSync(dir)) return dir;

  // Also try URL-encoded form
  const urlEncoded = encodeURIComponent(cwd);
  const dir2 = join(base, urlEncoded);
  if (existsSync(dir2)) return dir2;

  // Fallback: scan for a directory whose name matches the cwd
  try {
    const entries = readdirSync(base, { withFileTypes: true });
    for (const entry of entries) {
      if (!entry.isDirectory()) continue;
      // Reverse dash-encoding: replace leading dash and internal dashes with /
      const restored = entry.name.replace(/^-/, '/').replace(/-/g, '/');
      if (restored === cwd) return join(base, entry.name);
      // Also try URL decode
      try {
        const decoded = decodeURIComponent(entry.name);
        if (decoded === cwd) return join(base, entry.name);
      } catch { /* skip malformed names */ }
    }
  } catch { /* base dir unreadable */ }

  return null;
}

// ---------------------------------------------------------------------------
// Session discovery
// ---------------------------------------------------------------------------

/**
 * Lists all sessions in a project directory with metadata.
 * @param {string} projectPath — the project directory (from findProjectDir)
 * @returns {{ id: string, filePath: string, mtime: Date, size: number }[]}
 */
export function findAllSessions(projectPath) {
  if (!projectPath || !existsSync(projectPath)) return [];

  const results = [];
  try {
    const entries = readdirSync(projectPath);
    for (const name of entries) {
      if (!name.endsWith('.jsonl')) continue;
      const filePath = join(projectPath, name);
      try {
        const st = statSync(filePath);
        results.push({
          id: basename(name, '.jsonl'),
          filePath,
          mtime: st.mtime,
          size: st.size,
        });
      } catch { /* skip unreadable files */ }
    }
  } catch { /* dir unreadable */ }

  results.sort((a, b) => b.mtime.getTime() - a.mtime.getTime());
  return results;
}

/**
 * Finds the most recently modified .jsonl in the project directory.
 * @param {string} projectPath
 * @returns {{ id: string, filePath: string, mtime: Date, size: number } | null}
 */
export function findLatestSession(projectPath) {
  const all = findAllSessions(projectPath);
  return all.length > 0 ? all[0] : null;
}

// ---------------------------------------------------------------------------
// Content extraction helpers
// ---------------------------------------------------------------------------

function extractTextFromContent(content) {
  if (!content) return '';
  if (typeof content === 'string') return content;
  if (Array.isArray(content)) {
    return content
      .filter(b => b.type === 'text')
      .map(b => b.text || '')
      .join('\n');
  }
  return '';
}

/**
 * Extract image and file attachment blocks from content array.
 * Claude vision: images cost ~1,600 tokens for standard, ~800 for low-res.
 * Files/documents: estimate from base64 size or metadata.
 */
function extractAttachments(content) {
  if (!Array.isArray(content)) return [];
  const attachments = [];
  for (const b of content) {
    if (b.type === 'image') {
      // Image block: { type: "image", source: { type: "base64", media_type, data } }
      const src = b.source || {};
      const dataLen = (src.data || '').length;
      // base64 data → rough byte size → token estimate
      // Standard image ≈ 1,600 tokens, but scale with size
      const bytesEst = dataLen ? Math.ceil(dataLen * 0.75) : 0;
      const tokenEst = bytesEst > 0 ? Math.max(85, Math.ceil(bytesEst / 750)) : 1600;
      attachments.push({
        type: 'image',
        mediaType: src.media_type || 'image/unknown',
        tokens: tokenEst,
        sizeBytes: bytesEst,
      });
    } else if (b.type === 'document' || b.type === 'file') {
      // Document/file block
      const src = b.source || {};
      const dataLen = (src.data || '').length;
      const bytesEst = dataLen ? Math.ceil(dataLen * 0.75) : 0;
      const tokenEst = bytesEst > 0 ? Math.ceil(bytesEst / 4) : 500;
      attachments.push({
        type: b.type,
        mediaType: src.media_type || b.media_type || 'unknown',
        name: b.name || b.title || null,
        tokens: tokenEst,
        sizeBytes: bytesEst,
      });
    }
  }
  return attachments;
}

function extractToolCalls(content, timestamp) {
  if (!Array.isArray(content)) return [];
  return content
    .filter(b => b.type === 'tool_use')
    .map(b => ({
      name: b.name,
      input: b.input || {},
      id: b.id,
      timestamp,
      tokenEstimate: estimateTokens(b.input),
    }));
}

function extractToolResults(content) {
  if (!Array.isArray(content)) return [];
  return content
    .filter(b => b.type === 'tool_result')
    .map(b => {
      const raw = typeof b.content === 'string'
        ? b.content
        : JSON.stringify(b.content || '');
      return {
        toolUseId: b.tool_use_id,
        content: raw.length > 500 ? raw.slice(0, 500) + '…' : raw,
        tokenEstimate: estimateTokens(raw),
      };
    });
}

function extractThinkingBlocks(content) {
  if (!Array.isArray(content)) return [];
  return content.filter(b => b.type === 'thinking');
}

function extractFilePath(toolCall) {
  const { name, input } = toolCall;
  switch (name) {
    case 'Read':
      return input.file_path || null;
    case 'Edit':
      return input.file_path || null;
    case 'Write':
      return input.file_path || null;
    case 'Grep':
      return input.path || null;
    case 'Glob':
      return input.path || null;
    case 'Bash':
      return null; // command string — no single file path
    default:
      return input.file_path || input.path || null;
  }
}

// Detect skill activations (Skill tool calls)
function isSkillActivation(toolCall) {
  return toolCall.name === 'Skill';
}

// Detect subagent spawns (Task, Explore, Plan, etc.)
const SUBAGENT_TOOLS = new Set([
  'Task', 'Agent', 'Explore', 'Plan', 'TaskCreate', 'SendMessage',
]);

function isSubagentSpawn(toolCall) {
  return SUBAGENT_TOOLS.has(toolCall.name);
}

// Detect plan-related tool calls
const PLAN_TOOLS = new Set([
  'TodoRead', 'TodoWrite', 'plan', 'todo_read', 'todo_write',
]);

function isPlanUsage(toolCall) {
  return PLAN_TOOLS.has(toolCall.name);
}

// Classify a tool call into a token bucket category
function classifyBucket(toolCall) {
  if (isSkillActivation(toolCall)) return 'skill_body';
  if (isSubagentSpawn(toolCall)) return 'subagent';
  if (isPlanUsage(toolCall)) return 'plan';
  return null; // generic tool call — accounted for in tool_results bucket
}

// ---------------------------------------------------------------------------
// Main parser
// ---------------------------------------------------------------------------

/**
 * Parses a Claude Code session JSONL file using streaming readline.
 * @param {string} filePath — path to the .jsonl file
 * @returns {Promise<Object>} parsed session object
 */
export async function parseSession(filePath) {
  const filenameId = basename(filePath, '.jsonl');

  const session = {
    sessionId: null,
    model: null,
    version: null,
    cwd: null,
    timestamps: { first: null, last: null },
    isContinuation: false,
    messages: [],
    toolCalls: [],
    toolResults: [],
    thinkingBlocks: { count: 0, totalEstimatedTokens: 0 },
    compactionEvents: { count: 0, timestamps: [] },
    turns: [],            // user turn boundaries: [{ index, timestamp, text, attachments }]
    attachments: [],       // image/file attachments with token costs
    skillActivations: [],
    subagentSpawns: [],
    planUsage: [],
    usage: null,
    tokenBuckets: {
      user_msg: 0,
      tool_results: 0,
      responses: 0,
      subagent: 0,
      skill_body: 0,
      plan: 0,
      thinking: 0,
    },
  };

  const rl = createInterface({
    input: createReadStream(filePath, { encoding: 'utf8' }),
    crlfDelay: Infinity,
  });

  let firstSessionId = null;

  for await (const line of rl) {
    if (!line.trim()) continue;

    let record;
    try {
      record = JSON.parse(line);
    } catch {
      continue; // skip malformed lines
    }

    const ts = record.timestamp || null;

    // Track timestamps
    if (ts) {
      if (!session.timestamps.first) session.timestamps.first = ts;
      session.timestamps.last = ts;
    }

    // Track session metadata from first record
    if (!session.sessionId && record.sessionId) {
      session.sessionId = record.sessionId;
      firstSessionId = record.sessionId;
    }
    if (!session.version && record.version) session.version = record.version;
    if (!session.cwd && record.cwd) session.cwd = record.cwd;

    // Detect continuation: first sessionId differs from filename UUID
    if (firstSessionId && firstSessionId !== filenameId) {
      session.isContinuation = true;
    }

    const type = record.type;
    const msg = record.message || {};
    const isCompact = record.isCompactSummary === true;

    // ------------------------------------------------------------------
    // compact_boundary
    // ------------------------------------------------------------------
    if (type === 'compact_boundary') {
      session.compactionEvents.count++;
      if (ts) session.compactionEvents.timestamps.push(ts);
      continue;
    }

    // ------------------------------------------------------------------
    // summary (compact summary records)
    // ------------------------------------------------------------------
    if (type === 'summary' || isCompact) {
      // Synthetic records — skip for message counting but note existence
      continue;
    }

    // ------------------------------------------------------------------
    // user records
    // ------------------------------------------------------------------
    if (type === 'user') {
      const content = msg.content;
      const text = extractTextFromContent(content);
      const toolResults = extractToolResults(
        Array.isArray(content) ? content : [],
      );
      const attachments = extractAttachments(
        Array.isArray(content) ? content : [],
      );

      const tokenEst = estimateTokens(text);
      const attachTokens = attachments.reduce((s, a) => s + a.tokens, 0);

      session.messages.push({
        role: 'user',
        type,
        uuid: record.uuid || null,
        parentUuid: record.parentUuid || null,
        timestamp: ts,
        text: text.length > 1000 ? text.slice(0, 1000) + '…' : text,
        tokenEstimate: tokenEst + attachTokens,
        toolResultCount: toolResults.length,
        attachmentCount: attachments.length,
        attachmentTokens: attachTokens,
        userType: record.userType || 'external',
        isSidechain: record.isSidechain || false,
      });

      // Track user turns (real human messages, not system/task notifications)
      const isSystemMsg = text.startsWith('<task-notification') ||
        text.startsWith('<system-reminder') ||
        text.startsWith('<available-deferred-tools');
      if (record.userType !== 'internal' && text.length > 0 && !isSystemMsg) {
        session.turns.push({
          index: session.turns.length,
          timestamp: ts,
          text: text.length > 120 ? text.slice(0, 120) + '…' : text,
          attachments: attachments.length,
          messageIndex: session.messages.length - 1,
        });
      }

      // Track attachments
      for (const att of attachments) {
        att.timestamp = ts;
        session.attachments.push(att);
      }

      // Accumulate token buckets (including attachment tokens)
      session.tokenBuckets.user_msg += tokenEst + attachTokens;

      for (const tr of toolResults) {
        session.toolResults.push(tr);
        session.tokenBuckets.tool_results += tr.tokenEstimate;
      }

      continue;
    }

    // ------------------------------------------------------------------
    // assistant records
    // ------------------------------------------------------------------
    if (type === 'assistant') {
      const content = msg.content;
      const text = extractTextFromContent(content);
      const toolCalls = extractToolCalls(
        Array.isArray(content) ? content : [],
        ts,
      );
      const thinkingBlocks = extractThinkingBlocks(
        Array.isArray(content) ? content : [],
      );

      // Model tracking
      if (msg.model) session.model = msg.model;

      // Usage tracking
      if (msg.usage) session.usage = msg.usage;

      const tokenEst = estimateTokens(text);

      session.messages.push({
        role: 'assistant',
        type,
        uuid: record.uuid || null,
        parentUuid: record.parentUuid || null,
        timestamp: ts,
        text: text.length > 1000 ? text.slice(0, 1000) + '…' : text,
        tokenEstimate: tokenEst,
        model: msg.model || null,
        usage: msg.usage || null,
        toolCallCount: toolCalls.length,
        thinkingBlockCount: thinkingBlocks.length,
        isSidechain: record.isSidechain || false,
      });

      // Accumulate response token bucket
      session.tokenBuckets.responses += tokenEst;

      // Process tool calls
      for (const tc of toolCalls) {
        tc.filePath = extractFilePath(tc);
        session.toolCalls.push(tc);

        // Classify into buckets
        const bucket = classifyBucket(tc);
        if (bucket) {
          session.tokenBuckets[bucket] += tc.tokenEstimate;
        }

        // Skill activations
        if (isSkillActivation(tc)) {
          session.skillActivations.push({
            skill: tc.input.skill || tc.input.skill_name || null,
            args: tc.input.args || null,
            timestamp: ts,
            id: tc.id,
          });
        }

        // Subagent spawns
        if (isSubagentSpawn(tc)) {
          session.subagentSpawns.push({
            tool: tc.name,
            subagentType: tc.input.subagent_type || tc.input.type || null,
            description: tc.input.description || tc.input.prompt || null,
            timestamp: ts,
            id: tc.id,
          });
        }

        // Plan usage
        if (isPlanUsage(tc)) {
          session.planUsage.push({
            tool: tc.name,
            input: tc.input,
            timestamp: ts,
            id: tc.id,
          });
        }
      }

      // Process thinking blocks
      for (const tb of thinkingBlocks) {
        session.thinkingBlocks.count++;
        const thinkEst = estimateTokens(tb.thinking);
        session.thinkingBlocks.totalEstimatedTokens += thinkEst;
        session.tokenBuckets.thinking += thinkEst;
      }

      continue;
    }
  }

  return session;
}
