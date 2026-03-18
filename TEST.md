# ctx-monitor — Test Strategy

## Overview

This document describes the testing approach for ctx-monitor (Go rewrite). Tests are organized by package with clear test cases and acceptance criteria.

---

## 1. Unit Tests by Package

### 1.1 `internal/model`

**File:** `internal/model/*_test.go`

| Test | Purpose | Acceptance |
|------|---------|-----------|
| `TestComponentDef` | Claude/Codex component definitions load correctly | 13 Claude + 12 Codex components present, colors set |
| `TestColorMapping` | ANSI/hex color pairs match | All 25 colors have ANSI code + hex value |
| `TestModelRegistry` | Model lookup resolves correctly | 40+ entries, `/fast` suffix handling, `[1m]` variants |
| `TestContextWindow` | Model context window sizes correct | Claude 200K–1M, Codex 128K–256K |
| `TestCompositionStruct` | Session and component types serialize/deserialize | JSON roundtrip successful |

### 1.2 `internal/parser/claude`

**File:** `internal/parser/claude/*_test.go`

| Test | Purpose | Acceptance |
|------|---------|-----------|
| `TestParseSessionJSONL` | Stream JSONL records correctly | 100 records parsed, token counts correct |
| `TestFindProjectDir` | Locate Claude project directory | Dash-encoded path matching works |
| `TestSessionDiscovery` | List local Claude sessions | Sessions sorted by mtime, IDs extracted |
| `TestParseConfig` | Load `~/.claude/config.json` | User ID, selected project, version extracted |
| `TestUsageAPI` | Fetch OAuth token metrics | Token counts match expected values |
| `TestHandleCompactBoundary` | Parse compaction events | Boundary markers detected, token count preserved |

### 1.3 `internal/parser/codex`

**File:** `internal/parser/codex/*_test.go`

| Test | Purpose | Acceptance |
|------|---------|-----------|
| `TestParseSessionJSONL` | Stream Codex JSONL records | Records parsed, model name extracted |
| `TestSessionDiscovery` | List local Codex sessions | Sessions found, sorted, metadata valid |
| `TestParseTOML` | Minimal TOML parser | Key-value pairs extracted, nested tables supported |
| `TestParseConfig` | Load `~/.codex/config.toml` | API key, model, parameters extracted |

### 1.4 `internal/estimator`

**File:** `internal/estimator/*_test.go`

| Test | Purpose | Acceptance |
|------|---------|-----------|
| `TestEstimateComposition` | Calculate context breakdown | Components sum to context window (±1%) |
| `TestTokenAggregation` | Accumulate tokens per component | Usage fields summed correctly |
| `TestPercentageCalculation` | Round percentages to 2 decimals | Totals = 100% (±0.01) |
| `TestFreeSpace` | Compute remaining tokens | Free space = context_window - used |
| `TestNilSafety` | Handle nil/empty usage fields | No panics, graceful defaults |

### 1.5 `internal/renderer/terminal`

**File:** `internal/renderer/terminal_test.go`

| Test | Purpose | Acceptance |
|------|---------|-----------|
| `TestStackedBar` | Render 80-column colored bar | Bar width ≤ 80 chars, segment widths proportional |
| `TestTableRenderer` | Per-component table output | All components listed, tokens + % shown |
| `TestOrderDiagram` | Component loading sequence | Sequence correct, fixed components listed first |
| `TestTimelineRenderer` | Token growth over time | X-axis timestamps, Y-axis token counts |
| `TestAnsiColor` | Map hex to ANSI 256 | Color codes in valid range (0–255) |
| `TestStatusline` | Compact single-line format | Fits in 80 chars, readable tokens + bar |
| `TestNoColor` | Strip ANSI codes when requested | Output contains no ANSI escape sequences |

### 1.6 `internal/server`

**File:** `internal/server/*_test.go`

| Test | Purpose | Acceptance |
|------|---------|-----------|
| `TestAPIEndpoints` | HTTP endpoints respond | `/api/data`, `/api/sessions`, `/api/session/:id` all 200 OK |
| `TestJSONFormatting` | Response JSON is valid | `json.Unmarshal` succeeds, proper indentation |
| `TestErrorHandling` | 4xx/5xx responses proper | 404 for missing session, 500 for errors, JSON error envelope |
| `TestCORS` | Browser access allowed | `Access-Control-Allow-Origin` header set |
| `TestGracefulShutdown` | Server closes cleanly | No goroutine leaks, connections drained |

---

## 2. Integration Tests

**File:** `internal/integration_test.go`

### 2.1 Full Pipeline Tests

| Scenario | Steps | Acceptance |
|----------|-------|-----------|
| **Claude session analysis** | (1) Load sample JSONL, (2) parse, (3) estimate, (4) render | Output shows 13 components, token counts match |
| **Codex session analysis** | (1) Load sample JSONL, (2) parse, (3) estimate, (4) render | Output shows 12 components, model resolved |
| **Watch mode** | (1) Start watch, (2) modify session file, (3) re-render | Output updates within 1 sec, no stale data |
| **HTTP server** | (1) Start server, (2) fetch `/api/data`, (3) verify JSON | Response 200 OK, all fields present |
| **CLI argument parsing** | (1) Parse mixed flags (mode, watch, serve, render), (2) validate state | No conflicts, defaults applied correctly |

### 2.2 Error Path Tests

| Scenario | Input | Expected Outcome |
|----------|-------|------------------|
| **Missing session file** | Non-existent path | Clear error message, exit code 1 |
| **Corrupted JSONL** | Truncated/invalid JSON | Skip bad record, continue parsing, warning logged |
| **Invalid model name** | Unknown model ID | Fallback to default context window (200K) |
| **Invalid port** | `-serve 99999` | Clear error, exit code 1 |
| **Permission denied** | Read-only config file | Graceful fallback, no crash |

---

## 3. Benchmark Suite

**File:** `internal/benchmark_test.go`

### 3.1 Performance Targets

| Benchmark | Input | Target | Acceptance |
|-----------|-------|--------|-----------|
| `BenchmarkParseJSONL` | 50MB JSONL (100K+ records) | < 2 sec | Complete parse, no timeouts |
| `BenchmarkEstimate` | Parsed session | < 50 ms | Composition calculated quickly |
| `BenchmarkRenderTerminal` | 13-component composition | < 10 ms | Bar + table rendered fast |
| `BenchmarkRenderHTML` | Same composition | < 50 ms | HTML generated quickly |
| `BenchmarkSessionDiscovery` | 10+ local sessions | < 100 ms | All sessions found and sorted |

### 3.2 Running Benchmarks

```bash
go test -bench=. -benchmem ./internal/...
```

---

## 4. Code Quality Verification

### 4.1 Static Analysis

```bash
go vet ./...
```

**Acceptance:** No warnings or errors

### 4.2 Race Detection

```bash
go test -race ./...
```

**Acceptance:** No race conditions detected (all goroutines properly synchronized)

### 4.3 Test Coverage

```bash
go test -cover ./...
```

**Target:** > 75% line coverage
**Critical packages (≥ 85%):** `estimator`, `parser/*`, `renderer/*`

### 4.4 Binary Verification

```bash
make build
ls -lh ctx-monitor
```

**Acceptance:**
- Binary exists
- Size < 15 MB
- Executable permissions set
- Runs: `./ctx-monitor --version`

---

## 5. Test Fixtures

### 5.1 Sample Data Files

**Location:** `fixtures/`

| File | Size | Purpose |
|------|------|---------|
| `claude-session-small.jsonl` | 1 MB | Quick unit tests |
| `claude-session-medium.jsonl` | 10 MB | Integration tests |
| `claude-session-large.jsonl` | 50 MB | Benchmark target |
| `codex-session-small.jsonl` | 500 KB | Unit tests |
| `claude-config.json` | 2 KB | Config parser tests |
| `codex-config.toml` | 1 KB | TOML parser tests |

### 5.2 Fixture Generation

**Script:** `fixtures/generate.sh` (if needed)

```bash
# Generate large JSONL with N records
./fixtures/generate.sh claude 100000 > fixtures/claude-session-large.jsonl
```

---

## 6. Test Execution Checklist

### Before Committing

```bash
# Unit tests
go test ./...

# Race detection
go test -race ./...

# Static analysis
go vet ./...

# Coverage
go test -cover ./...

# Benchmarks (optional)
go test -bench=. -benchmem ./internal/...

# Build
make build

# Manual smoke test
./ctx-monitor --help
./ctx-monitor --json | jq .
./ctx-monitor --serve 8000 &
sleep 2 && curl http://localhost:8000/api/data | jq . && kill %1
```

### CI/CD Integration

All tests run on every push:
1. `go test ./...`
2. `go test -race ./...`
3. `go vet ./...`
4. `make cross` (all platform builds)
5. Binary size check (must be < 15 MB)

---

## 7. Known Edge Cases

| Edge Case | Handling | Test |
|-----------|----------|------|
| Empty session (0 records) | Return default composition | `TestEmptySession` |
| Single message (just user input) | Estimate with minimal tokens | `TestSingleMessage` |
| Huge context window (4M tokens) | Percentage precision maintained | `TestLargeContext` |
| Timestamps out of order | Sort before timeline rendering | `TestOutOfOrderTimestamps` |
| Concurrent file access (watch mode) | Use file.Stat() mtime, not fd locks | `TestConcurrentWatch` |

---

## 8. Regression Tests

**Purpose:** Prevent reintroduction of past bugs

| Issue | Test | Acceptance |
|-------|------|-----------|
| Path encoding (dash vs. slash) | `TestDashEncodedProjectPath` | Both formats detected correctly |
| Token aggregation double-counting | `TestNoDoubleCount` | Sum matches expected, no overflow |
| ANSI color codes in JSON output | `TestJSONNoColor` | No escape sequences in JSON |
| Watch mode file handle leaks | `TestWatchFileHandles` | Open file count stable over time |

---

## 9. Acceptance Criteria (Final Verification)

- [x] All unit tests pass
- [x] All integration tests pass
- [x] Race detector passes (no data races)
- [x] `go vet` passes (no linting issues)
- [x] Benchmarks show < 2s for 50MB JSONL
- [x] Binary size < 15 MB
- [x] CLI runs without errors: `--help`, `--json`, `--serve`, `--watch`
- [x] Coverage > 75% (critical packages > 85%)
- [x] No hardcoded paths or environment assumptions
- [x] Graceful error handling (no panics in normal use)
