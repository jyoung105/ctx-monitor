# ctx-monitor — SPEC v2 Addendum
## Exact Tool Registry, File-Level Detail Extraction, Brand Design Systems & Visual Explainer

**Appended to:** SPEC.md v1.0.0
**Date:** 2026-03-18

---

## A. Exact Tool Registries

### A.1 Claude Code — Complete Built-in Tool Names

These are the exact `name` strings that appear in `tool_use` blocks inside session JSONL.

**Core tools (always available):**

| Tool name | Category | Purpose | Typical token cost per call |
|-----------|----------|---------|---------------------------|
| `Bash` | Execute | Run shell commands | ~50 input + variable output |
| `Read` | File I/O | Read file contents | ~20 input + file_size/4 output |
| `Write` | File I/O | Write/create files | ~20 input + content/4 |
| `Edit` | File I/O | Edit existing files (surgical replace) | ~30 input + diff_size/4 |
| `MultiEdit` | File I/O | Batch edit multiple locations in one file | ~40 input + diffs/4 |
| `Glob` | Search | Find files by pattern (fast, uses glob) | ~15 input + match_count × ~10 |
| `Grep` | Search | Search content in files (uses ripgrep) | ~20 input + match_lines × ~15 |
| `LS` | File I/O | List directory contents | ~10 input + entries × ~8 |
| `NotebookRead` | Notebook | Read Jupyter notebook cells | ~20 input + cells/4 |
| `NotebookEdit` | Notebook | Edit Jupyter notebook cells | ~30 input + diff/4 |
| `WebFetch` | Web | Fetch URL content with prompt | ~25 input + page_content/4 |
| `WebSearch` | Web | Search the web | ~15 input + results × ~100 |
| `TodoRead` | Planning | Read current todo list | ~10 input + todos × ~20 |
| `TodoWrite` | Planning | Create/update todo list | ~30 input (includes todo items) |

**Agent/subagent tools:**

| Tool name | Category | Purpose | Token impact on main session |
|-----------|----------|---------|------------------------------|
| `Task` | Subagent | Spawn general-purpose, Explore, or Plan subagent | Only summary returns (~300-2,000) |
| `Skill` | Skill | Invoke a skill (meta-tool) | Injects SKILL.md body (~2,000-5,000) |
| `SendMessageTool` | Communication | Send message to user (agent teams) | ~message_length/4 |
| `TaskList` | Teams | Manage shared task board (agent teams) | ~task_count × 30 |
| `Computer` | Browser | Chrome browser interaction (if enabled) | Variable |

**Built-in subagent types** (appear in `Task` tool's `subagent_type` parameter):

| `subagent_type` value | Description | Tools available to subagent |
|----------------------|-------------|---------------------------|
| `"general-purpose"` | Complex multi-step tasks | All tools (`*`) |
| `"Explore"` | Read-only codebase exploration | `Glob`, `Grep`, `Read`, `Bash` |
| `"Plan"` | Research for plan mode | `Glob`, `Grep`, `Read`, `Bash` |
| `"statusline-setup"` | Configure statusline | `Read`, `Edit` |
| `"output-style-setup"` | Create output style | `Read`, `Write`, `Edit`, `Glob`, `LS`, `Grep` |
| Custom agents from `.claude/agents/*.md` | User-defined | As specified in `allowed-tools` frontmatter |

**MCP tools** (naming pattern):
```
mcp__<server-name>__<tool-name>
```
Examples: `mcp__memory__create_entities`, `mcp__sentry__get_issue_details`, `mcp__playwright__screenshot`

### A.2 Codex CLI — Complete Built-in Tool Names

| Tool name | Category | Purpose |
|-----------|----------|---------|
| `shell` | Execute | Run terminal commands (equivalent to Bash) |
| `apply_patch` | File I/O | Apply diffs (model trained on this specific format) |
| `file_read` | File I/O | Read file contents |
| `update_plan` | Planning | Create/update structured plan |
| `spawn_agent` | Subagent | Spawn a parallel subagent |
| `wait_agent` | Subagent | Wait for subagent completion, collect results |
| `send_input` | Subagent | Send additional input to a running agent |
| `web_search` | Web | Search the web |
| `spawn_agents_on_csv` | Orchestration | Fan out work from CSV, one agent per row |

**Custom agent definitions** (in `config.toml`):
```toml
[agents.reviewer]
name = "reviewer"
description = "PR reviewer focused on correctness"
developer_instructions = "Review code like an owner..."
model = "gpt-5.4-mini"
model_reasoning_effort = "low"
nickname_candidates = ["Atlas", "Delta", "Echo"]
```

---

## B. File-Level & Line-Level Detail Extraction

### B.1 Extracting exact file paths and line numbers from Claude Code JSONL

**Read tool calls contain the exact file path and optional line range:**
```json
{
  "type": "tool_use",
  "name": "Read",
  "input": {
    "file_path": "/home/user/myapp/src/auth/login.ts",
    "offset": 45,
    "limit": 120
  }
}
```
→ Extract: `file_path`, `offset` (start line), `limit` (line count)

**Edit tool calls contain file path and exact match strings:**
```json
{
  "type": "tool_use",
  "name": "Edit",
  "input": {
    "file_path": "/home/user/myapp/src/auth/login.ts",
    "old_string": "const token = jwt.sign(payload, secret);",
    "new_string": "const token = jwt.sign(payload, secret, { expiresIn: '24h' });"
  }
}
```
→ Extract: `file_path`, `old_string` (for locating the line), `new_string`

**Grep results contain file path and line numbers:**
```json
{
  "type": "tool_result",
  "content": "/home/user/myapp/src/auth/login.ts:47:  const token = jwt.sign(payload, secret);\n/home/user/myapp/src/auth/refresh.ts:23:  const refreshToken = jwt.sign(..."
}
```
→ Parse: `filepath:line_number:content` format (ripgrep output)

**Bash tool calls with file context:**
```json
{
  "type": "tool_use",
  "name": "Bash",
  "input": {
    "command": "git diff HEAD~1 -- src/auth/login.ts",
    "description": "Check recent changes to login"
  }
}
```
→ Extract: command arguments for file paths, description for context

**Skill invocations:**
```json
{
  "type": "tool_use",
  "name": "Skill",
  "input": {
    "skill_name": "systematic-debugging",
    "arguments": "auth module"
  }
}
```
→ Extract: `skill_name` for "which skill is active" tracking

**Task/subagent spawns:**
```json
{
  "type": "tool_use",
  "name": "Task",
  "input": {
    "subagent_type": "Explore",
    "description": "Find authentication implementation",
    "prompt": "Search the codebase to find where user authentication is implemented..."
  }
}
```
→ Extract: `subagent_type`, `description`, `prompt` summary

### B.2 What to display per tool call

For each tool call detected in the JSONL, display:

```
┌ Read  src/auth/login.ts:45-165  [~3.2k tokens]
├ Grep  "jwt.sign" across **/*.ts  [12 matches, ~800 tokens]
├ Edit  src/auth/login.ts  old→new [+1 line, ~200 tokens]
├ Bash  npm test -- --grep "auth"  [~1.5k tokens output]
├ Skill systematic-debugging  [+3.5k tokens loaded]
├ Task  Explore "Find auth implementation"  [→isolated, ~300 tokens summary returned]
└ TodoWrite  3 items (1 in_progress, 2 pending)  [~150 tokens]
```

### B.3 Codex CLI — extracting from rollout JSONL

**Tool call events:**
```json
{
  "type": "event_msg",
  "payload": {
    "type": "tool_call",
    "tool_name": "shell",
    "arguments": { "command": "cat src/auth/login.ts" }
  }
}
```

**Tool result events:**
```json
{
  "type": "event_msg",
  "payload": {
    "type": "tool_result",
    "tool_call_id": "call_abc123",
    "content": "..."
  }
}
```

**apply_patch contains the exact diff:**
```json
{
  "type": "event_msg",
  "payload": {
    "type": "tool_call",
    "tool_name": "apply_patch",
    "arguments": {
      "patch": "*** Begin Patch\n*** src/auth/login.ts\n@@@ -45,3 +45,4 @@@\n  const token = jwt.sign(payload, secret);\n+ const expiresIn = '24h';\n"
    }
  }
}
```
→ Parse: file path from `*** <filepath>`, line numbers from `@@@ -start,count +start,count @@@`

---

## C. Brand Design Systems (0-900 Ramps)

### C.1 Claude Code — Anthropic brand theme

Primary ramp: **Coral / Warm Terracotta** (Anthropic's brand color)

| Stop | Hex | Usage |
|------|-----|-------|
| 50 | `#FFF5F0` | Page background, lightest surface |
| 100 | `#FFE0D1` | Card backgrounds, hover states |
| 200 | `#FFC4A8` | Active surfaces, selected states |
| 300 | `#FFA07E` | Borders on active elements |
| 400 | `#E8784A` | Secondary buttons, accents |
| 500 | `#D4603A` | Primary accent color (Anthropic brand) |
| 600 | `#B84A28` | Primary buttons, strong accents |
| 700 | `#943A1E` | Hover on primary buttons |
| 800 | `#702C16` | Text on light backgrounds |
| 900 | `#4A1B0C` | Darkest text, headings |

Secondary ramp: **Slate / Cool Gray** (for structure)

| Stop | Hex | Usage |
|------|-----|-------|
| 50 | `#F8F7F5` | Background surface |
| 100 | `#EEEDEB` | Card borders, dividers |
| 200 | `#D8D6D2` | Secondary borders |
| 300 | `#B8B5B0` | Disabled states |
| 400 | `#8A8780` | Placeholder text |
| 500 | `#6B6862` | Secondary text |
| 600 | `#504E49` | Body text |
| 700 | `#3A3835` | Headings |
| 800 | `#252422` | Primary text |
| 900 | `#121110` | Darkest, near black |

**Component color assignments (using context taxonomy colors):**

| Component | Ramp | Fill (light) | Fill (dark) | Text (light) | Text (dark) |
|-----------|------|-------------|-------------|-------------|-------------|
| System prompt | Purple | `#EEEDFE` (50) | `#3C3489` (800) | `#3C3489` (800) | `#EEEDFE` (50) |
| Built-in tools | Teal | `#E1F5EE` (50) | `#085041` (800) | `#085041` (800) | `#E1F5EE` (50) |
| MCP tools | Coral | `#FAECE7` (50) | `#712B13` (800) | `#712B13` (800) | `#FAECE7` (50) |
| Agents | Blue | `#E6F1FB` (50) | `#0C447C` (800) | `#0C447C` (800) | `#E6F1FB` (50) |
| Memory | Pink | `#FBEAF0` (50) | `#72243E` (800) | `#72243E` (800) | `#FBEAF0` (50) |
| Skill frontmatter | Green | `#EAF3DE` (50) | `#27500A` (800) | `#27500A` (800) | `#EAF3DE` (50) |
| Skill body | Green | `#C0DD97` (100) | `#3B6D11` (600) | `#173404` (900) | `#C0DD97` (100) |
| Plan / todo | Teal | `#9FE1CB` (100) | `#0F6E56` (600) | `#04342C` (900) | `#9FE1CB` (100) |
| User messages | Amber | `#FAEEDA` (50) | `#633806` (800) | `#633806` (800) | `#FAEEDA` (50) |
| Tool results | Amber | `#FAC775` (100) | `#854F0B` (600) | `#412402` (900) | `#FAC775` (100) |
| Responses | Amber | `#EF9F27` (400) | `#854F0B` (600) | `#412402` (900) | `#FAEEDA` (50) |
| Subagent summaries | Blue | `#B5D4F4` (100) | `#185FA5` (600) | `#042C53` (900) | `#B5D4F4` (100) |
| Compact buffer | Gray | `#D3D1C7` (100) | `#444441` (700) | `#2C2C2A` (900) | `#D3D1C7` (100) |
| Free space | Gray | `#F1EFE8` (50) | `#2C2C2A` (900) | `#5F5E5A` (600) | `#F1EFE8` (50) |

### C.2 Codex CLI — OpenAI brand theme

Primary ramp: **Emerald / Green** (OpenAI's brand color)

| Stop | Hex | Usage |
|------|-----|-------|
| 50 | `#F0FDF4` | Page background |
| 100 | `#DCFCE7` | Card backgrounds |
| 200 | `#BBF7D0` | Active surfaces |
| 300 | `#86EFAC` | Borders on active |
| 400 | `#4ADE80` | Secondary accents |
| 500 | `#10A37F` | Primary accent (OpenAI brand) |
| 600 | `#0D8A6A` | Primary buttons |
| 700 | `#0A6B52` | Hover states |
| 800 | `#064E3B` | Text on light |
| 900 | `#022C22` | Darkest text |

Secondary ramp: **Neutral / Warm Gray** (for structure)

| Stop | Hex | Usage |
|------|-----|-------|
| 50 | `#FAFAF9` | Background |
| 100 | `#F5F5F4` | Surfaces |
| 200 | `#E7E5E4` | Borders |
| 300 | `#D6D3D1` | Disabled |
| 400 | `#A8A29E` | Placeholder |
| 500 | `#78716C` | Secondary text |
| 600 | `#57534E` | Body text |
| 700 | `#44403C` | Headings |
| 800 | `#292524` | Primary text |
| 900 | `#1C1917` | Darkest |

**Component color assignments for Codex** follow the same taxonomy colors from Section 3 but with the green-primary accent for UI chrome (tabs, buttons, borders).

### C.3 Design tokens (CSS custom properties)

```css
/* Claude Code theme */
:root[data-theme="claude"] {
  --brand-50: #FFF5F0;
  --brand-500: #D4603A;
  --brand-600: #B84A28;
  --brand-900: #4A1B0C;
  --surface-0: #FFFFFF;
  --surface-1: #F8F7F5;
  --surface-2: #EEEDEB;
  --text-primary: #252422;
  --text-secondary: #6B6862;
  --text-tertiary: #8A8780;
  --border-default: #D8D6D2;
  --border-subtle: #EEEDEB;
}

/* Codex CLI theme */
:root[data-theme="codex"] {
  --brand-50: #F0FDF4;
  --brand-500: #10A37F;
  --brand-600: #0D8A6A;
  --brand-900: #022C22;
  --surface-0: #FFFFFF;
  --surface-1: #FAFAF9;
  --surface-2: #F5F5F4;
  --text-primary: #292524;
  --text-secondary: #78716C;
  --text-tertiary: #A8A29E;
  --border-default: #E7E5E4;
  --border-subtle: #F5F5F4;
}

/* Shared dark mode */
:root[data-theme="claude"].dark,
:root[data-theme="codex"].dark {
  --surface-0: #121110;
  --surface-1: #1E1D1C;
  --surface-2: #2A2928;
  --text-primary: #EEEDEB;
  --text-secondary: #8A8780;
  --text-tertiary: #6B6862;
  --border-default: #3A3835;
  --border-subtle: #2A2928;
}
```

---

## D. Visual Explainer — HTML Dashboard Component Spec

### D.1 Route: `--serve [port]`

Launches `http://localhost:3456` serving a single self-contained HTML file.

### D.2 Page structure

```
┌────────────────────────────────────────────────────────┐
│  Header: Tool logo + model + session ID + big % number │
├────────────────────────────────────────────────────────┤
│  Stacked composition bar (full width, 48px)            │
│  ████████░░░░░░████████████████████████░░░░░░░░░░░░░░ │
├────────────────────────────────────────────────────────┤
│  Legend: colored dots + component names (inline flex)   │
├────────────────────────────────────────────────────────┤
│ ┌──── Tabs ──────────────────────────────────────────┐ │
│ │ [Components] [Timeline] [Tool calls] [Agents] [Setup]│
│ └────────────────────────────────────────────────────┘ │
│                                                        │
│  [Components tab]                                      │
│  Row per component:                                    │
│  🟣 System prompt    ═══░░░░░░░░░░░  3.2k   1.6%     │
│  🟢 Built-in tools   ═══════░░░░░░░ 19.8k   9.9%     │
│  🟠 MCP tools        ═══════════░░░ 26.5k  13.3%     │
│  ...                                                   │
│  [slider for non-fixed components]                     │
│                                                        │
│  [Timeline tab]                                        │
│  Area chart: x=time, y=tokens, colored stacks          │
│  Vertical markers at compaction events                 │
│                                                        │
│  [Tool calls tab]  ← NEW                               │
│  ┌ Read  src/auth/login.ts:45-165     [~3.2k tokens]  │
│  ├ Grep  "jwt.sign" across **/*.ts    [12 matches]    │
│  ├ Edit  src/auth/login.ts            [+1 line]       │
│  ├ Skill systematic-debugging         [+3.5k loaded]  │
│  ├ Task  Explore "Find auth impl"     [isolated→300]  │
│  └ TodoWrite  3 items                 [~150 tokens]   │
│                                                        │
│  [Agents tab]                                          │
│  Box diagram: lead ↔ teammates/subagents               │
│                                                        │
│  [Setup tab]                                           │
│  Integration instructions for both tools               │
└────────────────────────────────────────────────────────┘
```

### D.3 Tool calls panel — detailed extraction view

For each tool call found in the JSONL, render a card:

```html
<div class="tool-call" data-tool="Read" data-tokens="3200">
  <div class="tool-call-header">
    <span class="tool-icon" style="background: var(--color-tools)">R</span>
    <span class="tool-name">Read</span>
    <span class="tool-target">src/auth/login.ts</span>
    <span class="tool-line">:45-165</span>
    <span class="tool-tokens">~3.2k tokens</span>
  </div>
  <div class="tool-call-detail">
    <!-- collapsible: first 3 lines of output -->
  </div>
</div>
```

**Tool call card design:**
- Left border color matches the component category (amber for Read/Write, teal for subagents, green for skills)
- Monospace font for file paths and line numbers
- Token cost badge on the right, color-coded by size (gray < 1k, yellow 1-5k, red > 5k)
- Collapsible output preview (first 3 lines)
- Click to expand full output

**Aggregation view** (above the list):
```
Tool call summary: 47 calls in session
──────────────────────────────────────
Read      ██████████████████  23 calls  (62.4k tokens)
Bash      ████████            12 calls  (18.2k tokens)  
Edit      ████                 5 calls   (2.1k tokens)
Grep      ███                  4 calls   (3.8k tokens)
Task      ██                   2 calls   (0.6k tokens returned)
Skill     █                    1 call    (3.5k tokens loaded)
```

### D.4 Skill detail panel

When a `Skill` tool call is detected, show:

```
┌─────────────────────────────────────────────────────┐
│ 🟢 Skill: systematic-debugging                      │
│    Source: ~/.claude/skills/systematic-debugging/     │
│    SKILL.md body: ~3,500 tokens loaded               │
│    Allowed tools: Read, Grep, Glob, Bash             │
│    Triggered by: user message "debug the auth flow"  │
│    References loaded:                                │
│      └ references/debugging-checklist.md (~800 tok)  │
│    Scripts executed:                                 │
│      └ scripts/trace-errors.sh (output: 12 lines)   │
└─────────────────────────────────────────────────────┘
```

### D.5 Subagent detail panel

When a `Task` tool call is detected, show:

```
┌─────────────────────────────────────────────────────┐
│ 🔵 Subagent: Explore                                │
│    Prompt: "Find authentication implementation"      │
│    Tools: Glob, Grep, Read, Bash (read-only)        │
│    Status: completed                                │
│    Duration: ~12 seconds                            │
│    Context used: ~45k tokens (in isolated window)   │
│    Summary returned: ~350 tokens to main session    │
│    Files explored:                                  │
│      src/auth/login.ts                              │
│      src/auth/middleware.ts                          │
│      src/auth/jwt.ts                                │
│      lib/session.ts                                 │
└─────────────────────────────────────────────────────┘
```

### D.6 HTML rendering requirements

- Single self-contained `.html` file (no external CSS/JS dependencies)
- CSS custom properties for theming (auto-detect tool → apply Claude coral or Codex green)
- Dark mode via `prefers-color-scheme` media query
- All component colors use the exact ramp stops from Section C
- Font stack: `"Inter", "SF Pro Display", -apple-system, system-ui, sans-serif`
- Monospace: `"SF Mono", "JetBrains Mono", "Fira Code", monospace`
- Auto-refresh via `setInterval(() => fetch('/api/data'), 3000)`
- Responsive: works at 320px mobile through 1920px desktop
- Stacked bar hover tooltips use absolute positioning (not fixed)
- Charts rendered with `<canvas>` API directly (no Chart.js dependency)
- Smooth transitions: `transition: width 0.4s ease, opacity 0.2s ease`

### D.7 Terminal ANSI color mapping

For terminal output, map each component to 256-color ANSI:

| Component | ANSI 256 BG | ANSI 256 FG | Bright variant |
|-----------|-------------|-------------|----------------|
| System prompt | `\x1b[48;5;56m` | `\x1b[38;5;255m` | `\x1b[48;5;141m` |
| Built-in tools | `\x1b[48;5;29m` | `\x1b[38;5;255m` | `\x1b[48;5;43m` |
| MCP tools | `\x1b[48;5;166m` | `\x1b[38;5;255m` | `\x1b[48;5;208m` |
| Agents | `\x1b[48;5;33m` | `\x1b[38;5;255m` | `\x1b[48;5;75m` |
| Memory | `\x1b[48;5;162m` | `\x1b[38;5;255m` | `\x1b[48;5;206m` |
| Skill front. | `\x1b[48;5;64m` | `\x1b[38;5;255m` | `\x1b[48;5;107m` |
| Skill body | `\x1b[48;5;107m` | `\x1b[38;5;232m` | `\x1b[48;5;150m` |
| Plan | `\x1b[48;5;36m` | `\x1b[38;5;255m` | `\x1b[48;5;79m` |
| User messages | `\x1b[48;5;136m` | `\x1b[38;5;255m` | `\x1b[48;5;178m` |
| Tool results | `\x1b[48;5;214m` | `\x1b[38;5;232m` | `\x1b[48;5;220m` |
| Responses | `\x1b[48;5;94m` | `\x1b[38;5;255m` | `\x1b[48;5;136m` |
| Subagent | `\x1b[48;5;110m` | `\x1b[38;5;232m` | `\x1b[48;5;153m` |
| Buffer | `\x1b[48;5;249m` | `\x1b[38;5;232m` | `\x1b[48;5;252m` |
| Reasoning | `\x1b[48;5;245m` | `\x1b[38;5;255m` | `\x1b[48;5;248m` |
| Free space | `\x1b[48;5;254m` | `\x1b[38;5;245m` | — |

---

## E. JSONL Parsing — Complete Field Extraction Map

### E.1 Claude Code session record — all extractable fields

```
Record fields:
├── type                    → "user" | "assistant" | "summary" | "compact_boundary"
├── uuid                    → unique record ID
├── parentUuid              → parent record (DAG linking)
├── isSidechain             → boolean (subagent sidechain)
├── userType                → "external" (human) | "internal" (system)
├── sessionId               → session UUID
├── cwd                     → working directory at time of record
├── version                 → Claude Code version string
├── timestamp               → ISO 8601
├── slug                    → human-readable session name
├── isCompactSummary        → boolean (skip these for message counting)
├── message
│   ├── role                → "user" | "assistant"
│   ├── id                  → message ID (msg_01XXX...)
│   ├── model               → model ID string
│   ├── content             → string | array of content blocks
│   │   ├── [type: "text"]
│   │   │   └── text        → response text content
│   │   ├── [type: "tool_use"]
│   │   │   ├── id          → tool call ID (toolu_01XXX...)
│   │   │   ├── name        → EXACT tool name (see registry above)
│   │   │   └── input       → tool-specific parameters
│   │   ├── [type: "tool_result"]
│   │   │   ├── tool_use_id → references tool_use.id
│   │   │   └── content     → result text or structured data
│   │   ├── [type: "thinking"]
│   │   │   └── thinking    → extended thinking text
│   │   └── [type: "redacted_thinking"]
│   │       └── data        → encrypted thinking (can't read)
│   ├── usage
│   │   ├── input_tokens
│   │   ├── cache_creation_input_tokens
│   │   ├── cache_read_input_tokens
│   │   ├── output_tokens
│   │   └── service_tier    → "standard" | "fast"
│   └── stop_reason         → "end_turn" | "tool_use" | "max_tokens"
└── requestId               → API request ID
```

### E.2 Codex CLI rollout record — all extractable fields

```
Record fields:
├── type                    → "event_msg" | "session_meta" | ...
├── payload
│   ├── type                → event type discriminator
│   │   ├── "token_count"
│   │   │   ├── total_token_usage
│   │   │   ├── last_token_usage
│   │   │   ├── model_context_window
│   │   │   ├── input_tokens
│   │   │   ├── cached_input_tokens
│   │   │   ├── output_tokens
│   │   │   └── reasoning_tokens
│   │   ├── "turn_started"
│   │   │   └── (model info via turn_context)
│   │   ├── "turn_completed"
│   │   │   └── (completion status)
│   │   ├── "context_compacted"
│   │   │   └── (pre/post sizes)
│   │   ├── "tool_call"
│   │   │   ├── tool_name   → EXACT tool name (see registry)
│   │   │   ├── tool_call_id
│   │   │   └── arguments   → tool-specific params
│   │   ├── "tool_result"
│   │   │   ├── tool_call_id
│   │   │   └── content     → result text
│   │   ├── "response_item"
│   │   │   └── (model response chunks)
│   │   └── "session_meta"
│   │       └── (initial session metadata)
├── turn_context
│   ├── model               → model name string
│   └── reasoning_effort    → "low" | "medium" | "high" | "xhigh"
└── timestamp               → ISO 8601 (if present)
```

---

## F. Implementation Checklist

- [ ] Parse Claude Code JSONL: extract all tool_use by name, map to component buckets
- [ ] Parse Codex JSONL: extract all event_msg by payload.type, map to component buckets  
- [ ] Count tokens per component from content lengths (÷4 heuristic) + usage objects
- [ ] Track file paths from Read/Edit/Grep tool calls with line numbers
- [ ] Track active skills from Skill tool invocations
- [ ] Track subagent spawns from Task tool invocations with subagent_type
- [ ] Detect compaction events (compact_boundary records / context_compacted events)
- [ ] Render terminal stacked bar with 15 distinct ANSI 256 colors
- [ ] Render terminal component table with mini bars and percentages
- [ ] Render terminal tool call log with file:line detail
- [ ] Serve HTML dashboard with brand-themed design system (Claude coral / Codex green)
- [ ] HTML: stacked bar with hover tooltips
- [ ] HTML: component table with interactive sliders
- [ ] HTML: tool call timeline with file:line extraction
- [ ] HTML: skill detail cards with references and scripts
- [ ] HTML: subagent detail cards with explored files
- [ ] HTML: dark mode support via CSS custom properties
- [ ] HTML: auto-refresh via polling `/api/data` every 3 seconds
- [ ] Statusline bridge: read Claude Code stdin JSON, output compact composition bar
- [ ] Watch mode: track file mtime, re-parse only on change
- [ ] JSON output mode for CI/automation piping
