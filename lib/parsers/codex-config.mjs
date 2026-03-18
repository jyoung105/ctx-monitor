// Codex CLI configuration parser
// Zero npm dependencies — Node.js built-ins only

import { readFile, readdir, stat, access } from 'node:fs/promises';
import { join, basename, dirname } from 'node:path';
import { homedir } from 'node:os';

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function estimateTokens(text) {
  if (!text) return 0;
  if (typeof text !== 'string') text = JSON.stringify(text);
  return Math.ceil(text.length / 4);
}

async function fileExists(p) {
  try {
    await access(p);
    return true;
  } catch {
    return false;
  }
}

async function safeReadFile(p) {
  try {
    return await readFile(p, 'utf8');
  } catch {
    return null;
  }
}

// ---------------------------------------------------------------------------
// Minimal TOML parser
// ---------------------------------------------------------------------------

/**
 * Parse a subset of TOML sufficient for Codex config files.
 *
 * Supports:
 *  - Key = value pairs (strings, numbers, booleans)
 *  - Quoted strings (double-quoted with basic escapes)
 *  - Bare strings (unquoted values that aren't numbers/booleans)
 *  - Arrays: [item, item, ...]  (single-line and multi-line)
 *  - Tables: [table] and [table.subtable]
 *  - Comments: # ...
 *
 * Does NOT support: inline tables, datetime, multi-line basic strings,
 * literal strings, or the full TOML spec.
 */
export function parseToml(text) {
  const root = {};
  let current = root;
  const lines = text.split('\n');
  let i = 0;

  function getOrCreateTable(path) {
    let obj = root;
    for (const key of path) {
      if (obj[key] == null) obj[key] = {};
      obj = obj[key];
    }
    return obj;
  }

  function parseValue(raw) {
    raw = raw.trim();
    if (!raw) return '';

    // Boolean
    if (raw === 'true') return true;
    if (raw === 'false') return false;

    // Quoted string
    if (raw.startsWith('"')) {
      return parseQuotedString(raw);
    }

    // Array (may span multiple lines)
    if (raw.startsWith('[')) {
      return parseArray(raw);
    }

    // Number
    if (/^-?\d+(\.\d+)?$/.test(raw)) {
      return raw.includes('.') ? parseFloat(raw) : parseInt(raw, 10);
    }

    // Bare string (fallback)
    return raw;
  }

  function parseQuotedString(raw) {
    // Find the closing quote, respecting escapes
    let result = '';
    let j = 1; // skip opening quote
    while (j < raw.length) {
      const ch = raw[j];
      if (ch === '\\' && j + 1 < raw.length) {
        const next = raw[j + 1];
        switch (next) {
          case 'n': result += '\n'; break;
          case 't': result += '\t'; break;
          case 'r': result += '\r'; break;
          case '\\': result += '\\'; break;
          case '"': result += '"'; break;
          default: result += '\\' + next; break;
        }
        j += 2;
        continue;
      }
      if (ch === '"') break;
      result += ch;
      j++;
    }
    return result;
  }

  function parseArray(raw) {
    // Collect the full array text, handling multi-line arrays
    let arrayText = raw;
    let bracketDepth = 0;
    for (const ch of arrayText) {
      if (ch === '[') bracketDepth++;
      if (ch === ']') bracketDepth--;
    }
    // If brackets aren't balanced, pull more lines
    while (bracketDepth > 0 && i + 1 < lines.length) {
      i++;
      const nextLine = lines[i];
      arrayText += '\n' + nextLine;
      for (const ch of nextLine) {
        if (ch === '[') bracketDepth++;
        if (ch === ']') bracketDepth--;
      }
    }

    // Strip outer brackets
    const inner = arrayText.trim().slice(1, -1).trim();
    if (!inner) return [];

    // Tokenize respecting quoted strings and nested arrays
    const items = [];
    let buf = '';
    let depth = 0;
    let inQuote = false;
    let escaped = false;

    for (let k = 0; k < inner.length; k++) {
      const ch = inner[k];
      if (escaped) {
        buf += ch;
        escaped = false;
        continue;
      }
      if (ch === '\\' && inQuote) {
        buf += ch;
        escaped = true;
        continue;
      }
      if (ch === '"') {
        inQuote = !inQuote;
        buf += ch;
        continue;
      }
      if (!inQuote) {
        if (ch === '[') depth++;
        if (ch === ']') depth--;
        if (ch === ',' && depth === 0) {
          const val = buf.trim();
          if (val) items.push(parseValue(val));
          buf = '';
          continue;
        }
        // Skip comments at end of array
        if (ch === '#' && depth === 0) {
          // consume rest of this logical line segment
          while (k + 1 < inner.length && inner[k + 1] !== '\n') k++;
          continue;
        }
      }
      buf += ch;
    }
    const last = buf.trim();
    if (last) items.push(parseValue(last));

    return items;
  }

  // Main parse loop
  for (i = 0; i < lines.length; i++) {
    let line = lines[i];

    // Strip comments (outside quotes)
    let inQ = false;
    let commentIdx = -1;
    for (let c = 0; c < line.length; c++) {
      if (line[c] === '"') inQ = !inQ;
      if (line[c] === '#' && !inQ) { commentIdx = c; break; }
    }
    if (commentIdx >= 0) line = line.slice(0, commentIdx);
    line = line.trim();
    if (!line) continue;

    // Table header: [table.path]
    const tableMatch = line.match(/^\[([^\]]+)\]$/);
    if (tableMatch) {
      const path = tableMatch[1].split('.').map(k => k.trim());
      current = getOrCreateTable(path);
      continue;
    }

    // Key = value
    const eqIdx = line.indexOf('=');
    if (eqIdx === -1) continue;

    const key = line.slice(0, eqIdx).trim().replace(/^"|"$/g, '');
    const rawVal = line.slice(eqIdx + 1).trim();
    current[key] = parseValue(rawVal);
  }

  return root;
}

// ---------------------------------------------------------------------------
// Config file discovery
// ---------------------------------------------------------------------------

/**
 * Locate all Codex config-related files.
 * @param {string} [projectPath=process.cwd()] Project root directory
 */
export async function findCodexConfigFiles(projectPath = process.cwd()) {
  const home = homedir();
  const codexHome = process.env.CODEX_HOME || join(home, '.codex');

  const files = {
    globalConfig: join(codexHome, 'config.toml'),
    projectConfig: join(projectPath, '.codex', 'config.toml'),
    globalInstructions: join(codexHome, 'AGENTS.md'),
    globalSkillsDir: join(home, '.agents', 'skills'),
    projectSkillsDir: join(projectPath, '.agents', 'skills'),
  };

  const result = {};
  for (const [key, path] of Object.entries(files)) {
    result[key] = { path, exists: await fileExists(path) };
  }
  return result;
}

// ---------------------------------------------------------------------------
// Skills discovery
// ---------------------------------------------------------------------------

async function discoverSkills(skillsDir) {
  const skills = [];
  let dirEntries;
  try {
    dirEntries = await readdir(skillsDir, { withFileTypes: true });
  } catch {
    return skills;
  }

  for (const entry of dirEntries) {
    if (!entry.isDirectory()) continue;
    const skillFile = join(skillsDir, entry.name, 'SKILL.md');
    const content = await safeReadFile(skillFile);
    if (content != null) {
      skills.push({
        name: entry.name,
        path: skillFile,
        chars: content.length,
        tokens: estimateTokens(content),
      });
    }
  }
  return skills;
}

// ---------------------------------------------------------------------------
// MCP server extraction from parsed config
// ---------------------------------------------------------------------------

function extractMcpServers(config) {
  const mcpServers = config.mcp_servers || {};
  const servers = [];
  let totalTokens = 0;

  for (const [name, def] of Object.entries(mcpServers)) {
    if (typeof def !== 'object' || def === null) continue;
    const server = {
      name,
      url: def.url || null,
      command: def.command || null,
      args: Array.isArray(def.args) ? def.args : [],
      enabledTools: Array.isArray(def.enabled_tools) ? def.enabled_tools : null,
      disabledTools: Array.isArray(def.disabled_tools) ? def.disabled_tools : null,
      estimatedTokens: 0,
    };
    // Estimate token cost of the server definition itself
    server.estimatedTokens = estimateTokens(JSON.stringify(def));
    totalTokens += server.estimatedTokens;
    servers.push(server);
  }

  return { servers, totalTokens };
}

// ---------------------------------------------------------------------------
// Agent definitions extraction
// ---------------------------------------------------------------------------

function extractAgents(config) {
  const agentsDef = config.agents || {};
  const definitions = [];
  let totalTokens = 0;

  for (const [name, def] of Object.entries(agentsDef)) {
    if (typeof def !== 'object' || def === null) continue;
    const agent = {
      name,
      description: def.description || null,
      model: def.model || null,
    };
    const est = estimateTokens(JSON.stringify(def));
    totalTokens += est;
    definitions.push(agent);
  }

  return { definitions, totalTokens };
}

// ---------------------------------------------------------------------------
// Main config parser
// ---------------------------------------------------------------------------

/**
 * Parse all Codex CLI configuration for a project.
 *
 * @param {string} [projectPath=process.cwd()]
 * @returns {Promise<Object>} Merged configuration data
 */
export async function parseCodexConfig(projectPath = process.cwd()) {
  const home = homedir();
  const codexHome = process.env.CODEX_HOME || join(home, '.codex');

  // Read config files in parallel
  const [globalToml, projectToml, instructionsContent] = await Promise.all([
    safeReadFile(join(codexHome, 'config.toml')),
    safeReadFile(join(projectPath, '.codex', 'config.toml')),
    safeReadFile(join(codexHome, 'AGENTS.md')),
  ]);

  // Parse TOML configs
  const globalConfig = globalToml ? parseToml(globalToml) : {};
  const projectConfig = projectToml ? parseToml(projectToml) : {};

  // Merge: project overrides global for top-level keys;
  // mcp_servers and agents are merged at the server/agent level.
  const merged = { ...globalConfig };
  for (const [key, val] of Object.entries(projectConfig)) {
    if (key === 'mcp_servers' && typeof val === 'object' && typeof merged.mcp_servers === 'object') {
      merged.mcp_servers = { ...merged.mcp_servers, ...val };
    } else if (key === 'agents' && typeof val === 'object' && typeof merged.agents === 'object') {
      merged.agents = { ...merged.agents, ...val };
    } else {
      merged[key] = val;
    }
  }

  // Extract structured sections
  const mcp = extractMcpServers(merged);
  const agents = extractAgents(merged);

  // Instructions
  const instructions = instructionsContent != null
    ? {
        path: join(codexHome, 'AGENTS.md'),
        chars: instructionsContent.length,
        tokens: estimateTokens(instructionsContent),
      }
    : { path: join(codexHome, 'AGENTS.md'), chars: 0, tokens: 0 };

  // Discover skills from both global and project dirs
  const [globalSkills, projectSkills] = await Promise.all([
    discoverSkills(join(home, '.agents', 'skills')),
    discoverSkills(join(projectPath, '.agents', 'skills')),
  ]);
  const allSkills = [...globalSkills, ...projectSkills];
  const skillsTotalTokens = allSkills.reduce((sum, s) => sum + s.tokens, 0);

  return {
    model: merged.model || null,
    reasoningEffort: merged.reasoning_effort || merged.reasoningEffort || null,
    compactionThreshold: merged.compaction_threshold || merged.compactionThreshold || null,

    mcp,

    agents,

    instructions,

    skills: {
      files: allSkills,
      count: allSkills.length,
      totalTokens: skillsTotalTokens,
    },
  };
}
