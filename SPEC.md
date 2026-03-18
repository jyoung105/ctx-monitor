# ctx-monitor — Product Spec / PRD
## Context Composition Monitor for Claude Code & Codex CLI
**Version:** 1.0.0
**Date:** 2026-03-18
**Author:** AI context engineering project

---

## 1. Overview

A zero-dependency Node.js CLI tool that reads real session data from Claude Code and Codex CLI local storage, parses every context component (system prompt, tools, MCP, agents, memory, skills, messages, tool calls, plan, subagent summaries, thinking tokens, compact buffer), renders them as a color-coded stacked bar with per-component breakdown, and optionally serves an HTML visual-explainer dashboard in the browser.

**Two output modes:**
1. **Terminal CLI** — ANSI-colored stacked bar + table + diagrams, watch mode, statusline bridge
2. **Browser dashboard** — `--serve` flag opens an HTML page with interactive stacked bars, timeline, sliders, subagent diagrams

---

## 2. Data Sources — Complete Reference

### 2.1 Claude Code

#### 2.1.1 Session JSONL files
**Location:** `~/.claude/projects/<url-encoded-project-path>/<session-uuid>.jsonl`
**Format:** Append-only JSONL. Each line is one event record.

**Record envelope schema:**
```json
{
  "type": "user" | "assistant" | "summary" | "compact_boundary",
  "uuid": "3fa85f64-...",
  "parentUuid": "1a2b3c4d-..." | null,
  "isSidechain": false,
  "userType": "external" | "internal",
  "sessionId": "797df13f-...",
  "cwd": "/home/user/myapp",
  "version": "2.1.32",
  "timestamp": "2025-07-01T10:43:40.323Z",
  "message": { ... }
}
```

**Key record types to parse:**

| `type` | Contains | Extract |
|--------|----------|---------|
| `"user"` | `message.role = "user"`, `message.content` (string or array) | User message tokens, slash command detection |
| `"assistant"` | `message.role = "assistant"`, `message.content[]` (text, tool_use, thinking blocks), `message.usage`, `message.model` | Token usage per turn, model name, tool calls, thinking tokens |
| `"summary"` | Compact summary records | Compaction events, `isCompactSummary: true` |
| `"compact_boundary"` | Marks where compaction happened | Compaction timestamps |

**Usage object (inside assistant records):**
```json
{
  "usage": {
    "input_tokens": 4,
    "cache_creation_input_tokens": 6462,
    "cache_read_input_tokens": 14187,
    "output_tokens": 1,
    "service_tier": "standard"
  }
}
```

**Tool use blocks (inside assistant `message.content[]`):**
```json
{
  "type": "tool_use",
  "id": "toolu_01XYZ...",
  "name": "Bash",        // or "Read", "Write", "Search", "Skill", "Task", etc.
  "input": { "command": "ls -la" }
}
```

**Tool result blocks (inside subsequent user records):**
```json
{
  "type": "tool_result",
  "tool_use_id": "toolu_01XYZ...",
  "content": "drwxr-xr-x  5 user  staff  160 Mar 18 10:00 ."
}
```

**Thinking blocks (extended thinking):**
```json
{
  "type": "thinking",
  "thinking": "Let me analyze this codebase..."
}
```

**Subagent invocation (Skill tool or Task tool):**
- Tool name `"Skill"` → skill activation. Parse `input` for skill name.
- Tool name `"Task"` → subagent spawn. The result contains only the summary.
- Tool name `"Explore"` / `"Plan"` → built-in subagent invocations.

**Continuation sessions:**
- If first `sessionId` in file differs from filename UUID → this is a continuation.
- Shared `slug` field connects parent and child sessions.
- `parentUuid` bridges across files.
- Records with `isCompactSummary: true` are synthetic — skip for message counting.

#### 2.1.2 Statusline JSON (stdin bridge)
**When:** Claude Code pipes JSON to statusline script after each assistant message.

**Full JSON schema:**
```json
{
  "hook_event_name": "Status",
  "session_id": "abc123...",
  "cwd": "/current/working/directory",
  "model": {
    "id": "claude-sonnet-4-20250514",
    "display_name": "Sonnet 4"
  },
  "workspace": {
    "current_dir": "/home/user/project",
    "git_branch": "main"
  },
  "context_window": {
    "used_percentage": 42.5,
    "context_window_size": 200000,
    "total_input_tokens": 84000,
    "total_output_tokens": 12000
  },
  "current_usage": {
    "input_tokens": 2400,
    "cache_creation_input_tokens": 1200,
    "cache_read_input_tokens": 80400,
    "output_tokens": 850
  },
  "cost": {
    "total_cost_usd": 1.23,
    "total_lines_added": 156,
    "total_lines_removed": 23
  },
  "version": "2.1.32"
}
```

**Note:** `used_percentage` is calculated from input tokens only: `input_tokens + cache_creation_input_tokens + cache_read_input_tokens`. Does NOT include `output_tokens`.

**Note:** `current_usage` is `null` before first API call in a session.

#### 2.1.3 Settings and config files
| File | Purpose | Parse for |
|------|---------|-----------|
| `~/.claude/settings.json` | User settings | `statusLine` config, `hooks`, `permissions`, `env` |
| `~/.claude.json` | Preferences + OAuth | Theme, MCP servers (user scope), OAuth tokens |
| `.claude/settings.json` (project) | Project settings | Project-specific permissions, MCP, hooks |
| `.mcp.json` (project root) | MCP server configs | List of connected MCP servers → estimate MCP token load |
| `~/.claude/CLAUDE.md` | Global memory | File size → estimate memory tokens (~1 token per 4 chars) |
| `{project}/CLAUDE.md` | Project memory | File size → estimate memory tokens |
| `{project}/**/.claude/CLAUDE.md` | Subdir memory | Additional memory file sizes |
| `~/.claude/agents/*.md` | Custom agent definitions | Count + estimate agent metadata tokens |
| `~/.claude/skills/*/SKILL.md` | Installed skills | Count skills, parse frontmatter sizes |
| `.claude/skills/*/SKILL.md` | Project skills | Additional skill frontmatter |

#### 2.1.4 OAuth usage API (optional, for plan limits)
**Endpoint:** `GET https://api.anthropic.com/api/oauth/usage`
**Auth:** Bearer token from `~/.claude/.credentials.json` → `claudeAiOauth.accessToken`
**Headers:** `anthropic-beta: oauth-2025-04-20`

**Response:**
```json
{
  "five_hour": { "utilization": 37.0, "resets_at": "2026-03-18T04:59:59Z" },
  "seven_day": { "utilization": 26.0, "resets_at": "2026-03-22T14:59:59Z" },
  "seven_day_opus": { "utilization": 0.0, "resets_at": null },
  "extra_usage": { "is_enabled": false, "monthly_limit": null, "used_credits": null }
}
```

---

### 2.2 Codex CLI

#### 2.2.1 Session JSONL files
**Location:** `$CODEX_HOME/sessions/YYYY/MM/DD/rollout-<uuid>.jsonl`
**Default CODEX_HOME:** `~/.codex`
**Fallback:** `~/.codex_home/sessions/`

**Format:** JSONL. Each line is a `type: "event_msg"` wrapper or metadata.

**Key event types to parse:**

| `payload.type` | Contains | Extract |
|-----------------|----------|---------|
| `"token_count"` | Cumulative token totals | `total_token_usage`, `last_token_usage`, `input_tokens`, `cached_input_tokens`, `output_tokens`, `reasoning_tokens` |
| `"turn_started"` | Turn metadata | Model name, turn ID |
| `"turn_completed"` | Turn end | Completion status |
| `"context_compacted"` | Compaction event | Pre/post context sizes |
| `"tool_call"` | Tool invocation | Tool name, input params |
| `"tool_result"` | Tool output | Result text size |
| `"response_item"` | Model response chunks | Text content, function calls |
| `"session_meta"` | Session metadata | model_context_window, configured model |

**Token count event schema:**
```json
{
  "type": "event_msg",
  "payload": {
    "type": "token_count",
    "total_token_usage": 45200,
    "last_token_usage": 3200,
    "model_context_window": 200000,
    "input_tokens": 32000,
    "cached_input_tokens": 8000,
    "output_tokens": 5200,
    "reasoning_tokens": 4800
  }
}
```

**Turn context (model identification):**
```json
{
  "turn_context": {
    "model": "gpt-5.4",
    "reasoning_effort": "medium"
  }
}
```

#### 2.2.2 Config files
| File | Purpose | Parse for |
|------|---------|-----------|
| `~/.codex/config.toml` | Main config | Model, MCP servers, agent definitions, sandbox, compaction threshold |
| `.codex/config.toml` (project) | Project config | Project-scoped MCP, agents, skills |
| `~/.codex/AGENTS.md` | Custom instructions | File size → instruction tokens estimate |
| `.agents/skills/*/SKILL.md` | Agent skills | Count + parse frontmatter |
| `~/.agents/skills/*/SKILL.md` | Global skills | Count + parse frontmatter |

#### 2.2.3 Config.toml key fields
```toml
model = "gpt-5.4"
history_compaction_threshold = 120000  # auto-compact trigger

[mcp_servers.figma]
url = "https://mcp.figma.com/mcp"
enabled = true

[mcp_servers.sentry]
command = "npx"
args = ["-y", "@sentry/mcp-server"]
enabled_tools = ["get_issue_details", "search_events"]
disabled_tools = ["screenshot"]

[agents.reviewer]
name = "reviewer"
description = "PR reviewer"
developer_instructions = "Review code..."
model = "gpt-5.4-mini"
model_reasoning_effort = "low"
nickname_candidates = ["Atlas", "Delta"]
```

---

## 3. Context Composition Model

### 3.1 Claude Code — Component Taxonomy (13 components)

Parse and display in this exact loading order:

| # | Component | Key | Color (hex) | Source | Estimated tokens |
|---|-----------|-----|-------------|--------|------------------|
| 1 | System prompt | `system` | `#534AB7` (purple) | Fixed per session | ~3,100-3,200 |
| 2 | Built-in tools | `tools` | `#0F6E56` (dark teal) | Tool schemas (Bash, Read, Write, Search, git, etc.) | ~11,600-19,800 |
| 3 | MCP tools | `mcp` | `#D85A30` (coral) | Count MCP servers from `.mcp.json` + settings × ~600-1,500 tokens/tool | 0-55,000+ |
| 4 | Agent metadata | `agents` | `#378ADD` (blue) | Count `.claude/agents/*.md` files, parse frontmatter | ~69-5,000 |
| 5 | Memory (CLAUDE.md) | `memory` | `#D4537E` (pink) | Sum file sizes of all CLAUDE.md hierarchy ÷ 4 | ~700-6,000 |
| 6 | Skill frontmatter | `skill_meta` | `#3B6D11` (dark green) | Count skills × ~100 tokens each | ~100-3,000 |
| 7 | Active skill body | `skill_body` | `#97C459` (light green) | Detect Skill tool_use in session JSONL | 0-5,000 per skill |
| 8 | Plan / todo | `plan` | `#1D9E75` (teal) | Detect plan mode or todo tool_use | ~0-2,000 |
| 9 | User messages | `user_msg` | `#BA7517` (dark amber) | Count user records × average content length | varies |
| 10 | Tool call results | `tool_results` | `#EF9F27` (bright amber) | Sum tool_result content lengths from JSONL | largest consumer |
| 11 | Claude responses | `responses` | `#854F0B` (brown) | Sum assistant text content lengths from JSONL | 2nd largest |
| 12 | Subagent summaries | `subagent` | `#85B7EB` (light blue) | Detect Task/Explore/Plan tool results | ~300-2,000 per spawn |
| 13 | Compact buffer | `buffer` | `#B4B2A9` (gray) | Fixed ~45,000 reserved | ~45,000 |

### 3.2 Codex CLI — Component Taxonomy (12 components)

| # | Component | Key | Color (hex) | Source | Estimated tokens |
|---|-----------|-----|-------------|--------|------------------|
| 1 | Instructions | `instructions` | `#534AB7` (purple) | Model-specific prompt file size | ~2,500-3,000 |
| 2 | Built-in tools | `tools` | `#0F6E56` (dark teal) | shell, apply_patch, update_plan, file_read schemas | ~8,000-10,000 |
| 3 | MCP tools | `mcp` | `#D85A30` (coral) | Parse `[mcp_servers.*]` from config.toml, count enabled tools | 0-50,000+ |
| 4 | AGENTS.md / memories | `agents` | `#378ADD` (blue) | File size of AGENTS.md + workspace memories | ~500-3,000 |
| 5 | Skills | `skills` | `#639922` (green) | Count `.agents/skills/*/SKILL.md` | 0-5,000 |
| 6 | Plan (update_plan) | `plan` | `#1D9E75` (teal) | Detect update_plan tool calls in JSONL | ~0-2,000 |
| 7 | User messages | `user_msg` | `#BA7517` (dark amber) | Count user items in JSONL | varies |
| 8 | Tool call results | `tool_results` | `#EF9F27` (bright amber) | Sum tool_result content from JSONL | largest consumer |
| 9 | Codex responses | `responses` | `#854F0B` (brown) | Sum response text content from JSONL | 2nd largest |
| 10 | Subagent summaries | `subagent` | `#85B7EB` (light blue) | Detect spawn_agent/wait_agent tool calls | ~300-2,000 per |
| 11 | Reasoning tokens | `reasoning` | `#888780` (gray) | `reasoning_tokens` from token_count events | varies |
| 12 | Free space | `free` | `#E8E6DF` (light) | `model_context_window - total_used` | remainder |

---

## 4. Token Estimation Strategy

### 4.1 Direct measurement (preferred when available)

**From JSONL `usage` objects:**
- `input_tokens` → fresh input processed
- `cache_read_input_tokens` → cached prompt (already counted)
- `cache_creation_input_tokens` → new cache written
- `output_tokens` → Claude/Codex response length

**Context % formula (matches Claude Code):**
```
used_pct = (input_tokens + cache_creation_input_tokens + cache_read_input_tokens) / context_window_size × 100
```

### 4.2 Per-component estimation (when breakdown unavailable)

Since neither tool exposes component-level metrics via API, estimate by:

1. **Fixed overhead (measured from real /context data):**
   - Claude system prompt: 3,200 tokens (constant)
   - Claude system tools: median 17,000 (range 11,600-19,800 depending on enabled tools)
   - Claude compact buffer: 45,000 (constant)
   - Codex instructions: 2,500 (constant)
   - Codex built-in tools: 8,000 (constant)

2. **MCP estimation:**
   - Parse `.mcp.json` (Claude) or `config.toml` `[mcp_servers.*]` (Codex)
   - Count enabled tools per server
   - Estimate: ~700 tokens per tool (median from real data)
   - Known sizes: Playwright MCP = 22 tools × ~650 = ~14,300; Sentry MCP ≈ 10,000; Memory MCP (9 tools) ≈ 6,000

3. **Memory estimation:**
   - Read all CLAUDE.md files in hierarchy (global + project + subdirs)
   - Token estimate: `Math.ceil(charCount / 4)`

4. **Skill estimation:**
   - Count SKILL.md files: frontmatter ≈ 100 tokens each
   - Detect active skills from JSONL tool_use records: body ≈ 2,000-5,000 tokens each

5. **Message components (from JSONL parsing):**
   - Walk all records in current session JSONL
   - For each `type: "user"`: estimate `content` length ÷ 4
   - For each `type: "assistant"`: split `content[]` into:
     - `text` blocks → responses bucket
     - `tool_use` blocks → (minimal, just the call)
     - `thinking` blocks → tracked separately (stripped after each turn)
   - For each `tool_result`: → tool_results bucket (content length ÷ 4)
   - Detect skill activations, subagent spawns, plan tool calls

---

## 5. CLI Interface

### 5.1 Commands

```
ctx-monitor [options]

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
  --timeline            Show context growth timeline from JSONL events
  --diff                Compare two sessions side-by-side

Statusline integration:
  --statusline          Read Claude Code JSON from stdin, output compact bar
  --statusline-full     Read stdin, output multi-line component breakdown

Output:
  --json                Output structured JSON (for piping)
  --no-color            Disable ANSI colors
  --compact             Single-line output mode
  --help, -h            Show help
```

### 5.2 Exit codes

| Code | Meaning |
|------|---------|
| 0 | Success |
| 1 | No session data found |
| 2 | Invalid arguments |
| 3 | Session file parse error |

---

## 6. Browser Dashboard (`--serve`)

### 6.1 Architecture

```
ctx-monitor --serve 3456
```

Starts a zero-dependency HTTP server (Node.js `http` module) that serves a single self-contained HTML file. The HTML file contains:
- Inline CSS (no external stylesheets)
- Inline JavaScript (no build step, no framework)
- Auto-refresh via `fetch('/api/data')` polling every 3 seconds

### 6.2 API endpoints

| Endpoint | Method | Response |
|----------|--------|----------|
| `/` | GET | HTML dashboard page |
| `/api/data` | GET | JSON with current context composition |
| `/api/sessions` | GET | List of available sessions |
| `/api/session/:id` | GET | Full parsed data for a specific session |
| `/api/timeline/:id` | GET | Event-by-event token growth timeline |

### 6.3 `/api/data` response schema

```json
{
  "tool": "claude" | "codex",
  "model": "Claude Sonnet 4",
  "sessionId": "abc123",
  "contextWindowSize": 200000,
  "totalUsedPct": 52.3,
  "totalUsedTokens": 104600,
  "freeTokens": 95400,
  "components": [
    {
      "key": "system",
      "label": "System prompt",
      "tokens": 3200,
      "pct": 1.6,
      "color": "#534AB7",
      "textColor": "#EEEDFE",
      "fixed": true,
      "source": "estimated"
    },
    {
      "key": "tools",
      "label": "Built-in tools",
      "tokens": 19800,
      "pct": 9.9,
      "color": "#0F6E56",
      "textColor": "#E1F5EE",
      "fixed": true,
      "source": "estimated"
    }
  ],
  "compactionEvents": [
    { "timestamp": "2026-03-18T10:30:00Z", "preTokens": 150000, "postTokens": 63000 }
  ],
  "subagents": [
    { "name": "code-reviewer", "status": "completed", "tokensReturned": 350 }
  ],
  "mcpServers": [
    { "name": "sentry", "toolCount": 8, "estimatedTokens": 5600 }
  ],
  "skills": {
    "installed": 15,
    "active": ["systematic-debugging", "pdf"],
    "frontmatterTokens": 1500,
    "bodyTokens": 3500
  },
  "planUsage": {
    "fiveHour": { "utilization": 37.0, "resetsAt": "2026-03-18T15:00:00Z" },
    "sevenDay": { "utilization": 26.0, "resetsAt": "2026-03-22T15:00:00Z" }
  },
  "timestamp": "2026-03-18T10:45:00Z"
}
```

### 6.4 HTML dashboard components

The single HTML file renders these sections:

#### 6.4.1 Header
- Tool name (Claude Code / Codex CLI)
- Model name
- Session ID
- Big percentage number with color coding (green < 50, yellow < 75, red ≥ 75)

#### 6.4.2 Stacked bar (primary visualization)
- Full-width horizontal bar, height 48px, rounded corners
- Each component rendered as a colored segment proportional to its token share
- Hover → tooltip showing component name, token count, percentage
- Click → sendPrompt-style detail expansion below the bar
- Color mapping matches Section 3 tables exactly

#### 6.4.3 Component table
- Rows: colored dot + label + slider + token count + percentage + mini bar
- Sliders for non-fixed components (simulate what-if scenarios)
- Fixed components (system, tools, buffer) have locked sliders
- Real-time recalculation of free space as sliders move

#### 6.4.4 Context loading order timeline
- Vertical timeline with numbered steps
- Each step: icon + component name + token delta + note
- Divider lines at "user types" and "compaction trigger" boundaries
- Animated entrance for watch mode updates

#### 6.4.5 Session timeline (from JSONL events)
- Horizontal scrollable timeline
- X-axis: timestamps from JSONL records
- Y-axis: cumulative token count
- Colored bands showing which component contributed each increment
- Vertical markers at compaction events
- Hover on any point → show the specific message/tool_call that caused the spike

#### 6.4.6 Subagent / team diagram
- Box diagram showing main session ↔ isolated subagent contexts
- Claude: lead + teammates with peer messaging arrows + shared task board
- Codex: parent + spawned agents with report-to-parent-only arrows
- Token cost annotations per subagent

#### 6.4.7 MCP server panel
- List each connected MCP server
- Tool count per server
- Estimated token cost per server
- Total MCP overhead with % of context window

#### 6.4.8 Plan usage panel (Claude only)
- 5-hour utilization bar with reset timer
- 7-day utilization bar with reset timer
- Burn rate calculation

---

## 7. File Structure

```
ctx-monitor/
├── bin/
│   ├── ctx-monitor.mjs           # Main CLI entry point
│   └── statusline-bridge.mjs     # Claude Code statusline stdin→stdout bridge
├── lib/
│   ├── parsers/
│   │   ├── claude-session.mjs    # Parse Claude Code JSONL sessions
│   │   ├── claude-config.mjs     # Parse settings, CLAUDE.md, .mcp.json, skills, agents
│   │   ├── claude-usage-api.mjs  # Optional: fetch plan usage from OAuth API
│   │   ├── codex-session.mjs     # Parse Codex CLI JSONL rollouts
│   │   └── codex-config.mjs      # Parse config.toml, AGENTS.md, skills
│   ├── estimator.mjs             # Context composition estimation engine
│   ├── renderer/
│   │   ├── terminal.mjs          # ANSI terminal rendering (stacked bar, table, diagrams)
│   │   └── html-template.mjs     # Inline HTML dashboard template (exported as string)
│   ├── server.mjs                # HTTP server for --serve mode
│   └── colors.mjs                # Color definitions shared between terminal + HTML
├── package.json
├── README.md
└── SPEC.md                       # This file
```

---

## 8. Implementation Notes

### 8.1 Zero dependencies
The tool must have **zero npm dependencies**. Use only Node.js built-in modules:
- `fs`, `path`, `os` — file system access
- `http` — server for `--serve` mode
- `readline` — stdin processing for statusline bridge
- `child_process` — optional: run `git` for branch info
- `crypto` — optional: session ID hashing

### 8.2 TOML parsing (for Codex config)
Since we cannot use npm packages, implement a minimal TOML parser that handles:
- Key-value pairs: `model = "gpt-5.4"`
- Tables: `[mcp_servers.figma]`
- Arrays: `args = ["npx", "-y", "@sentry/mcp-server"]`
- Booleans: `enabled = true`
- Strings: both `"quoted"` and bare
- Comments: `# ...`

### 8.3 Token estimation accuracy
- Mark every estimate with `source: "estimated"` or `source: "measured"` in JSON output
- When `/context` output is pasted via stdin, parse it directly for exact breakdown → `source: "measured"`
- When reading JSONL, cumulative `usage` objects give measured totals → `source: "measured"` for aggregate
- Per-component split from JSONL parsing is `source: "estimated"` (heuristic allocation)

### 8.4 Performance
- Session JSONL files can be 50MB+. Read with streaming line-by-line (`readline` interface), not `readFileSync`.
- For watch mode, track file mtime and only re-parse when changed.
- Cache OAuth usage API responses for 60 seconds.

### 8.5 Cross-platform
- macOS: OAuth credentials in Keychain (`security find-generic-password -s "Claude Code-credentials" -w`)
- Linux: OAuth credentials in `~/.claude/.credentials.json`
- Windows: OAuth credentials in `%USERPROFILE%\.claude\.credentials.json`
- Path separator handling for session directory names (URL-encoded with `-` separator)

---

## 9. Color Palette Reference

All 13 component colors (used in both terminal ANSI and HTML):

| Component | Hex | ANSI 256 | CSS var safe | Purpose |
|-----------|-----|----------|--------------|---------|
| System prompt | `#534AB7` | `56` | purple-600 | Core identity |
| Built-in tools | `#0F6E56` | `29` | teal-600 | Tool schemas |
| MCP tools | `#D85A30` | `166` | coral-400 | External integrations |
| Agents | `#378ADD` | `33` | blue-400 | Custom agents |
| Memory (CLAUDE.md) | `#D4537E` | `162` | pink-400 | Persistent context |
| Skill frontmatter | `#3B6D11` | `64` | green-600 | Discovery layer |
| Skill body (active) | `#97C459` | `107` | green-200 | On-demand knowledge |
| Plan / todo | `#1D9E75` | `36` | teal-400 | Task management |
| User messages | `#BA7517` | `136` | amber-400 | Human input |
| Tool call results | `#EF9F27` | `214` | amber-200 | AI tool outputs |
| AI responses | `#854F0B` | `94` | amber-800 | Model output |
| Subagent summaries | `#85B7EB` | `110` | blue-200 | Delegated work |
| Compact buffer | `#B4B2A9` | `249` | gray-200 | Reserved space |
| Reasoning tokens | `#888780` | `245` | gray-400 | Codex thinking |
| Free space | `#E8E6DF` | `254` | gray-50 | Available |

---

## 10. Integration Recipes

### 10.1 Claude Code statusline
```json
{
  "statusLine": {
    "type": "command",
    "command": "node /path/to/ctx-monitor/bin/statusline-bridge.mjs"
  }
}
```

### 10.2 Claude Code PostToolUse hook (alert at thresholds)
```json
{
  "hooks": {
    "PostToolUse": [{
      "type": "command",
      "command": "node /path/to/ctx-monitor/bin/ctx-monitor.mjs --statusline --compact"
    }]
  }
}
```

### 10.3 Codex CLI tmux split pane
```bash
tmux new-session -d -s work "codex" \; \
  split-window -h "node /path/to/ctx-monitor/bin/ctx-monitor.mjs --codex --watch 3" \; \
  select-pane -t 0
```

### 10.4 Browser dashboard alongside either tool
```bash
# In one terminal:
node /path/to/ctx-monitor/bin/ctx-monitor.mjs --claude --serve 3456

# Opens http://localhost:3456 with auto-refreshing dashboard
# Keep this running alongside Claude Code or Codex in another terminal
```

### 10.5 CI/automation (JSON output for parsing)
```bash
node ctx-monitor/bin/ctx-monitor.mjs --claude --json --session abc123 | jq '.components[] | select(.pct > 10)'
```

---

## 11. Future Enhancements (out of scope for v1)

- **Real /context parsing**: When Claude Code issue #34879 ships (per-tool token metrics exposed to hooks), replace estimation with direct measurement.
- **Codex multi-agent aggregation**: When Codex issue #14642 ships (combined multi-agent statusline), add per-agent dashboard.
- **WebSocket live streaming**: Replace polling with WebSocket for sub-second dashboard updates.
- **Diff mode**: Compare context composition between two sessions or two points in time.
- **MCP tool catalog**: Auto-detect known MCP servers and use their known token costs instead of estimates.
- **Claude Code plugin**: Package as a native Claude Code plugin (`.skill` format) for `/ctx-monitor` slash command.
