# ctx-monitor — Task List

## Phase 1: Core Infrastructure
- [x] Project scaffolding (package.json, directory structure)
- [x] Color definitions & shared constants (`lib/colors.mjs`)
- [x] Context composition estimator engine (`lib/estimator.mjs`)

## Phase 2: Parsers
- [x] Claude Code JSONL session parser (`lib/parsers/claude-session.mjs`)
- [x] Claude Code config parser (`lib/parsers/claude-config.mjs`)
- [x] Claude Code OAuth usage API client (`lib/parsers/claude-usage-api.mjs`)
- [x] Codex CLI JSONL session parser (`lib/parsers/codex-session.mjs`)
- [x] Codex CLI config parser + minimal TOML parser (`lib/parsers/codex-config.mjs`)

## Phase 3: Renderers
- [x] Terminal ANSI renderer — stacked bar, table, diagrams (`lib/renderer/terminal.mjs`)
- [x] HTML dashboard template — self-contained single-file (`lib/renderer/html-template.mjs`)

## Phase 4: Server & CLI
- [x] HTTP server for `--serve` mode (`lib/server.mjs`)
- [x] Main CLI entry point (`bin/ctx-monitor.mjs`)
- [x] Statusline bridge for Claude Code integration (`bin/statusline-bridge.mjs`)

## Phase 5: Documentation
- [x] README.md
- [x] TASK.md
- [x] Copy SPEC.md and SPEC-v2-addendum.md into project

## Phase 6: Testing & Verification
- [x] Verify `node bin/ctx-monitor.mjs --help` runs without errors
- [x] Verify `node bin/ctx-monitor.mjs --claude` detects local sessions
- [x] Verify `node bin/ctx-monitor.mjs --json` outputs valid JSON
- [x] Verify `node bin/ctx-monitor.mjs --serve` starts HTTP server and serves API + HTML
- [x] Verify `echo '...' | node bin/statusline-bridge.mjs` handles stdin
- [x] Verify `--pct 75` simulation mode works
- [x] Verify `--compact` single-line mode works
- [x] Verify `--table` component breakdown works
- [x] Verify `--order` loading order diagram works
- [x] Fix: terminal renderer `getComponents()` to handle array-format components from estimator
- [x] Fix: `findProjectDir()` to use dash-encoded paths (Claude's actual format)
- [ ] Verify `node bin/ctx-monitor.mjs --codex` detects local sessions
- [ ] Verify `--watch` mode re-renders on file changes
- [ ] Verify HTML dashboard loads in browser and auto-refreshes
- [ ] Verify dark mode works in HTML dashboard
- [ ] Verify responsive layout at 320px and 1920px

## Future Enhancements (v2+)
- [ ] Real /context parsing (when Claude Code exposes per-tool token metrics)
- [ ] Codex multi-agent aggregation
- [ ] WebSocket live streaming (replace polling)
- [ ] Diff mode — compare two sessions side-by-side
- [ ] MCP tool catalog — auto-detect known MCP servers with known token costs
- [ ] Claude Code plugin packaging (`.skill` format for `/ctx-monitor` slash command)
