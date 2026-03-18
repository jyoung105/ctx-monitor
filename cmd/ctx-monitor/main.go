package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
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
		session, err := claudeparser.ParseSession(filePath)
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

	session, err := claudeparser.ParseSession(latest.FilePath)
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
			session, err := codexparser.ParseSession(args.session)
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

	session, err := codexparser.ParseSession(latest.Path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Parse error: %s\n", err)
		os.Exit(3)
	}
	return session, projectPath
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
		config := claudeparser.ParseConfig(projectPath)

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
		config, _ := codexparser.ParseConfig(projectPath)

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
				session, err := claudeparser.ParseSession(filePath)
				if err != nil {
					return nil, nil
				}
				projectPath := args.project
				if projectPath == "" {
					projectPath, _ = os.Getwd()
				}
				config := claudeparser.ParseConfig(projectPath)
				return estimator.EstimateClaudeContext(session, config, nil), nil
			}
			sessions, _ := codexparser.FindAllSessions()
			for _, s := range sessions {
				if strings.Contains(s.Name, id) {
					session, err := codexparser.ParseSession(s.Path)
					if err != nil {
						return nil, nil
					}
					projectPath := args.project
					if projectPath == "" {
						projectPath, _ = os.Getwd()
					}
					config, _ := codexparser.ParseConfig(projectPath)
					return estimator.EstimateCodexContext(session, config), nil
				}
			}
			return nil, nil
		},
		GetTimeline: func(id string) (interface{}, error) {
			var messages []model.Message
			if mode == "claude" {
				_, sessionDir := resolveProjectDir(args)
				if sessionDir == "" {
					return nil, nil
				}
				filePath := filepath.Join(sessionDir, id+".jsonl")
				session, err := claudeparser.ParseSession(filePath)
				if err != nil {
					return nil, nil
				}
				messages = session.Messages
			}

			if len(messages) == 0 {
				return nil, nil
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
			}, nil
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
