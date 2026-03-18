# ctx-monitor

Context composition monitor for Claude Code and Codex CLI.

`ctx-monitor` reads local session files and config, estimates how the active context window is being used, and renders the result as terminal output, JSON, or a browser dashboard. Single static binary — zero runtime dependencies.

## What It Does

- Auto-detects Claude Code or Codex CLI from local session data
- Parses local JSONL rollouts/sessions (handles files 50MB+)
- Breaks usage into context components: system instructions, built-in tools, MCP tools, memories, skills, user messages, tool results, model responses, subagents, reasoning tokens, and free space
- Supports terminal views, watch mode, JSON output, and an HTTP dashboard
- Includes a statusline bridge for compact terminal bars

## Requirements

- Local Claude Code data in `~/.claude` and/or Codex CLI data in `~/.codex`

## Install

### From source (requires Go 1.23+)

```bash
go install github.com/tonylee/ctx-monitor/cmd/ctx-monitor@latest
```

### Build from repository

```bash
git clone https://github.com/tonylee/ctx-monitor.git
cd ctx-monitor
make build
./ctx-monitor --help
```

### Cross-compile

```bash
make cross
# Produces: ctx-monitor-darwin-amd64, ctx-monitor-darwin-arm64,
#           ctx-monitor-linux-amd64, ctx-monitor-linux-arm64
```

## Quick Start

```bash
# Auto-detect tool from local state
ctx-monitor

# Force a specific tool
ctx-monitor --claude
ctx-monitor --codex

# Alternate views
ctx-monitor --table
ctx-monitor --order
ctx-monitor --agents
ctx-monitor --timeline
ctx-monitor --compact

# JSON / dashboard / watch mode
ctx-monitor --json
ctx-monitor --serve
ctx-monitor --watch 3

# Simulate a target usage percentage
ctx-monitor --pct 75
```

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
  --setup, -s           Print setup guidance
  --timeline            Show context growth timeline

Statusline integration:
  --statusline          Read Claude Code JSON from stdin, output compact bar
  --statusline-full     Read stdin, output multi-line component breakdown

Output:
  --json                Output structured JSON
  --no-color            Disable ANSI colors
  --compact             Single-line output mode
  --version, -v         Show version
  --help, -h            Show help
```

Exit codes:

- `0`: success
- `1`: no session found
- `2`: invalid arguments
- `3`: parse error

## Data Sources

### Claude Code

Reads from local Claude files:

- `~/.claude/projects/<encoded-project-path>/*.jsonl`
- `~/.claude/settings.json`
- `~/.claude.json`
- `<project>/.claude/settings.json`
- `<project>/.mcp.json`
- `~/.claude/CLAUDE.md`, `<project>/CLAUDE.md`, nested `.claude/CLAUDE.md`
- `~/.claude/agents/*.md`
- Skill directories under `~/.claude/skills` and `<project>/.claude/skills`

Session selection is project-aware. Use `--project` or `--session` to override.

### Codex CLI

Reads from local Codex files:

- `$CODEX_HOME/sessions/**/*.jsonl` (or `~/.codex/sessions/`)
- `$CODEX_HOME/config.toml`
- `<project>/.codex/config.toml`
- `$CODEX_HOME/AGENTS.md`
- Skill directories under `~/.agents/skills` and `<project>/.agents/skills`

Codex session discovery is global; the latest rollout is used unless `--session` is provided.

## Output Shape

`--json` returns a composition object with fields:

- `tool`, `model`, `sessionId`, `contextWindowSize`
- `totalUsedPct`, `totalUsedTokens`, `freeTokens`
- `components` (array of `{key, label, tokens, pct, fixed, source}`)
- `toolCalls`, `turns`, `timestamp`

For Claude, `apiMatchPct`/`apiMatchTokens` exclude the buffer reservation to match Claude Code's HUD.

## Dashboard

```bash
ctx-monitor --serve
# Open http://localhost:3456
```

API endpoints: `GET /`, `/api/data`, `/api/sessions`, `/api/session/{id}`, `/api/timeline/{id}`

## Integration Examples

### Claude Code Statusline

```json
{
  "statusLine": {
    "type": "command",
    "command": "/path/to/ctx-monitor --statusline"
  }
}
```

### Codex Side Pane

```bash
tmux split-window -h "ctx-monitor --codex --watch 3"
```

## Development

```bash
make build         # Build binary
make test          # Run tests
make test-race     # Run tests with race detector
make vet           # Run go vet
make clean         # Remove build artifacts
```

## Architecture

```
cmd/ctx-monitor/main.go        CLI entry point
internal/
  model/                        Types: components, sessions, configs, registry
  parser/
    claude/                     Claude session, config, usage API parsers
    codex/                      Codex session, config parsers
    toml/                       Minimal TOML parser
  estimator/                    Context composition estimation engine
  renderer/                     Terminal ANSI renderer + embedded HTML dashboard
  server/                       HTTP API server
```

Zero external Go modules — stdlib only. HTML dashboard is compiled into the binary via `//go:embed`.

## Caveats

- Token accounting is mixed: when measured usage is unavailable, estimates fall back to heuristics and fixed defaults.
- Claude output includes a reserved compact buffer. `totalUsedPct` includes that buffer; `apiMatchPct` matches Claude's HUD more closely.
