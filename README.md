# ctx-monitor

Context composition monitor for Claude Code and Codex CLI.

`ctx-monitor` reads local session files and config, estimates how the active context window is being used, and renders the result as terminal output, JSON, or a browser dashboard. It is a zero-dependency Node.js CLI built on core modules only.

## What It Does

- Auto-detects Claude Code or Codex CLI from local session data
- Parses local JSONL rollouts/sessions instead of relying on screenshots or manual counting
- Breaks usage into context components such as system instructions, built-in tools, MCP tools, memories, skills, user messages, tool results, model responses, subagents, reasoning tokens, and free space
- Supports terminal views, watch mode, JSON output, and an HTTP dashboard
- Includes a small Claude Code statusline bridge for compact terminal bars

## Requirements

- Node.js 18+
- Local Claude Code data in `~/.claude` and/or Codex CLI data in `~/.codex`

## Install

Run from the repository:

```bash
node bin/ctx-monitor.mjs --help
```

Or link the CLI into your shell:

```bash
npm link
ctx-monitor --help
```

## Quick Start

```bash
# Auto-detect tool from local state
node bin/ctx-monitor.mjs

# Force a specific tool
node bin/ctx-monitor.mjs --claude
node bin/ctx-monitor.mjs --codex

# Alternate views
node bin/ctx-monitor.mjs --table
node bin/ctx-monitor.mjs --order
node bin/ctx-monitor.mjs --agents
node bin/ctx-monitor.mjs --timeline
node bin/ctx-monitor.mjs --compact

# JSON / dashboard / watch mode
node bin/ctx-monitor.mjs --json
node bin/ctx-monitor.mjs --serve
node bin/ctx-monitor.mjs --watch 3

# Simulate a target usage percentage
node bin/ctx-monitor.mjs --pct 75
```

If you linked the package with `npm link`, replace `node bin/ctx-monitor.mjs` with `ctx-monitor`.

## CLI

```text
ctx-monitor [options]

Options:
  --claude, -c          Force Claude Code mode
  --codex, -x           Force Codex CLI mode
  --watch [N], -w [N]   Re-render every N seconds (default: 5)
  --pct <N>, -p <N>     Simulate N% context usage
  --session <id>        Target specific session by UUID or rollout path
  --project <path>      Target specific project directory
  --serve [port]        Start HTTP server with browser dashboard (default: 3456)

Views:
  --table, -t           Component breakdown table only
  --order, -o           Context loading order diagram
  --agents, -a          Subagent/team isolation diagram
  --setup, -s           Print built-in setup guidance
  --timeline            Show context growth timeline
  --diff                Reserved in help text; see caveats below

Statusline integration:
  --statusline          Read Claude Code JSON from stdin, output compact bar
  --statusline-full     Read stdin, output multi-line component breakdown

Output:
  --json                Output structured JSON
  --no-color            Disable ANSI colors
  --compact             Single-line output mode
  --help, -h            Show help
```

Exit codes:

- `0`: success
- `1`: no session found
- `2`: invalid arguments
- `3`: parse error

## Data Sources

### Claude Code

`ctx-monitor` reads from local Claude files such as:

- `~/.claude/projects/<encoded-project-path>/*.jsonl`
- `~/.claude/settings.json`
- `~/.claude.json`
- `<project>/.claude/settings.json`
- `<project>/.mcp.json`
- `~/.claude/CLAUDE.md`, `<project>/CLAUDE.md`, and nested `.claude/CLAUDE.md`
- `~/.claude/agents/*.md`
- skill directories under `~/.claude/skills` and `<project>/.claude/skills`

Claude session selection is project-aware. By default it resolves sessions from the current working directory; use `--project` or `--session` to override that.

### Codex CLI

`ctx-monitor` reads from local Codex files such as:

- `$CODEX_HOME/sessions/**/*.jsonl`
- `~/.codex/sessions/**/*.jsonl` if `CODEX_HOME` is unset
- `$CODEX_HOME/config.toml`
- `<project>/.codex/config.toml`
- `$CODEX_HOME/AGENTS.md`
- skill directories under `~/.agents/skills` and `<project>/.agents/skills`

Codex session discovery is global rather than project-folder scoped; the latest rollout is used unless `--session` is provided.

## Output Shape

`--json` returns a composition object with top-level fields such as:

- `tool`
- `model`
- `sessionId`
- `contextWindowSize`
- `totalUsedPct`
- `totalUsedTokens`
- `freeTokens`
- `components`
- `toolCalls`
- `turns`
- `timestamp`

Each component includes:

- `key`
- `label`
- `tokens`
- `pct`
- `fixed`
- `source`

## Dashboard

Start the web UI:

```bash
node bin/ctx-monitor.mjs --serve
```

The server exposes:

- `GET /` for the HTML dashboard
- `GET /api/data` for the current composition
- `GET /api/sessions` for discovered sessions
- `GET /api/session/:id` for a parsed session payload
- `GET /api/timeline/:id` for timeline data

Default port is `3456`.

## Integration Examples

### Claude Code Statusline

Use the dedicated bridge script in Claude Code:

```json
{
  "statusLine": {
    "type": "command",
    "command": "node /path/to/ctx-monitor/bin/statusline-bridge.mjs"
  }
}
```

### Claude Code Hook

If you want the main CLI to render from Claude statusline JSON on stdin:

```json
{
  "hooks": {
    "PostToolUse": [
      {
        "type": "command",
        "command": "node /path/to/ctx-monitor/bin/ctx-monitor.mjs --statusline --compact"
      }
    ]
  }
}
```

### Codex Side Pane

```bash
tmux split-window -h "node /path/to/ctx-monitor/bin/ctx-monitor.mjs --codex --watch 3"
```

## Caveats

- Token accounting is mixed: when measured usage is unavailable, estimates fall back to simple heuristics and fixed per-component defaults.
- Claude output includes a reserved compact buffer. `totalUsedPct` includes that buffer; `apiMatchPct` in JSON is the value intended to more closely match Claude's own HUD.
- `--diff` appears in the current CLI help, but there is no dedicated diff rendering path wired up yet.
- `--setup` is implemented, but its printed command examples are older alias-style guidance rather than the exact flag-based commands above.

## Development

Useful local checks:

```bash
node bin/ctx-monitor.mjs --help
node bin/ctx-monitor.mjs --claude --json
node bin/ctx-monitor.mjs --codex --json
```
