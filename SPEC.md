# ctx-monitor — Technical Specification

**Project:** Context window composition monitor for Claude Code & Codex CLI
**Language:** Go 1.23+
**Distribution:** Single statically-linked binary, zero external dependencies
**Version:** 1.0.0
**Date:** 2026-03-19

---

## 1. Project Structure

```
ctx-monitor/
├── cmd/ctx-monitor/
│   └── main.go                    # CLI entry point, argument parsing, main event loop
├── internal/
│   ├── model/
│   │   ├── colors.go              # 13 Claude + 12 Codex component definitions
│   │   ├── composition.go         # Session, component, and timeline data types
│   │   ├── config.go              # Config structure for CLI/HTTP
│   │   ├── registry.go            # 40+ model entries (Claude, GPT, o3, etc.)
│   │   └── session.go             # Session discovery and metadata
│   ├── parser/
│   │   ├── toml/
│   │   │   └── toml.go            # Minimal TOML parser (Codex config only)
│   │   ├── claude/
│   │   │   ├── session.go         # JSONL session file parser + discovery
│   │   │   ├── config.go          # Claude Code config parser
│   │   │   └── usage.go           # Claude OAuth usage API client
│   │   └── codex/
│   │       ├── session.go         # JSONL session file parser + discovery
│   │       └── config.go          # Codex CLI config parser
│   ├── estimator/
│   │   └── estimator.go           # Context window composition calculator
│   ├── renderer/
│   │   ├── terminal.go            # ANSI stacked bar, table, diagrams (962 lines)
│   │   └── html.go                # Minimal HTML template stub
│   └── server/
│       └── server.go              # HTTP server, /api/* endpoints, dashboard
├── Makefile                       # Build targets: build, test, test-race, vet, lint
└── go.mod                         # Module declaration (no dependencies)
```

---

## 2. Core Runtime Behavior

### 2.1 CLI Argument Parsing

**No external flag library.** Manual parsing in `main.go` handles positional ambiguity:
- `-mode claude|codex` — Force tool detection
- `-watch N` — Re-render every N seconds (file polling)
- `-serve PORT` — Start HTTP server on PORT, open browser
- `-pct FLOAT` — Simulation mode (override token calculations)
- `-session UUID` — Specific session to inspect
- `-project PATH` — Override project directory detection
- `-table`, `-order`, `-agents`, `-timeline`, `-diff` — Render modes
- `-statusline`, `-statusline-full` — Compact terminal output
- `-json` — Machine-readable JSON output
- `-nocolor` — Strip ANSI codes
- `-help`, `-version` — Print help or version string

### 2.2 JSONL Streaming

**File I/O strategy:**
- `bufio.Scanner` with 10MB buffer (handles large session files)
- One JSON record per line: `{ "type": "user|assistant|summary|compact_boundary", ... }`
- Streaming decoder (`json.Decoder`) processes records sequentially
- Support for interleaved user/assistant messages, thinking blocks, tool calls
- Graceful handling of truncated/corrupt records

**Record envelope (both Claude and Codex):**
```
{
  "type": "user|assistant|summary|compact_boundary",
  "uuid": "...",
  "timestamp": "2026-03-19T...",
  "message": { ... },
  "usage": { "input_tokens": N, "output_tokens": N, ... }
}
```

### 2.3 Configuration and Discovery

**Claude Code:**
- Session dir: `~/.claude/projects/`
- Config file: `~/.claude/config.json`
- Project encoding: `/Users/foo/bar` → `-Users-foo-bar`
- Usage API: `~/.claude/oauth_cache.json` for token metrics

**Codex CLI:**
- Session dir: `~/.codex/sessions/`
- Config file: `~/.codex/config.toml`
- Minimal TOML parser in `internal/parser/toml/`

---

## 3. Model Registry

**40+ entries** in `internal/model/registry.go`:

| Family | Models | Context Window |
|--------|--------|---|
| Claude | Claude 3, 3.5, 4, Opus, Sonnet, Haiku (13 entries) | 200K–1M |
| Codex/GPT | GPT-4o, 4.1, 5.2, 5.3, 5.4, o3, o4 (12+ entries) | 128K–256K |

**Model lookup:** Case-insensitive, supports `/fast` suffix and `[1m]` variants.

---

## 4. Context Components

### 4.1 Claude Code (13 components)

| Key | Label | Color | Fixed | Tokens |
|-----|-------|-------|-------|--------|
| `system` | System prompt | #534AB7 | ✓ | 3,200 |
| `tools` | Built-in tools | #0F6E56 | ✓ | 17,000 |
| `mcp` | MCP tools | #D85A30 | | 0–N |
| `agents` | Agents | #378ADD | | 0–N |
| `memory` | Memory (CLAUDE.md) | #D4537E | | 0–N |
| `skill_meta` | Skill frontmatter | #3B6D11 | | 0–N |
| `skill_body` | Skill body (active) | #97C459 | | 0–N |
| `plan` | Plan / todo | #1D9E75 | | 0–N |
| `user_msg` | User messages | #BA7517 | | 0–N |
| `tool_results` | Tool call results | #EF9F27 | | 0–N |
| `responses` | AI responses | #854F0B | | 0–N |
| `subagent` | Subagent summaries | #85B7EB | | 0–N |
| `free_space` | Free space | #F0F0F0 | | Remainder |

### 4.2 Codex CLI (12 components)

Similar layout with OpenAI-specific components (functions, context, reasoning tokens, etc.).

---

## 5. Estimator Engine

**File:** `internal/estimator/estimator.go`

**Algorithm:**
1. Parse session JSONL: extract messages, tool calls, token usage
2. Build component array from component definitions
3. Sum tokens per component from JSONL events
4. Calculate percentages: `(tokens / context_window) * 100`
5. Append free space: `max(0, context_window - total_tokens)`
6. Return composition with model metadata

**Token sources:**
- `usage.input_tokens`, `usage.cache_read_input_tokens`, `usage.cache_creation_input_tokens`
- Per-message heuristics: system prompt, tool definitions, user content length
- Thinking tokens (for reasoning models)

---

## 6. Renderers

### 6.1 Terminal (ANSI)

**File:** `internal/renderer/terminal.go` (962 lines)

**Outputs:**
- **Stacked bar:** 80-column ANSI colored bar with component segments
- **Table:** Per-component breakdown with token count and percentage
- **Order diagram:** Visual sequence of component loading (for diagnostics)
- **Timeline:** Token growth over session lifetime
- **Statusline:** Single-line compact format for Claude Code integration

**Colors:** ANSI 256-color codes mapped from hex definitions in `model/colors.go`

### 6.2 HTML Dashboard

**File:** `internal/renderer/html.go`

**Strategy:**
- `//go:embed` directive compiles static HTML into binary
- Single-file SPA (no external assets)
- API endpoints served by HTTP server

**Endpoints:**
- `GET /` — Serve dashboard HTML
- `GET /api/data` — Current composition JSON
- `GET /api/sessions` — List discovered sessions
- `GET /api/session/:id` — Session metadata
- `GET /api/timeline/:id` — Token growth time series

---

## 7. HTTP Server

**File:** `internal/server/server.go`

**Features:**
- Go 1.22+ routing (no third-party router)
- JSON API for composition data
- CORS headers for browser access
- Graceful shutdown via `context.Context`
- Fallback HTML if embedded template not found

**Lifecycle:**
1. Parse `-serve PORT` flag
2. Start server on `localhost:PORT`
3. Open browser to `http://localhost:PORT`
4. Serve `/api/*` and `/` (dashboard)
5. Reload on file changes (if `-watch` enabled)

---

## 8. Watch Mode

**Implementation:**
- `time.Ticker` for periodic polling (interval from `-watch N`)
- Goroutines with `context.Context` for cancellation
- Signal handler: `os.signal.NotifyContext` for SIGINT/SIGTERM
- Re-parse session JSONL on each tick, re-render if changed

---

## 9. CLI Entry Point

**File:** `cmd/ctx-monitor/main.go` (685 lines)

**Flow:**
1. Parse CLI arguments (manual flag parsing)
2. Auto-detect Claude or Codex (or use `-mode`)
3. Find session directory and config
4. Load JSONL session file (streaming)
5. Call estimator with session data
6. Render output (terminal, JSON, or HTTP server)
7. If `-watch`, loop with ticker
8. If `-serve`, start HTTP server and block

---

## 10. Build and Distribution

**Makefile targets:**

| Target | Purpose |
|--------|---------|
| `make build` | Compile `ctx-monitor` binary for current OS |
| `make test` | Run unit tests |
| `make test-race` | Race detector (find concurrent access bugs) |
| `make vet` | Go vet (static analysis) |
| `make install` | Install to `$GOBIN` or `$HOME/go/bin` |
| `make cross` | Build for macOS (amd64, arm64) + Linux (amd64, arm64) |
| `make clean` | Remove binaries |

**Version injection:**
```makefile
LDFLAGS := -ldflags "-X main.version=$(VERSION)"
```

**Binary size target:** < 15MB (statically linked, includes embedded HTML)

---

## 11. Zero Dependencies

**Allowed imports:** Only Go standard library
- `bufio`, `encoding/json`, `os`, `io`
- `net/http` for server
- `time` for watch mode
- `context` for cancellation
- `sync` for concurrency

**No external modules.** All custom code in `internal/` packages.

---

## 12. Error Handling

**Strategy:**
- Graceful degradation: missing config → continue with defaults
- Invalid JSONL records → skip and continue
- File not found → clear error message, exit code 1
- HTTP errors → 5xx with JSON error envelope
- Parse errors → detailed logging, suggest fixes

---

## 13. Testing Strategy

See `TEST.md` for full test plan, including:
- Unit tests per package
- Integration tests (full pipeline)
- Benchmarks (50MB JSONL < 2s)
- Race detection
- Binary size verification
