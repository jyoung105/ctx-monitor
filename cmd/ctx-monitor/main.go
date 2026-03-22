package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"hash"
	"hash/fnv"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/tonylee/ctx-monitor/internal/estimator"
	"github.com/tonylee/ctx-monitor/internal/model"
	claudeparser "github.com/tonylee/ctx-monitor/internal/parser/claude"
	codexparser "github.com/tonylee/ctx-monitor/internal/parser/codex"
	"github.com/tonylee/ctx-monitor/internal/renderer"
	"github.com/tonylee/ctx-monitor/internal/server"
)

var version = "dev"

type sessionCacheKey struct {
	path     string
	size     int64
	mtimeNS  int64
	detailed bool
}

type codexConfigCacheKey struct {
	projectPath string
	fingerprint uint64
}

type claudeConfigCacheEntry struct {
	config    *model.ClaudeConfig
	expiresAt time.Time
}

type timelineCacheKey struct {
	path    string
	size    int64
	mtimeNS int64
}

var (
	claudeSessionCacheMu sync.Mutex
	claudeSessionCache   = map[sessionCacheKey]*model.ClaudeSession{}
	codexSessionCacheMu  sync.Mutex
	codexSessionCache    = map[sessionCacheKey]*model.CodexSession{}
	claudeConfigCacheMu  sync.Mutex
	claudeConfigCache    = map[string]claudeConfigCacheEntry{}
	claudeConfigCacheTTL = time.Second
	claudeConfigNow      = time.Now
	timelineCacheMu      sync.Mutex
	timelineCache        = map[timelineCacheKey]interface{}{}
	codexConfigCacheMu   sync.Mutex
	codexConfigCache     = map[codexConfigCacheKey]*model.CodexConfig{}
)

type cachedCodexSession struct {
	key     sessionCacheKey
	session *model.CodexSession
}

// ---------------------------------------------------------------------------
// Argument parsing
// ---------------------------------------------------------------------------

type cliArgs struct {
	mode    string // "claude", "codex", or "" (auto-detect)
	watch   int    // -1 = no watch, >=0 = seconds
	pct     float64
	hasPct  bool
	session string
	project string
	serve   int // -1 = no serve, >=0 = port

	table    bool
	order    bool
	agents   bool
	setup    bool
	timeline bool
	diff     bool

	statusline     bool
	statuslineFull bool

	jsonOut bool
	noColor bool
	compact bool
	help    bool
	ver     bool
}

func parseArgs(argv []string) cliArgs {
	args := cliArgs{watch: -1, serve: -1, pct: -1}
	i := 0
	for i < len(argv) {
		arg := argv[i]
		switch arg {
		case "--claude", "-c":
			args.mode = "claude"
		case "--codex", "-x":
			args.mode = "codex"
		case "--watch", "-w":
			next := ""
			if i+1 < len(argv) {
				next = argv[i+1]
			}
			if next != "" && !strings.HasPrefix(next, "-") {
				if n, err := strconv.Atoi(next); err == nil {
					args.watch = n
					i++
				} else {
					args.watch = 5
				}
			} else {
				args.watch = 5
			}
		case "--pct", "-p":
			if i+1 >= len(argv) {
				fmt.Fprintln(os.Stderr, "Error: --pct requires a value")
				os.Exit(2)
			}
			next := argv[i+1]
			n, err := strconv.ParseFloat(next, 64)
			if err != nil || n < 0 || n > 100 {
				fmt.Fprintln(os.Stderr, "Error: --pct requires a number between 0 and 100")
				os.Exit(2)
			}
			args.pct = n
			args.hasPct = true
			i++
		case "--session":
			if i+1 >= len(argv) || strings.HasPrefix(argv[i+1], "-") {
				fmt.Fprintln(os.Stderr, "Error: --session requires a session ID")
				os.Exit(2)
			}
			args.session = argv[i+1]
			i++
		case "--project":
			if i+1 >= len(argv) || strings.HasPrefix(argv[i+1], "-") {
				fmt.Fprintln(os.Stderr, "Error: --project requires a path")
				os.Exit(2)
			}
			args.project = argv[i+1]
			i++
		case "--serve":
			next := ""
			if i+1 < len(argv) {
				next = argv[i+1]
			}
			if next != "" && !strings.HasPrefix(next, "-") {
				if n, err := strconv.Atoi(next); err == nil {
					args.serve = n
					i++
				} else {
					args.serve = 3456
				}
			} else {
				args.serve = 3456
			}
		case "--table", "-t":
			args.table = true
		case "--order", "-o":
			args.order = true
		case "--agents", "-a":
			args.agents = true
		case "--setup", "-s":
			args.setup = true
		case "--timeline":
			args.timeline = true
		case "--diff":
			args.diff = true
		case "--statusline":
			args.statusline = true
		case "--statusline-full":
			args.statuslineFull = true
		case "--json":
			args.jsonOut = true
		case "--no-color":
			args.noColor = true
		case "--compact":
			args.compact = true
		case "--help", "-h":
			args.help = true
		case "--version", "-v":
			args.ver = true
		default:
			if strings.HasPrefix(arg, "-") {
				fmt.Fprintf(os.Stderr, "Unknown option: %s\n", arg)
				os.Exit(2)
			}
		}
		i++
	}
	return args
}

// ---------------------------------------------------------------------------
// Help text
// ---------------------------------------------------------------------------

const helpText = `ctx-monitor — Context Composition Monitor for Claude Code & Codex CLI

Usage: ctx-monitor [options]

Options:
  --claude, -c          Force Claude Code mode
  --codex, -x           Force Codex CLI mode
  --watch [N], -w [N]   Re-render every N seconds (default: 5)
  --pct <N>, -p <N>     Simulate N%% context usage
  --session <id>        Target specific session by UUID
  --project <path>      Target specific project directory
  --serve [port]        Start HTTP server with browser dashboard (default: 3456)

Views:
  --table, -t           Component breakdown table only
  --order, -o           Context loading order diagram
  --agents, -a          Subagent/team isolation diagram
  --setup, -s           Setup instructions
  --timeline            Show context growth timeline
  --diff                Compare two sessions side-by-side

Statusline integration:
  --statusline          Read Claude Code JSON from stdin, output compact bar
  --statusline-full     Read stdin, output multi-line component breakdown

Output:
  --json                Output structured JSON
  --no-color            Disable ANSI colors
  --compact             Single-line output mode
  --help, -h            Show help
  --version, -v         Show version

Exit codes:
  0  Success
  1  No session found
  2  Invalid arguments
  3  Parse error
`

// ---------------------------------------------------------------------------
// Mode detection
// ---------------------------------------------------------------------------

func detectMode() string {
	home, _ := os.UserHomeDir()
	if home == "" {
		return "claude"
	}
	if info, err := os.Stat(filepath.Join(home, ".claude")); err == nil && info.IsDir() {
		return "claude"
	}
	if info, err := os.Stat(filepath.Join(home, ".codex")); err == nil && info.IsDir() {
		return "codex"
	}
	return "claude"
}

// ---------------------------------------------------------------------------
// Session discovery
// ---------------------------------------------------------------------------

func resolveProjectDir(args cliArgs) (projectPath, sessionDir string) {
	if args.project != "" {
		dir := claudeparser.FindProjectDir(args.project)
		return args.project, dir
	}
	cwd, _ := os.Getwd()
	dir := claudeparser.FindProjectDir(cwd)
	return cwd, dir
}

func findAndParseClaudeSession(args cliArgs) (*model.ClaudeSession, string) {
	projectPath, sessionDir := resolveProjectDir(args)
	if sessionDir == "" {
		return nil, projectPath
	}

	if args.session != "" {
		filePath := filepath.Join(sessionDir, args.session+".jsonl")
		if _, err := os.Stat(filePath); err != nil {
			fmt.Fprintf(os.Stderr, "Session not found: %s\n", args.session)
			return nil, projectPath
		}
		session, err := parseClaudeSessionForArgs(filePath, args)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Parse error: %s\n", err)
			os.Exit(3)
		}
		return session, projectPath
	}

	latest := claudeparser.FindLatestSession(sessionDir)
	if latest == nil {
		return nil, projectPath
	}

	session, err := parseClaudeSessionForArgs(latest.FilePath, args)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Parse error: %s\n", err)
		os.Exit(3)
	}
	return session, projectPath
}

func findAndParseCodexSession(args cliArgs) (*model.CodexSession, string) {
	projectPath := args.project
	if projectPath == "" {
		projectPath, _ = os.Getwd()
	}

	if args.session != "" {
		if _, err := os.Stat(args.session); err == nil {
			session, err := parseCodexSessionForArgs(args.session, args)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Parse error: %s\n", err)
				os.Exit(3)
			}
			return session, projectPath
		}
		fmt.Fprintf(os.Stderr, "Session not found: %s\n", args.session)
		return nil, projectPath
	}

	latest, _ := codexparser.FindLatestSession()
	if latest == nil {
		return nil, projectPath
	}

	session, err := parseCodexSessionForArgs(latest.Path, args)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Parse error: %s\n", err)
		os.Exit(3)
	}
	return session, projectPath
}

func needsDetailedSession(args cliArgs) bool {
	return args.jsonOut
}

func makeSessionCacheKey(filePath string, detailed bool) (sessionCacheKey, error) {
	info, err := os.Stat(filePath)
	if err != nil {
		return sessionCacheKey{}, err
	}
	return sessionCacheKey{
		path:     filePath,
		size:     info.Size(),
		mtimeNS:  info.ModTime().UnixNano(),
		detailed: detailed,
	}, nil
}

func parseClaudeSession(filePath string, detailed bool) (*model.ClaudeSession, error) {
	key, err := makeSessionCacheKey(filePath, detailed)
	if err != nil {
		return nil, err
	}

	claudeSessionCacheMu.Lock()
	cached := claudeSessionCache[key]
	claudeSessionCacheMu.Unlock()
	if cached != nil {
		return cached, nil
	}

	if detailed {
		session, err := claudeparser.ParseSession(filePath)
		if err != nil {
			return nil, err
		}
		claudeSessionCacheMu.Lock()
		claudeSessionCache[key] = session
		claudeSessionCacheMu.Unlock()
		return session, nil
	}
	session, err := claudeparser.ParseSessionSummary(filePath)
	if err != nil {
		return nil, err
	}
	claudeSessionCacheMu.Lock()
	claudeSessionCache[key] = session
	claudeSessionCacheMu.Unlock()
	return session, nil
}

func parseClaudeSessionForArgs(filePath string, args cliArgs) (*model.ClaudeSession, error) {
	return parseClaudeSession(filePath, needsDetailedSession(args))
}

func findCachedCodexSessionForAppend(path string, detailed bool, newSize int64) *cachedCodexSession {
	codexSessionCacheMu.Lock()
	defer codexSessionCacheMu.Unlock()

	var best *cachedCodexSession
	for key, session := range codexSessionCache {
		if key.path != path || key.detailed != detailed {
			continue
		}
		if key.size >= newSize {
			continue
		}
		if best == nil || key.size > best.key.size {
			best = &cachedCodexSession{key: key, session: session}
		}
	}
	return best
}

func cloneCodexSession(src *model.CodexSession) *model.CodexSession {
	if src == nil {
		return nil
	}
	cloned := *src
	cloned.ToolCalls = append([]model.CodexToolCall(nil), src.ToolCalls...)
	cloned.ToolResults = append([]model.ToolResult(nil), src.ToolResults...)
	cloned.CompactionEvents = append([]model.CompactionEvent(nil), src.CompactionEvents...)
	cloned.SubagentSpawns = append([]model.CodexToolCall(nil), src.SubagentSpawns...)
	cloned.PlanUsage = append([]model.CodexToolCall(nil), src.PlanUsage...)
	cloned.Turns = append([]interface{}(nil), src.Turns...)
	return &cloned
}

func mergeCodexSessionDelta(base, delta *model.CodexSession) *model.CodexSession {
	if base == nil {
		return delta
	}
	if delta == nil {
		return base
	}

	merged := cloneCodexSession(base)
	if delta.SessionID != "" {
		merged.SessionID = delta.SessionID
	}
	if delta.Model != "" {
		merged.Model = delta.Model
	}
	if delta.ContextWindowSize > 0 {
		merged.ContextWindowSize = delta.ContextWindowSize
	}
	if delta.ReasoningEffort != "" {
		merged.ReasoningEffort = delta.ReasoningEffort
	}

	merged.TokenUsage.Total += delta.TokenUsage.Total
	merged.TokenUsage.Input += delta.TokenUsage.Input
	merged.TokenUsage.Cached += delta.TokenUsage.Cached
	merged.TokenUsage.Output += delta.TokenUsage.Output
	merged.TokenUsage.Reasoning += delta.TokenUsage.Reasoning
	if delta.LastTokenUsage.Total > 0 || delta.LastTokenUsage.Input > 0 || delta.LastTokenUsage.Output > 0 {
		merged.LastTokenUsage = delta.LastTokenUsage
	}

	merged.ToolCalls = append(merged.ToolCalls, delta.ToolCalls...)
	merged.ToolResults = append(merged.ToolResults, delta.ToolResults...)
	merged.CompactionEvents = append(merged.CompactionEvents, delta.CompactionEvents...)
	merged.SubagentSpawns = append(merged.SubagentSpawns, delta.SubagentSpawns...)
	merged.PlanUsage = append(merged.PlanUsage, delta.PlanUsage...)
	merged.Turns = append(merged.Turns, delta.Turns...)

	merged.TokenBuckets.UserMsg += delta.TokenBuckets.UserMsg
	merged.TokenBuckets.ToolResults += delta.TokenBuckets.ToolResults
	merged.TokenBuckets.Responses += delta.TokenBuckets.Responses
	merged.TokenBuckets.Subagent += delta.TokenBuckets.Subagent
	merged.TokenBuckets.SkillBody += delta.TokenBuckets.SkillBody
	merged.TokenBuckets.Plan += delta.TokenBuckets.Plan
	merged.TokenBuckets.Thinking += delta.TokenBuckets.Thinking
	merged.TokenBuckets.Reasoning += delta.TokenBuckets.Reasoning

	merged.RawStats.LineCount += delta.RawStats.LineCount
	merged.RawStats.ParseErrors += delta.RawStats.ParseErrors

	return merged
}

func storeCodexSessionCache(key sessionCacheKey, session *model.CodexSession) {
	codexSessionCacheMu.Lock()
	defer codexSessionCacheMu.Unlock()

	for existingKey := range codexSessionCache {
		if existingKey.path == key.path && existingKey.detailed == key.detailed && existingKey.size < key.size {
			delete(codexSessionCache, existingKey)
		}
	}
	codexSessionCache[key] = session
}

func parseCodexSessionForArgs(filePath string, args cliArgs) (*model.CodexSession, error) {
	detailed := needsDetailedSession(args)
	key, err := makeSessionCacheKey(filePath, detailed)
	if err != nil {
		return nil, err
	}

	codexSessionCacheMu.Lock()
	cached := codexSessionCache[key]
	codexSessionCacheMu.Unlock()
	if cached != nil {
		return cached, nil
	}

	if !detailed {
		if previous := findCachedCodexSessionForAppend(filePath, detailed, key.size); previous != nil && previous.key.mtimeNS <= key.mtimeNS {
			delta, err := codexparser.ParseSessionSummaryFromOffset(filePath, previous.key.size)
			if err == nil {
				session := mergeCodexSessionDelta(previous.session, delta)
				storeCodexSessionCache(key, session)
				return session, nil
			}
		}
	}

	if detailed {
		session, err := codexparser.ParseSession(filePath)
		if err != nil {
			return nil, err
		}
		storeCodexSessionCache(key, session)
		return session, nil
	}
	session, err := codexparser.ParseSessionSummary(filePath)
	if err != nil {
		return nil, err
	}
	storeCodexSessionCache(key, session)
	return session, nil
}

func writePathFingerprint(hasher hash.Hash64, path string) {
	info, err := os.Stat(path)
	if err != nil {
		_, _ = hasher.Write([]byte(path + ":missing;"))
		return
	}
	_, _ = hasher.Write([]byte(fmt.Sprintf("%s:%d:%d;", path, info.Size(), info.ModTime().UnixNano())))
}

func writeSkillDirFingerprint(hasher hash.Hash64, dir string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		_, _ = hasher.Write([]byte(dir + ":missing;"))
		return
	}

	_, _ = hasher.Write([]byte(fmt.Sprintf("%s:entries=%d;", dir, len(entries))))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		skillPath := filepath.Join(dir, entry.Name(), "SKILL.md")
		writePathFingerprint(hasher, skillPath)
	}
}

func codexConfigFingerprint(projectPath string) uint64 {
	hasher := fnv.New64a()

	home, _ := os.UserHomeDir()
	if home != "" {
		writePathFingerprint(hasher, filepath.Join(home, ".codex", "config.toml"))
		writePathFingerprint(hasher, filepath.Join(home, ".codex", "AGENTS.md"))
		writeSkillDirFingerprint(hasher, filepath.Join(home, ".agents", "skills"))
	}

	if projectPath != "" {
		writePathFingerprint(hasher, filepath.Join(projectPath, ".codex", "config.toml"))
		writeSkillDirFingerprint(hasher, filepath.Join(projectPath, ".agents", "skills"))
	}

	return hasher.Sum64()
}

func parseCodexConfigCached(projectPath string) (*model.CodexConfig, error) {
	key := codexConfigCacheKey{
		projectPath: projectPath,
		fingerprint: codexConfigFingerprint(projectPath),
	}

	codexConfigCacheMu.Lock()
	cached := codexConfigCache[key]
	codexConfigCacheMu.Unlock()
	if cached != nil {
		return cached, nil
	}

	cfg, err := codexparser.ParseConfig(projectPath)
	if err != nil {
		return nil, err
	}

	codexConfigCacheMu.Lock()
	codexConfigCache[key] = cfg
	codexConfigCacheMu.Unlock()
	return cfg, nil
}

func parseClaudeConfigCached(projectPath string) *model.ClaudeConfig {
	now := claudeConfigNow()

	claudeConfigCacheMu.Lock()
	cached, ok := claudeConfigCache[projectPath]
	claudeConfigCacheMu.Unlock()
	if ok && now.Before(cached.expiresAt) {
		return cached.config
	}

	cfg := claudeparser.ParseConfig(projectPath)
	claudeConfigCacheMu.Lock()
	claudeConfigCache[projectPath] = claudeConfigCacheEntry{
		config:    cfg,
		expiresAt: now.Add(claudeConfigCacheTTL),
	}
	claudeConfigCacheMu.Unlock()
	return cfg
}

// ---------------------------------------------------------------------------
// Composition builder
// ---------------------------------------------------------------------------

func buildComposition(mode string, args cliArgs) *model.Composition {
	if mode == "claude" {
		session, projectPath := findAndParseClaudeSession(args)
		if session == nil {
			fmt.Fprintln(os.Stderr, "No Claude Code session found.")
			os.Exit(1)
		}
		config := parseClaudeConfigCached(projectPath)

		comp := estimator.EstimateClaudeContext(session, config, nil)

		// Try to fetch plan usage (non-blocking)
		planUsage, _ := claudeparser.FetchPlanUsage()
		if planUsage != nil {
			comp.PlanUsage = planUsage
		}

		if args.hasPct {
			comp = estimator.SimulateUsage(comp, args.pct)
		}
		return comp
	}

	if mode == "codex" {
		session, projectPath := findAndParseCodexSession(args)
		if session == nil {
			fmt.Fprintln(os.Stderr, "No Codex CLI session found.")
			os.Exit(1)
		}
		config, _ := parseCodexConfigCached(projectPath)

		comp := estimator.EstimateCodexContext(session, config)

		if args.hasPct {
			comp = estimator.SimulateUsage(comp, args.pct)
		}
		return comp
	}

	fmt.Fprintf(os.Stderr, "Unknown mode: %s\n", mode)
	os.Exit(2)
	return nil
}

// ---------------------------------------------------------------------------
// Render dispatch
// ---------------------------------------------------------------------------

func render(comp *model.Composition, args cliArgs) {
	opts := renderer.RenderOpts{NoColor: args.noColor}

	var output string
	switch {
	case args.table:
		output = renderer.RenderTable(comp, opts)
	case args.order:
		output = renderer.RenderOrder(comp, opts)
	case args.agents:
		output = renderer.RenderAgents(comp, opts)
	case args.timeline:
		output = renderer.RenderTimeline(comp, opts)
	case args.compact:
		output = renderer.RenderCompact(comp, opts)
	default:
		output = renderer.RenderFull(comp, opts)
	}
	fmt.Println(output)
}

func buildTimelineData(id string, messages []model.Message) interface{} {
	if len(messages) == 0 {
		return nil
	}

	runningTotal := 0
	events := make([]map[string]interface{}, 0, len(messages))
	for i, msg := range messages {
		runningTotal += msg.TokenEstimate
		events = append(events, map[string]interface{}{
			"index":            i,
			"role":             msg.Role,
			"timestamp":        msg.Timestamp,
			"tokens":           msg.TokenEstimate,
			"cumulativeTokens": runningTotal,
			"toolCalls":        msg.ToolCallCount,
		})
	}

	return map[string]interface{}{
		"sessionId":   id,
		"eventCount":  len(events),
		"totalTokens": runningTotal,
		"events":      events,
	}
}

func makeTimelineCacheKey(filePath string) (timelineCacheKey, error) {
	info, err := os.Stat(filePath)
	if err != nil {
		return timelineCacheKey{}, err
	}
	return timelineCacheKey{
		path:    filePath,
		size:    info.Size(),
		mtimeNS: info.ModTime().UnixNano(),
	}, nil
}

func buildClaudeTimelineCached(filePath, id string) (interface{}, error) {
	key, err := makeTimelineCacheKey(filePath)
	if err != nil {
		return nil, err
	}

	timelineCacheMu.Lock()
	cached := timelineCache[key]
	timelineCacheMu.Unlock()
	if cached != nil {
		return cached, nil
	}

	session, err := claudeparser.ParseSessionTimeline(filePath)
	if err != nil {
		return nil, err
	}
	data := buildTimelineData(id, session.Messages)

	timelineCacheMu.Lock()
	timelineCache[key] = data
	timelineCacheMu.Unlock()
	return data, nil
}

// ---------------------------------------------------------------------------
// Watch mode
// ---------------------------------------------------------------------------

func startWatchMode(ctx context.Context, mode string, args cliArgs) {
	interval := time.Duration(args.watch) * time.Second
	if interval <= 0 {
		interval = 5 * time.Second
	}

	var lastMtime time.Time

	tick := func() {
		var sessionFilePath string

		if mode == "claude" {
			_, sessionDir := resolveProjectDir(args)
			if sessionDir == "" {
				return
			}
			if args.session != "" {
				sessionFilePath = filepath.Join(sessionDir, args.session+".jsonl")
			} else {
				latest := claudeparser.FindLatestSession(sessionDir)
				if latest != nil {
					sessionFilePath = latest.FilePath
				}
			}
		} else if mode == "codex" {
			if args.session != "" {
				if _, err := os.Stat(args.session); err == nil {
					sessionFilePath = args.session
				}
			} else {
				latest, _ := codexparser.FindLatestSession()
				if latest != nil {
					sessionFilePath = latest.Path
				}
			}
		}

		if sessionFilePath == "" {
			return
		}

		info, err := os.Stat(sessionFilePath)
		if err != nil {
			return
		}

		if !info.ModTime().After(lastMtime) {
			return
		}
		lastMtime = info.ModTime()

		comp := buildComposition(mode, args)
		// Clear screen
		fmt.Print("\x1b[2J\x1b[H")
		render(comp, args)
	}

	// Initial render
	tick()

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			tick()
		}
	}
}

// ---------------------------------------------------------------------------
// Server mode
// ---------------------------------------------------------------------------

func startServeMode(ctx context.Context, mode string, args cliArgs, comp *model.Composition) {
	htmlTemplate := renderer.GetHTMLTemplate()

	opts := server.ServerOpts{
		Port:         args.serve,
		HTMLTemplate: htmlTemplate,
		GetComposition: func() (interface{}, error) {
			c := buildComposition(mode, args)
			if c != nil {
				return c, nil
			}
			return comp, nil
		},
		GetSessions: func() (interface{}, error) {
			if mode == "claude" {
				_, sessionDir := resolveProjectDir(args)
				if sessionDir == "" {
					return []interface{}{}, nil
				}
				sessions := claudeparser.FindAllSessions(sessionDir)
				result := make([]map[string]interface{}, 0, len(sessions))
				for _, s := range sessions {
					result = append(result, map[string]interface{}{
						"id":    s.ID,
						"mtime": s.Mtime.Format(time.RFC3339),
						"size":  s.Size,
					})
				}
				return result, nil
			}
			sessions, _ := codexparser.FindAllSessions()
			result := make([]map[string]interface{}, 0, len(sessions))
			for _, s := range sessions {
				result = append(result, map[string]interface{}{
					"id":    s.Name,
					"path":  s.Path,
					"mtime": s.Mtime.Format(time.RFC3339),
					"size":  s.Size,
				})
			}
			return result, nil
		},
		GetSessionByID: func(id string) (interface{}, error) {
			if mode == "claude" {
				_, sessionDir := resolveProjectDir(args)
				if sessionDir == "" {
					return nil, nil
				}
				filePath := filepath.Join(sessionDir, id+".jsonl")
				if _, err := os.Stat(filePath); err != nil {
					return nil, nil
				}
				session, err := parseClaudeSessionForArgs(filePath, args)
				if err != nil {
					return nil, nil
				}
				projectPath := args.project
				if projectPath == "" {
					projectPath, _ = os.Getwd()
				}
				config := parseClaudeConfigCached(projectPath)
				return estimator.EstimateClaudeContext(session, config, nil), nil
			}
			sessions, _ := codexparser.FindAllSessions()
			for _, s := range sessions {
				if strings.Contains(s.Name, id) {
					session, err := parseCodexSessionForArgs(s.Path, args)
					if err != nil {
						return nil, nil
					}
					projectPath := args.project
					if projectPath == "" {
						projectPath, _ = os.Getwd()
					}
					config, _ := parseCodexConfigCached(projectPath)
					return estimator.EstimateCodexContext(session, config), nil
				}
			}
			return nil, nil
		},
		GetTimeline: func(id string) (interface{}, error) {
			if mode == "claude" {
				_, sessionDir := resolveProjectDir(args)
				if sessionDir == "" {
					return nil, nil
				}
				filePath := filepath.Join(sessionDir, id+".jsonl")
				data, err := buildClaudeTimelineCached(filePath, id)
				if err != nil {
					return nil, nil
				}
				return data, nil
			}
			return nil, nil
		},
	}

	if err := server.StartServer(opts, ctx); err != nil {
		fmt.Fprintf(os.Stderr, "Server error: %s\n", err)
		os.Exit(1)
	}
}

// ---------------------------------------------------------------------------
// Stdin reader (for --statusline)
// ---------------------------------------------------------------------------

func readStdin() string {
	scanner := bufio.NewScanner(os.Stdin)
	scanner.Buffer(nil, 10<<20)
	var sb strings.Builder
	for scanner.Scan() {
		sb.WriteString(scanner.Text())
		sb.WriteByte('\n')
	}
	return strings.TrimSpace(sb.String())
}

// ---------------------------------------------------------------------------
// Main
// ---------------------------------------------------------------------------

func main() {
	args := parseArgs(os.Args[1:])

	if args.help {
		fmt.Print(helpText)
		os.Exit(0)
	}

	if args.ver {
		fmt.Printf("ctx-monitor %s\n", version)
		os.Exit(0)
	}

	// Set up signal handling
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	mode := args.mode
	if mode == "" {
		mode = detectMode()
	}

	// --statusline: read stdin, compute, render compact bar, exit
	if args.statusline || args.statuslineFull {
		raw := readStdin()
		if raw == "" {
			os.Exit(0)
		}

		var statusline map[string]interface{}
		if err := json.Unmarshal([]byte(raw), &statusline); err != nil {
			fmt.Fprintln(os.Stderr, "Error: Invalid JSON on stdin")
			os.Exit(3)
		}

		comp := estimator.EstimateClaudeContext(nil, nil, statusline)
		if args.hasPct {
			comp = estimator.SimulateUsage(comp, args.pct)
		}

		opts := renderer.RenderOpts{NoColor: args.noColor}
		if args.statuslineFull {
			fmt.Println(renderer.RenderTable(comp, opts))
		} else {
			fmt.Print(renderer.RenderStatusline(comp, opts))
		}
		os.Exit(0)
	}

	// --setup: show setup instructions, exit
	if args.setup {
		opts := renderer.RenderOpts{NoColor: args.noColor}
		fmt.Println(renderer.RenderSetup(mode, opts))
		os.Exit(0)
	}

	// Build composition
	comp := buildComposition(mode, args)

	// --json: output JSON and exit
	if args.jsonOut {
		data, _ := json.MarshalIndent(comp, "", "  ")
		fmt.Println(string(data))
		os.Exit(0)
	}

	// --serve: start HTTP server
	if args.serve >= 0 {
		if args.watch >= 0 {
			fmt.Fprintf(os.Stderr, "Watching for changes every %ds (via API refresh)\n", args.watch)
		}
		startServeMode(ctx, mode, args, comp)
		return
	}

	// --watch: re-render on interval
	if args.watch >= 0 {
		startWatchMode(ctx, mode, args)
		return
	}

	// Single render and exit
	render(comp, args)
	os.Exit(0)
}
