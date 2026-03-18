/**
 * claude-config.mjs — Parser for Claude Code configuration and memory files.
 *
 * Estimates token contribution of each config surface to the context window.
 * Zero npm dependencies; Node.js built-ins only.
 */

import fs from 'node:fs';
import path from 'node:path';
import os from 'node:os';

// ---------------------------------------------------------------------------
// Constants
// ---------------------------------------------------------------------------

const HOME = os.homedir();
const CLAUDE_DIR = path.join(HOME, '.claude');

/** Median tokens per MCP tool (measured from real data). */
const TOKENS_PER_MCP_TOOL = 700;

/** Default assumed tool count when a server doesn't declare its tools. */
const DEFAULT_TOOLS_PER_SERVER = 10;

/** Well-known MCP server sizes (server-name substring -> estimated total tokens). */
const KNOWN_MCP_SIZES = {
  playwright: 14_300,
  sentry: 10_000,
};

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function readJsonSafe(filePath) {
  try {
    if (!fs.existsSync(filePath)) return null;
    const raw = fs.readFileSync(filePath, 'utf-8');
    return JSON.parse(raw);
  } catch {
    return null;
  }
}

function readTextSafe(filePath) {
  try {
    if (!fs.existsSync(filePath)) return null;
    return fs.readFileSync(filePath, 'utf-8');
  } catch {
    return null;
  }
}

function charsToTokens(charCount) {
  return Math.ceil(charCount / 4);
}

/**
 * List immediate subdirectories of `dir`.
 */
function subdirs(dir) {
  try {
    if (!fs.existsSync(dir)) return [];
    return fs
      .readdirSync(dir, { withFileTypes: true })
      .filter((d) => d.isDirectory())
      .map((d) => d.name);
  } catch {
    return [];
  }
}

/**
 * Recursively find files matching a predicate under `root`.
 * Uses fs.readdirSync with { recursive: true }.
 */
function findFilesRecursive(root, predicate) {
  const results = [];
  try {
    if (!fs.existsSync(root)) return results;
    const entries = fs.readdirSync(root, { withFileTypes: true, recursive: true });
    for (const entry of entries) {
      if (!entry.isFile()) continue;
      const parentDir = entry.parentPath ?? entry.path ?? root;
      const fullPath = path.join(parentDir, entry.name);
      if (predicate(fullPath, entry.name)) {
        results.push(fullPath);
      }
    }
  } catch {
    // Silently ignore permission errors, etc.
  }
  return results;
}

// ---------------------------------------------------------------------------
// MCP parsing
// ---------------------------------------------------------------------------

function parseMcpServers(mcpJson) {
  const servers = [];
  const mcpServers = mcpJson?.mcpServers ?? {};

  for (const [name, config] of Object.entries(mcpServers)) {
    // Check for well-known server sizes first.
    const knownKey = Object.keys(KNOWN_MCP_SIZES).find((k) =>
      name.toLowerCase().includes(k),
    );

    let toolCount;
    let estimatedTokens;

    if (knownKey) {
      estimatedTokens = KNOWN_MCP_SIZES[knownKey];
      toolCount = Math.round(estimatedTokens / TOKENS_PER_MCP_TOOL);
    } else if (Array.isArray(config?.tools)) {
      toolCount = config.tools.length;
      estimatedTokens = toolCount * TOKENS_PER_MCP_TOOL;
    } else {
      toolCount = DEFAULT_TOOLS_PER_SERVER;
      estimatedTokens = toolCount * TOKENS_PER_MCP_TOOL;
    }

    servers.push({ name, toolCount, estimatedTokens });
  }

  return servers;
}

/**
 * Parse `.mcp.json` at project root and MCP entries from settings files.
 *
 * @param {string} projectPath — Absolute path to the project root.
 * @returns {{ servers: Array<{name:string, toolCount:number, estimatedTokens:number}>, totalTokens: number }}
 */
export function parseMcpConfig(projectPath) {
  const allServers = [];
  const seen = new Set();

  // 1. Project .mcp.json
  const projectMcp = readJsonSafe(path.join(projectPath, '.mcp.json'));
  if (projectMcp) {
    for (const s of parseMcpServers(projectMcp)) {
      seen.add(s.name);
      allServers.push(s);
    }
  }

  // 2. Project .claude/settings.json may contain mcpServers
  const projSettings = readJsonSafe(path.join(projectPath, '.claude', 'settings.json'));
  if (projSettings) {
    for (const s of parseMcpServers(projSettings)) {
      if (!seen.has(s.name)) {
        seen.add(s.name);
        allServers.push(s);
      }
    }
  }

  // 3. User ~/.claude/settings.json may contain mcpServers
  const userSettings = readJsonSafe(path.join(CLAUDE_DIR, 'settings.json'));
  if (userSettings) {
    for (const s of parseMcpServers(userSettings)) {
      if (!seen.has(s.name)) {
        seen.add(s.name);
        allServers.push(s);
      }
    }
  }

  // 4. User ~/.claude.json may reference MCP servers
  const userPrefs = readJsonSafe(path.join(HOME, '.claude.json'));
  if (userPrefs) {
    for (const s of parseMcpServers(userPrefs)) {
      if (!seen.has(s.name)) {
        seen.add(s.name);
        allServers.push(s);
      }
    }
  }

  const totalTokens = allServers.reduce((sum, s) => sum + s.estimatedTokens, 0);
  return { servers: allServers, totalTokens };
}

// ---------------------------------------------------------------------------
// Memory files (CLAUDE.md)
// ---------------------------------------------------------------------------

/**
 * Find all CLAUDE.md files in the hierarchy that Claude Code would load.
 *
 * @param {string} projectPath
 * @returns {Array<{path: string, chars: number, tokens: number}>}
 */
export function findMemoryFiles(projectPath) {
  const files = [];

  const candidates = [
    path.join(HOME, '.claude', 'CLAUDE.md'),        // global memory
    path.join(projectPath, 'CLAUDE.md'),             // project memory
  ];

  for (const p of candidates) {
    const text = readTextSafe(p);
    if (text != null) {
      files.push({ path: p, chars: text.length, tokens: charsToTokens(text.length) });
    }
  }

  // Subdir memory files: {project}/**/.claude/CLAUDE.md
  const subdirFiles = findFilesRecursive(projectPath, (fullPath, name) => {
    if (name !== 'CLAUDE.md') return false;
    const dir = path.dirname(fullPath);
    // Must be inside a .claude directory, and not the project root's .claude
    if (path.basename(dir) !== '.claude') return false;
    const parentOfDotClaude = path.dirname(dir);
    // Exclude the project root level (already handled) and node_modules, etc.
    if (parentOfDotClaude === projectPath) return false;
    if (fullPath.includes('node_modules')) return false;
    return true;
  });

  for (const p of subdirFiles) {
    const text = readTextSafe(p);
    if (text != null) {
      files.push({ path: p, chars: text.length, tokens: charsToTokens(text.length) });
    }
  }

  return files;
}

// ---------------------------------------------------------------------------
// Agent files
// ---------------------------------------------------------------------------

/**
 * Find all agent definition files in ~/.claude/agents/*.md
 *
 * @returns {Array<{name: string, path: string, tokens: number}>}
 */
export function findAgentFiles() {
  const agentsDir = path.join(CLAUDE_DIR, 'agents');
  const results = [];

  try {
    if (!fs.existsSync(agentsDir)) return results;
    const entries = fs.readdirSync(agentsDir, { withFileTypes: true });
    for (const entry of entries) {
      if (!entry.isFile() || !entry.name.endsWith('.md')) continue;
      const fullPath = path.join(agentsDir, entry.name);
      const text = readTextSafe(fullPath);
      if (text == null) continue;
      const name = entry.name.replace(/\.md$/, '');
      results.push({
        name,
        path: fullPath,
        tokens: charsToTokens(text.length),
      });
    }
  } catch {
    // ignore
  }

  return results;
}

// ---------------------------------------------------------------------------
// Skill files
// ---------------------------------------------------------------------------

/**
 * Find all SKILL.md files (global + project).
 *
 * @param {string} projectPath
 * @returns {Array<{name: string, path: string, frontmatterTokens: number}>}
 */
export function findSkillFiles(projectPath) {
  const results = [];

  const dirs = [
    path.join(CLAUDE_DIR, 'skills'),
    path.join(projectPath, '.claude', 'skills'),
  ];

  for (const skillsDir of dirs) {
    const names = subdirs(skillsDir);
    for (const name of names) {
      const skillPath = path.join(skillsDir, name, 'SKILL.md');
      const text = readTextSafe(skillPath);
      if (text == null) continue;

      // Parse frontmatter size — the YAML block between --- markers.
      let frontmatterTokens = 100; // default estimate
      const fmMatch = text.match(/^---\r?\n([\s\S]*?)\r?\n---/);
      if (fmMatch) {
        frontmatterTokens = charsToTokens(fmMatch[0].length);
      }

      results.push({ name, path: skillPath, frontmatterTokens });
    }
  }

  return results;
}

// ---------------------------------------------------------------------------
// Main: parseClaudeConfig
// ---------------------------------------------------------------------------

/**
 * Full config analysis — aggregates MCP, memory, agents, skills, settings, and OAuth info.
 *
 * @param {string} projectPath — Absolute path to the project root.
 */
export function parseClaudeConfig(projectPath) {
  // --- MCP ---
  const mcp = parseMcpConfig(projectPath);

  // --- Memory ---
  const memoryFiles = findMemoryFiles(projectPath);
  const memory = {
    files: memoryFiles,
    totalTokens: memoryFiles.reduce((sum, f) => sum + f.tokens, 0),
  };

  // --- Agents ---
  const agentFiles = findAgentFiles();
  const agents = {
    files: agentFiles,
    totalTokens: agentFiles.reduce((sum, a) => sum + a.tokens, 0),
  };

  // --- Skills ---
  const skillFiles = findSkillFiles(projectPath);
  const skills = {
    installed: skillFiles,
    count: skillFiles.length,
    totalFrontmatterTokens: skillFiles.reduce((sum, s) => sum + s.frontmatterTokens, 0),
  };

  // --- Settings ---
  const userSettings = readJsonSafe(path.join(CLAUDE_DIR, 'settings.json')) ?? {};
  const projSettings =
    readJsonSafe(path.join(projectPath, '.claude', 'settings.json')) ?? {};

  const settings = {
    statusLine: userSettings.statusLine ?? projSettings.statusLine ?? null,
    hooks: {
      ...(userSettings.hooks ?? {}),
      ...(projSettings.hooks ?? {}),
    },
    permissions: {
      user: userSettings.permissions ?? [],
      project: projSettings.permissions ?? [],
    },
  };

  // --- OAuth ---
  const userPrefs = readJsonSafe(path.join(HOME, '.claude.json')) ?? {};
  const oauth = {
    hasToken: !!(
      userPrefs.oauthToken ??
      userPrefs.oauth_token ??
      userPrefs.accessToken ??
      userPrefs.access_token
    ),
  };

  return { mcp, memory, agents, skills, settings, oauth };
}
