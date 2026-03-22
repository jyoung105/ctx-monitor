package renderer

import (
	"fmt"
	"math"
	"os"
	"regexp"
	"strings"

	"github.com/tonylee/ctx-monitor/internal/model"
)

// RenderOpts controls rendering options.
type RenderOpts struct {
	NoColor bool
}

// componentInfo holds per-component display data.
type componentInfo struct {
	key    string
	label  string
	tokens int
	pct    float64
	color  string
	ansi   int
	fixed  bool
}

// ---------------------------------------------------------------------------
// Helper functions
// ---------------------------------------------------------------------------

// FormatTokens formats a token count as "3.2k", "104.6k", "1.2M".
func FormatTokens(n int) string {
	if n >= 1_000_000 {
		v := float64(n) / 1_000_000
		if v == math.Trunc(v) {
			return fmt.Sprintf("%dM", int(v))
		}
		return fmt.Sprintf("%.1fM", v)
	}
	if n >= 1000 {
		v := float64(n) / 1000
		if v == math.Trunc(v) {
			return fmt.Sprintf("%dk", int(v))
		}
		return fmt.Sprintf("%.1fk", v)
	}
	return fmt.Sprintf("%d", n)
}

// FormatPct formats a percentage to 1 decimal place.
func FormatPct(n float64) string {
	return fmt.Sprintf("%.1f%%", n)
}

// NumberWithCommas formats an integer with comma separators.
func NumberWithCommas(n int) string {
	s := fmt.Sprintf("%d", n)
	if n < 0 {
		s = s[1:]
	}
	var out strings.Builder
	start := len(s) % 3
	if start == 0 {
		start = 3
	}
	out.WriteString(s[:start])
	for i := start; i < len(s); i += 3 {
		out.WriteByte(',')
		out.WriteString(s[i : i+3])
	}
	if n < 0 {
		return "-" + out.String()
	}
	return out.String()
}

// PctColor returns an ANSI color escape for a percentage value.
// green <50, yellow 50-75, red >=75. Returns "" if noColor.
func PctColor(pct float64, noColor bool) string {
	if noColor {
		return ""
	}
	switch {
	case pct < 50:
		return model.AnsiFg(34) // green
	case pct < 75:
		return model.AnsiFg(214) // yellow/orange
	default:
		return model.AnsiFg(196) // red
	}
}

var ansiRe = regexp.MustCompile(`\x1b\[[0-9;]*m`)

// StripAnsi strips ANSI escape sequences from a string.
func StripAnsi(s string) string {
	var b strings.Builder
	for len(s) > 0 {
		loc := ansiRe.FindStringIndex(s)
		if loc == nil {
			b.WriteString(s)
			break
		}
		b.WriteString(s[:loc[0]])
		s = s[loc[1]:]
	}
	return b.String()
}

// visibleLen returns the display length of a string (ignoring ANSI escapes).
func visibleLen(s string) int {
	return len([]rune(StripAnsi(s)))
}

// padEnd pads s with spaces on the right to reach the given visible width.
func padEnd(s string, width int) string {
	vl := visibleLen(s)
	if vl >= width {
		return s
	}
	return s + strings.Repeat(" ", width-vl)
}

// padStart pads s with spaces on the left to reach the given visible width.
func padStart(s string, width int) string {
	vl := visibleLen(s)
	if vl >= width {
		return s
	}
	return strings.Repeat(" ", width-vl) + s
}

// c returns the ANSI code or "" if noColor is true.
func c(code string, noColor bool) string {
	if noColor {
		return ""
	}
	return code
}

// truncStr truncates s to maxLen visible characters, appending "…".
func truncStr(s string, maxLen int) string {
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	if maxLen <= 1 {
		return "…"
	}
	return string(runes[:maxLen-1]) + "…"
}

// shortenPath replaces $HOME with ~ and shortens long paths to last 3 segments.
func shortenPath(fp string) string {
	home := os.Getenv("HOME")
	if home != "" && strings.HasPrefix(fp, home) {
		fp = "~" + fp[len(home):]
	}
	if len(fp) > 50 {
		parts := strings.Split(fp, "/")
		if len(parts) > 3 {
			parts = parts[len(parts)-3:]
			fp = "…/" + strings.Join(parts, "/")
		}
	}
	return fp
}

// ---------------------------------------------------------------------------
// Component helpers
// ---------------------------------------------------------------------------

func getWindowSize(comp *model.Composition) int {
	if comp.ContextWindowSize > 0 {
		return comp.ContextWindowSize
	}
	return 200000
}

func getTotalUsed(comp *model.Composition) int {
	if comp.APIMatchTokens > 0 {
		return comp.APIMatchTokens
	}
	// Sum non-buffer components
	var total int
	for _, c := range comp.Components {
		if c.Key != "buffer" {
			total += c.Tokens
		}
	}
	if total > 0 {
		return total
	}
	return comp.TotalUsedTokens
}

func getComponents(comp *model.Composition) []componentInfo {
	var defs []model.ComponentDef
	if comp.Tool == "codex" {
		defs = model.CodexComponents()
	} else {
		defs = model.ClaudeComponents()
	}

	windowSize := getWindowSize(comp)

	// Build a lookup from comp.Components by key
	compByKey := make(map[string]int, len(comp.Components))
	for _, cc := range comp.Components {
		compByKey[cc.Key] = cc.Tokens
	}

	result := make([]componentInfo, 0, len(defs))
	for _, def := range defs {
		tokens := def.DefaultTokens
		if len(comp.Components) > 0 {
			if t, ok := compByKey[def.Key]; ok {
				tokens = t
			} else {
				tokens = 0
			}
		}
		var pct float64
		if windowSize > 0 {
			pct = float64(tokens) / float64(windowSize) * 100
		}
		result = append(result, componentInfo{
			key:    def.Key,
			label:  def.Label,
			tokens: tokens,
			pct:    pct,
			color:  def.Color,
			ansi:   def.Ansi,
			fixed:  def.Fixed,
		})
	}
	return result
}

// ---------------------------------------------------------------------------
// RenderBar
// ---------------------------------------------------------------------------

// RenderBar renders a full-width stacked horizontal bar.
func RenderBar(comp *model.Composition, width int, opts RenderOpts) string {
	if width <= 0 {
		width = 80
	}
	components := getComponents(comp)
	windowSize := getWindowSize(comp)

	var sb strings.Builder

	// Calculate char widths proportional to tokens
	totalChars := 0
	type segment struct {
		info  componentInfo
		chars int
	}
	segments := make([]segment, 0, len(components))
	for _, ci := range components {
		if ci.tokens <= 0 || ci.key == "buffer" {
			continue
		}
		chars := int(math.Round(float64(ci.tokens) / float64(windowSize) * float64(width)))
		if chars < 1 {
			chars = 1
		}
		segments = append(segments, segment{info: ci, chars: chars})
		totalChars += chars
	}

	// Adjust to fit width
	if totalChars > width {
		diff := totalChars - width
		// Shrink largest segments
		for diff > 0 && len(segments) > 0 {
			maxIdx := 0
			for i, s := range segments {
				if s.chars > segments[maxIdx].chars {
					maxIdx = i
				}
			}
			segments[maxIdx].chars--
			totalChars--
			diff--
		}
	}

	// Render colored segments
	for _, seg := range segments {
		label := ""
		if seg.chars > 5 {
			label = fmt.Sprintf(" %.0f%%", seg.info.pct)
		}
		content := padEnd(label, seg.chars)
		if len([]rune(content)) > seg.chars {
			content = string([]rune(content)[:seg.chars])
		}
		if !opts.NoColor {
			sb.WriteString(model.AnsiBg(seg.info.ansi))
			sb.WriteString(model.AnsiFg(255)) // white text
		}
		sb.WriteString(content)
		if !opts.NoColor {
			sb.WriteString(model.Reset)
		}
	}

	// Free space
	freeChars := width - totalChars
	if freeChars > 0 {
		freeStr := strings.Repeat("░", freeChars)
		if !opts.NoColor {
			sb.WriteString(model.Dim)
		}
		sb.WriteString(freeStr)
		if !opts.NoColor {
			sb.WriteString(model.Reset)
		}
	}

	return sb.String()
}

// ---------------------------------------------------------------------------
// RenderTable
// ---------------------------------------------------------------------------

// RenderTable renders a component table with dots, token counts, percentages, mini bars.
func RenderTable(comp *model.Composition, opts RenderOpts) string {
	components := getComponents(comp)
	windowSize := getWindowSize(comp)
	totalUsed := getTotalUsed(comp)
	totalPct := float64(totalUsed) / float64(windowSize) * 100

	var sb strings.Builder

	// Header
	sb.WriteString(c(model.Bold, opts.NoColor))
	sb.WriteString(fmt.Sprintf("Context Window: %s / %s (%s)\n",
		FormatTokens(totalUsed),
		FormatTokens(windowSize),
		FormatPct(totalPct),
	))
	sb.WriteString(c(model.Reset, opts.NoColor))

	// Model info
	if comp.Model != "" {
		sb.WriteString(c(model.Dim, opts.NoColor))
		sb.WriteString(fmt.Sprintf("Model: %s", comp.Model))
		if comp.ModelTier != "" {
			sb.WriteString(fmt.Sprintf(" (%s)", comp.ModelTier))
		}
		sb.WriteString("\n")
		sb.WriteString(c(model.Reset, opts.NoColor))
	}
	sb.WriteString("\n")

	// Component rows
	maxLabelLen := 0
	for _, ci := range components {
		if len(ci.label) > maxLabelLen {
			maxLabelLen = len(ci.label)
		}
	}

	for _, ci := range components {
		if ci.tokens <= 0 {
			continue
		}
		dot := "●"
		if !opts.NoColor {
			sb.WriteString(model.AnsiFg(ci.ansi))
		}
		sb.WriteString(dot)
		sb.WriteString(c(model.Reset, opts.NoColor))
		sb.WriteString(" ")

		label := padEnd(ci.label, maxLabelLen)
		sb.WriteString(label)
		sb.WriteString("  ")

		// Token count right-aligned
		tokStr := padStart(FormatTokens(ci.tokens), 8)
		sb.WriteString(tokStr)
		sb.WriteString("  ")

		// Percentage
		pctStr := padStart(FormatPct(ci.pct), 6)
		if !opts.NoColor {
			sb.WriteString(PctColor(ci.pct, opts.NoColor))
		}
		sb.WriteString(pctStr)
		sb.WriteString(c(model.Reset, opts.NoColor))
		sb.WriteString("  ")

		// Mini bar (20 chars)
		miniWidth := 20
		filled := int(math.Round(ci.pct / 100 * float64(miniWidth)))
		if filled > miniWidth {
			filled = miniWidth
		}
		bar := strings.Repeat("█", filled) + strings.Repeat("░", miniWidth-filled)
		if !opts.NoColor {
			sb.WriteString(model.AnsiFg(ci.ansi))
		}
		sb.WriteString(bar)
		sb.WriteString(c(model.Reset, opts.NoColor))
		sb.WriteString("\n")
	}

	// Sections
	if len(comp.MemoryFiles) > 0 {
		sb.WriteString("\n")
		sb.WriteString(c(model.Bold, opts.NoColor))
		sb.WriteString("Memory Files\n")
		sb.WriteString(c(model.Reset, opts.NoColor))
		for _, mf := range comp.MemoryFiles {
			sb.WriteString(fmt.Sprintf("  %s  %s\n", padEnd(shortenPath(mf.Path), 40), FormatTokens(mf.Tokens)))
		}
	}

	if len(comp.AgentFiles) > 0 {
		sb.WriteString("\n")
		sb.WriteString(c(model.Bold, opts.NoColor))
		sb.WriteString("Agent Definitions\n")
		sb.WriteString(c(model.Reset, opts.NoColor))
		for _, af := range comp.AgentFiles {
			sb.WriteString(fmt.Sprintf("  %s  %s\n", padEnd(af.Name, 30), FormatTokens(af.Tokens)))
		}
	}

	if len(comp.MCPServers) > 0 {
		sb.WriteString("\n")
		sb.WriteString(c(model.Bold, opts.NoColor))
		sb.WriteString("MCP Servers\n")
		sb.WriteString(c(model.Reset, opts.NoColor))
		for _, ms := range comp.MCPServers {
			sb.WriteString(fmt.Sprintf("  %s  %d tools  %s\n",
				padEnd(ms.Name, 25),
				ms.ToolCount,
				FormatTokens(ms.EstimatedTokens),
			))
		}
	}

	if len(comp.Subagents) > 0 {
		sb.WriteString("\n")
		sb.WriteString(c(model.Bold, opts.NoColor))
		sb.WriteString(fmt.Sprintf("Subagents (%d)\n", len(comp.Subagents)))
		sb.WriteString(c(model.Reset, opts.NoColor))
		for _, sa := range comp.Subagents {
			desc := truncStr(sa.Description, 50)
			sb.WriteString(fmt.Sprintf("  [%s] %s\n", sa.SubagentType, desc))
		}
	}

	if comp.Skills != nil {
		sb.WriteString("\n")
		sb.WriteString(c(model.Bold, opts.NoColor))
		sb.WriteString(fmt.Sprintf("Skills (%d installed", comp.Skills.Installed))
		if len(comp.Skills.Active) > 0 {
			sb.WriteString(fmt.Sprintf(", %d active", len(comp.Skills.Active)))
		}
		sb.WriteString(")\n")
		sb.WriteString(c(model.Reset, opts.NoColor))
		for _, sf := range comp.Skills.Files {
			sb.WriteString(fmt.Sprintf("  %s  %s\n", padEnd(sf.Name, 30), FormatTokens(sf.FrontmatterTokens)))
		}
	}

	if len(comp.ToolCalls) > 0 {
		sb.WriteString("\n")
		sb.WriteString(c(model.Bold, opts.NoColor))
		sb.WriteString(fmt.Sprintf("Tool Calls (%d)\n", len(comp.ToolCalls)))
		sb.WriteString(c(model.Reset, opts.NoColor))
		// Aggregate by name
		counts := make(map[string]int)
		tokens := make(map[string]int)
		order := make([]string, 0)
		for _, tc := range comp.ToolCalls {
			if _, seen := counts[tc.Name]; !seen {
				order = append(order, tc.Name)
			}
			counts[tc.Name]++
			tokens[tc.Name] += tc.TokenEstimate
		}
		for _, name := range order {
			sb.WriteString(fmt.Sprintf("  %s  x%d  %s\n",
				padEnd(name, 30),
				counts[name],
				FormatTokens(tokens[name]),
			))
		}
	}

	return sb.String()
}

// ---------------------------------------------------------------------------
// RenderOrder
// ---------------------------------------------------------------------------

// RenderOrder renders the context loading order diagram.
func RenderOrder(comp *model.Composition, opts RenderOpts) string {
	components := getComponents(comp)

	var sb strings.Builder
	sb.WriteString(c(model.Bold, opts.NoColor))
	sb.WriteString("Context Loading Order\n")
	sb.WriteString(c(model.Reset, opts.NoColor))
	sb.WriteString("\n")

	// Pre-message components (fixed/non-buffer, non-user content)
	preKeys := map[string]bool{
		"system": true, "instructions": true,
		"tools": true, "mcp": true,
		"agents": true, "memory": true,
		"skill_meta": true, "skills": true,
	}
	postKeys := map[string]bool{
		"plan": true, "user_msg": true,
		"tool_results": true, "responses": true,
		"subagent": true, "reasoning": true,
		"skill_body": true,
	}

	for _, ci := range components {
		if ci.key == "buffer" || ci.key == "free" {
			continue
		}
		if preKeys[ci.key] && ci.tokens > 0 {
			if !opts.NoColor {
				sb.WriteString(model.AnsiFg(ci.ansi))
			}
			sb.WriteString(fmt.Sprintf("  %-30s %s\n", ci.label, FormatTokens(ci.tokens)))
			sb.WriteString(c(model.Reset, opts.NoColor))
		}
	}

	sb.WriteString(c(model.Dim, opts.NoColor))
	sb.WriteString("  ── user types first message ──\n")
	sb.WriteString(c(model.Reset, opts.NoColor))

	for _, ci := range components {
		if ci.key == "buffer" || ci.key == "free" {
			continue
		}
		if postKeys[ci.key] && ci.tokens > 0 {
			if !opts.NoColor {
				sb.WriteString(model.AnsiFg(ci.ansi))
			}
			sb.WriteString(fmt.Sprintf("  %-30s %s\n", ci.label, FormatTokens(ci.tokens)))
			sb.WriteString(c(model.Reset, opts.NoColor))
		}
	}

	// Buffer last
	for _, ci := range components {
		if ci.key == "buffer" && ci.tokens > 0 {
			sb.WriteString(c(model.Dim, opts.NoColor))
			sb.WriteString("  ── compaction trigger ──\n")
			sb.WriteString(c(model.Reset, opts.NoColor))
			if !opts.NoColor {
				sb.WriteString(model.AnsiFg(ci.ansi))
			}
			sb.WriteString(fmt.Sprintf("  %-30s %s\n", ci.label, FormatTokens(ci.tokens)))
			sb.WriteString(c(model.Reset, opts.NoColor))
		}
	}

	return sb.String()
}

// ---------------------------------------------------------------------------
// RenderAgents
// ---------------------------------------------------------------------------

// RenderAgents renders a box diagram showing main session and subagent isolation.
func RenderAgents(comp *model.Composition, opts RenderOpts) string {
	var sb strings.Builder

	sb.WriteString(c(model.Bold, opts.NoColor))
	sb.WriteString("Agent Context Isolation\n")
	sb.WriteString(c(model.Reset, opts.NoColor))
	sb.WriteString("\n")

	windowSize := getWindowSize(comp)
	totalUsed := getTotalUsed(comp)
	pct := float64(totalUsed) / float64(windowSize) * 100

	// Main session box
	boxWidth := 50
	border := strings.Repeat("─", boxWidth-2)
	sb.WriteString("┌" + border + "┐\n")
	title := fmt.Sprintf(" Main Session: %s ", comp.Model)
	sb.WriteString("│" + padEnd(title, boxWidth-2) + "│\n")
	usage := fmt.Sprintf(" %s / %s  (%s)", FormatTokens(totalUsed), FormatTokens(windowSize), FormatPct(pct))
	sb.WriteString("│" + padEnd(usage, boxWidth-2) + "│\n")

	// Mini bar inside box
	barWidth := boxWidth - 4
	filled := int(math.Round(pct / 100 * float64(barWidth)))
	if filled > barWidth {
		filled = barWidth
	}
	bar := " [" + strings.Repeat("█", filled) + strings.Repeat("░", barWidth-filled) + "]"
	sb.WriteString("│" + padEnd(bar, boxWidth-2) + "│\n")
	sb.WriteString("└" + border + "┘\n")

	if len(comp.Subagents) > 0 {
		sb.WriteString("\n")
		sb.WriteString(c(model.Dim, opts.NoColor))
		sb.WriteString(fmt.Sprintf("Spawned %d subagent(s) – each runs in an isolated context:\n", len(comp.Subagents)))
		sb.WriteString(c(model.Reset, opts.NoColor))
		sb.WriteString("\n")

		for i, sa := range comp.Subagents {
			if i >= 5 {
				sb.WriteString(fmt.Sprintf("  … and %d more\n", len(comp.Subagents)-5))
				break
			}
			desc := truncStr(sa.Description, 44)
			sb.WriteString("  ┌" + strings.Repeat("─", boxWidth-4) + "┐\n")
			tag := fmt.Sprintf(" [%s] %s", sa.SubagentType, desc)
			sb.WriteString("  │" + padEnd(tag, boxWidth-4) + "│\n")
			sb.WriteString("  └" + strings.Repeat("─", boxWidth-4) + "┘\n")
		}
	}

	return sb.String()
}

// ---------------------------------------------------------------------------
// RenderTimeline
// ---------------------------------------------------------------------------

// RenderTimeline renders an ASCII chart of context growth over time.
func RenderTimeline(comp *model.Composition, opts RenderOpts) string {
	var sb strings.Builder

	sb.WriteString(c(model.Bold, opts.NoColor))
	sb.WriteString("Context Timeline\n")
	sb.WriteString(c(model.Reset, opts.NoColor))
	sb.WriteString("\n")

	if len(comp.Turns) == 0 && len(comp.CompactionEvents) == 0 {
		sb.WriteString(c(model.Dim, opts.NoColor))
		sb.WriteString("No timeline data available.\n")
		sb.WriteString(c(model.Reset, opts.NoColor))
		return sb.String()
	}

	windowSize := getWindowSize(comp)
	chartWidth := 60
	chartHeight := 10

	// Build data points from turns (use TotalUsedTokens as proxy since we have no per-turn data)
	// If turns exist, distribute context growth linearly
	nPoints := len(comp.Turns)
	if nPoints < 2 {
		nPoints = 2
	}
	totalUsed := getTotalUsed(comp)

	points := make([]float64, nPoints)
	for i := range points {
		// Linear growth estimate
		points[i] = float64(totalUsed) * float64(i+1) / float64(nPoints)
	}

	// Include compaction events as dips
	_ = comp.CompactionEvents

	maxVal := float64(windowSize)

	// Render chart rows top-to-bottom
	for row := chartHeight - 1; row >= 0; row-- {
		threshold := maxVal * float64(row+1) / float64(chartHeight)
		prevThreshold := maxVal * float64(row) / float64(chartHeight)

		// Y axis label
		if row == chartHeight-1 {
			sb.WriteString(padStart(FormatTokens(int(maxVal)), 6))
		} else if row == 0 {
			sb.WriteString(padStart("0", 6))
		} else {
			sb.WriteString("      ")
		}
		sb.WriteString(" │")

		// Plot points
		for col := 0; col < chartWidth; col++ {
			pointIdx := int(float64(col) / float64(chartWidth) * float64(nPoints))
			if pointIdx >= nPoints {
				pointIdx = nPoints - 1
			}
			val := points[pointIdx]
			if val >= prevThreshold && val < threshold {
				if !opts.NoColor {
					sb.WriteString(model.AnsiFg(33))
				}
				sb.WriteString("▓")
				sb.WriteString(c(model.Reset, opts.NoColor))
			} else if val >= threshold {
				if !opts.NoColor {
					sb.WriteString(model.AnsiFg(33))
				}
				sb.WriteString("█")
				sb.WriteString(c(model.Reset, opts.NoColor))
			} else {
				sb.WriteString(" ")
			}
		}
		sb.WriteString("\n")
	}

	// X axis
	sb.WriteString("       └" + strings.Repeat("─", chartWidth) + "\n")
	sb.WriteString(fmt.Sprintf("         Turn 1%s Turn %d\n",
		strings.Repeat(" ", chartWidth-14),
		len(comp.Turns),
	))

	// Compaction events
	if len(comp.CompactionEvents) > 0 {
		sb.WriteString("\n")
		sb.WriteString(c(model.Dim, opts.NoColor))
		sb.WriteString(fmt.Sprintf("Compaction events: %d\n", len(comp.CompactionEvents)))
		for _, ev := range comp.CompactionEvents {
			sb.WriteString(fmt.Sprintf("  %s → %s  (%s)\n",
				FormatTokens(ev.PreSize),
				FormatTokens(ev.PostSize),
				ev.Timestamp,
			))
		}
		sb.WriteString(c(model.Reset, opts.NoColor))
	}

	return sb.String()
}

// ---------------------------------------------------------------------------
// RenderToolCalls
// ---------------------------------------------------------------------------

// RenderToolCalls renders tool call log with aggregation and per-call detail.
func RenderToolCalls(comp *model.Composition, opts RenderOpts) string {
	var sb strings.Builder

	sb.WriteString(c(model.Bold, opts.NoColor))
	sb.WriteString(fmt.Sprintf("Tool Calls (%d total)\n", len(comp.ToolCalls)))
	sb.WriteString(c(model.Reset, opts.NoColor))
	sb.WriteString("\n")

	if len(comp.ToolCalls) == 0 {
		sb.WriteString(c(model.Dim, opts.NoColor))
		sb.WriteString("No tool calls recorded.\n")
		sb.WriteString(c(model.Reset, opts.NoColor))
		return sb.String()
	}

	// Aggregate by name
	type agg struct {
		count  int
		tokens int
		calls  []model.ToolCall
	}
	aggMap := make(map[string]*agg)
	order := make([]string, 0)
	for _, tc := range comp.ToolCalls {
		if _, ok := aggMap[tc.Name]; !ok {
			aggMap[tc.Name] = &agg{}
			order = append(order, tc.Name)
		}
		a := aggMap[tc.Name]
		a.count++
		a.tokens += tc.TokenEstimate
		a.calls = append(a.calls, tc)
	}

	for _, name := range order {
		a := aggMap[name]
		sb.WriteString(c(model.Bold, opts.NoColor))
		sb.WriteString(fmt.Sprintf("  %s", name))
		sb.WriteString(c(model.Reset, opts.NoColor))
		sb.WriteString(c(model.Dim, opts.NoColor))
		sb.WriteString(fmt.Sprintf("  ×%d  ~%s tokens\n", a.count, FormatTokens(a.tokens)))
		sb.WriteString(c(model.Reset, opts.NoColor))

		for _, tc := range a.calls {
			detail := ""
			if tc.FilePath != "" {
				detail = shortenPath(tc.FilePath)
			} else if tc.ID != "" {
				detail = tc.ID
			}
			if detail != "" {
				sb.WriteString(c(model.Dim, opts.NoColor))
				sb.WriteString(fmt.Sprintf("    → %s", truncStr(detail, 60)))
				if tc.TokenEstimate > 0 {
					sb.WriteString(fmt.Sprintf("  %s", FormatTokens(tc.TokenEstimate)))
				}
				sb.WriteString("\n")
				sb.WriteString(c(model.Reset, opts.NoColor))
			}
		}
	}

	return sb.String()
}

// ---------------------------------------------------------------------------
// RenderCompact
// ---------------------------------------------------------------------------

// RenderCompact renders a single compact line: "Model │ 52.3% │ ████░░░░ │ 104.6k/200k".
func RenderCompact(comp *model.Composition, opts RenderOpts) string {
	windowSize := getWindowSize(comp)
	totalUsed := getTotalUsed(comp)
	pct := float64(totalUsed) / float64(windowSize) * 100

	barWidth := 8
	filled := int(math.Round(pct / 100 * float64(barWidth)))
	if filled > barWidth {
		filled = barWidth
	}
	bar := strings.Repeat("█", filled) + strings.Repeat("░", barWidth-filled)

	modelLabel := comp.Model
	if modelLabel == "" {
		modelLabel = comp.Tool
	}

	pctStr := FormatPct(pct)
	usage := fmt.Sprintf("%s/%s", FormatTokens(totalUsed), FormatTokens(windowSize))

	if opts.NoColor {
		return fmt.Sprintf("%s │ %s │ %s │ %s", modelLabel, pctStr, bar, usage)
	}

	return fmt.Sprintf("%s%s%s │ %s%s%s │ %s%s%s │ %s",
		model.Bold, modelLabel, model.Reset,
		PctColor(pct, opts.NoColor), pctStr, model.Reset,
		model.AnsiFg(33), bar, model.Reset,
		usage,
	)
}

// ---------------------------------------------------------------------------
// RenderStatusline
// ---------------------------------------------------------------------------

// RenderStatusline renders an ultra-compact statusline: "52% ████░░░░ 104.6k".
func RenderStatusline(comp *model.Composition, opts RenderOpts) string {
	windowSize := getWindowSize(comp)
	totalUsed := getTotalUsed(comp)
	pct := float64(totalUsed) / float64(windowSize) * 100

	barWidth := 8
	filled := int(math.Round(pct / 100 * float64(barWidth)))
	if filled > barWidth {
		filled = barWidth
	}
	bar := strings.Repeat("█", filled) + strings.Repeat("░", barWidth-filled)

	pctLabel := fmt.Sprintf("%.0f%%", pct)
	tokLabel := FormatTokens(totalUsed)

	if opts.NoColor {
		return fmt.Sprintf("%s %s %s", pctLabel, bar, tokLabel)
	}

	return fmt.Sprintf("%s%s%s %s%s%s %s",
		PctColor(pct, opts.NoColor), pctLabel, model.Reset,
		model.AnsiFg(33), bar, model.Reset,
		tokLabel,
	)
}

// ---------------------------------------------------------------------------
// RenderFull
// ---------------------------------------------------------------------------

// RenderFull renders bar + table combined.
func RenderFull(comp *model.Composition, opts RenderOpts) string {
	var sb strings.Builder
	sb.WriteString(RenderBar(comp, 80, opts))
	sb.WriteString("\n")
	sb.WriteString(RenderTable(comp, opts))
	return sb.String()
}

// ---------------------------------------------------------------------------
// RenderSetup
// ---------------------------------------------------------------------------

// RenderSetup renders setup instructions for the given mode.
func RenderSetup(mode string, opts RenderOpts) string {
	var sb strings.Builder

	sb.WriteString(c(model.Bold, opts.NoColor))
	sb.WriteString("ctx-monitor Setup\n")
	sb.WriteString(c(model.Reset, opts.NoColor))
	sb.WriteString("\n")

	switch mode {
	case "claude", "":
		sb.WriteString("To monitor Claude Code context usage:\n\n")
		sb.WriteString(c(model.Bold, opts.NoColor))
		sb.WriteString("1. Add to your shell profile (~/.zshrc or ~/.bashrc):\n")
		sb.WriteString(c(model.Reset, opts.NoColor))
		sb.WriteString(c(model.Dim, opts.NoColor))
		sb.WriteString("   export CTX_MONITOR_ENABLE=1\n\n")
		sb.WriteString(c(model.Reset, opts.NoColor))
		sb.WriteString(c(model.Bold, opts.NoColor))
		sb.WriteString("2. Run the monitor in a separate terminal:\n")
		sb.WriteString(c(model.Reset, opts.NoColor))
		sb.WriteString(c(model.Dim, opts.NoColor))
		sb.WriteString("   ctx-monitor watch\n\n")
		sb.WriteString(c(model.Reset, opts.NoColor))
		sb.WriteString(c(model.Bold, opts.NoColor))
		sb.WriteString("3. Or view a snapshot:\n")
		sb.WriteString(c(model.Reset, opts.NoColor))
		sb.WriteString(c(model.Dim, opts.NoColor))
		sb.WriteString("   ctx-monitor show\n\n")
		sb.WriteString(c(model.Reset, opts.NoColor))

	case "codex":
		sb.WriteString("To monitor Codex CLI context usage:\n\n")
		sb.WriteString(c(model.Bold, opts.NoColor))
		sb.WriteString("1. Run ctx-monitor with codex flag:\n")
		sb.WriteString(c(model.Reset, opts.NoColor))
		sb.WriteString(c(model.Dim, opts.NoColor))
		sb.WriteString("   ctx-monitor watch --tool codex\n\n")
		sb.WriteString(c(model.Reset, opts.NoColor))

	case "statusline":
		sb.WriteString("Statusline integration:\n\n")
		sb.WriteString(c(model.Bold, opts.NoColor))
		sb.WriteString("tmux status-right:\n")
		sb.WriteString(c(model.Reset, opts.NoColor))
		sb.WriteString(c(model.Dim, opts.NoColor))
		sb.WriteString("   set -g status-right '#(ctx-monitor statusline)'\n\n")
		sb.WriteString(c(model.Reset, opts.NoColor))
		sb.WriteString(c(model.Bold, opts.NoColor))
		sb.WriteString("Neovim statusline (lualine):\n")
		sb.WriteString(c(model.Reset, opts.NoColor))
		sb.WriteString(c(model.Dim, opts.NoColor))
		sb.WriteString("   require('lualine').setup({ sections = { lualine_x = { 'ctx-monitor' } } })\n\n")
		sb.WriteString(c(model.Reset, opts.NoColor))

	default:
		sb.WriteString(fmt.Sprintf("Unknown setup mode: %q\n", mode))
		sb.WriteString("Available modes: claude, codex, statusline\n")
	}

	return sb.String()
}
