package codex

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/tonylee/ctx-monitor/internal/model"
)

// CodexSessionInfo holds metadata about a Codex session file.
type CodexSessionInfo struct {
	Path  string
	Name  string
	Mtime time.Time
	Size  int64
}

// FindCodexHome returns the Codex home directory.
// Priority: CODEX_HOME env, ~/.codex, ~/.codex_home.
func FindCodexHome() string {
	if v := os.Getenv("CODEX_HOME"); v != "" {
		return v
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	primary := filepath.Join(home, ".codex")
	if info, err := os.Stat(primary); err == nil && info.IsDir() {
		return primary
	}
	return filepath.Join(home, ".codex_home")
}

// FindAllSessions returns all Codex session JSONL files sorted by mtime descending.
func FindAllSessions() ([]CodexSessionInfo, error) {
	codexHome := FindCodexHome()
	sessionsDir := filepath.Join(codexHome, "sessions")

	var sessions []CodexSessionInfo
	err := filepath.Walk(sessionsDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // skip unreadable dirs
		}
		if !info.IsDir() && strings.HasSuffix(path, ".jsonl") {
			sessions = append(sessions, CodexSessionInfo{
				Path:  path,
				Name:  strings.TrimSuffix(filepath.Base(path), ".jsonl"),
				Mtime: info.ModTime(),
				Size:  info.Size(),
			})
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walking sessions dir %s: %w", sessionsDir, err)
	}

	sort.Slice(sessions, func(i, j int) bool {
		return sessions[i].Mtime.After(sessions[j].Mtime)
	})
	return sessions, nil
}

// FindLatestSession returns the most recently modified session.
func FindLatestSession() (*CodexSessionInfo, error) {
	sessions, err := FindAllSessions()
	if err != nil {
		return nil, err
	}
	if len(sessions) == 0 {
		return nil, fmt.Errorf("no codex sessions found")
	}
	return &sessions[0], nil
}

// --- JSON shapes for raw line parsing ---

type rawLine struct {
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload"`
	// top-level fields that appear on various event types
	Role             string          `json:"role"`
	Content          json.RawMessage `json:"content"`
	Model            string          `json:"model"`
	ReasoningEffort  string          `json:"reasoning_effort"`
	ModelContextWindow int           `json:"model_context_window"`
	ID               string          `json:"id"`
}

type rawPayload struct {
	Type               string          `json:"type"`
	Model              string          `json:"model"`
	ReasoningEffort    string          `json:"reasoning_effort"`
	ModelContextWindow int             `json:"model_context_window"`
	SessionID          string          `json:"session_id"`
	// token_count fields
	Usage              json.RawMessage `json:"usage"`
	TokenCount         json.RawMessage `json:"token_count"`
	InputTokens        int             `json:"input_tokens"`
	OutputTokens       int             `json:"output_tokens"`
	CachedTokens       int             `json:"cached_tokens"`
	ReasoningTokens    int             `json:"reasoning_tokens"`
	TotalTokens        int             `json:"total_tokens"`
	// tool_call fields
	Name               string          `json:"name"`
	Arguments          json.RawMessage `json:"arguments"`
	CallID             string          `json:"call_id"`
	// tool_result fields
	Content            string          `json:"content_str"`
	// compaction fields
	PreContextSize     int             `json:"pre_context_size"`
	PostContextSize    int             `json:"post_context_size"`
	Timestamp          string          `json:"timestamp"`
	// response_item fields
	Item               json.RawMessage `json:"item"`
	// raw content array
	ContentArr         json.RawMessage `json:"content"`
}

type rawTokenUsage struct {
	Total     int `json:"total_tokens"`
	Input     int `json:"input_tokens"`
	Cached    int `json:"cached_tokens"`
	Output    int `json:"output_tokens"`
	Reasoning int `json:"reasoning_tokens"`
	// alternate field names
	InputTotal  int `json:"input_token_count"`
	OutputTotal int `json:"output_token_count"`
	CacheHit    int `json:"cache_read_input_tokens"`
}

type rawContentItem struct {
	Type  string `json:"type"`
	Text  string `json:"text"`
	// function call fields
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
	ID        string          `json:"id"`
	CallID    string          `json:"call_id"`
	// tool result
	Output string `json:"output"`
}

var (
	patchFileRe = regexp.MustCompile(`\*\*\*\s+(.+)`)
	patchHunkRe = regexp.MustCompile(`@@@\s+-(\d+),(\d+)\s+\+(\d+),(\d+)\s+@@@`)
)

// ParseSession parses a Codex JSONL session file and returns a CodexSession.
func ParseSession(filePath string) (*model.CodexSession, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("opening session file: %w", err)
	}
	defer f.Close()

	session := &model.CodexSession{
		File: filePath,
	}

	scanner := bufio.NewScanner(f)
	const maxBuf = 10 * 1024 * 1024 // 10MB
	buf := make([]byte, maxBuf)
	scanner.Buffer(buf, maxBuf)

	lineCount := 0
	parseErrors := 0

	for scanner.Scan() {
		lineCount++
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var raw rawLine
		if err := json.Unmarshal(line, &raw); err != nil {
			parseErrors++
			continue
		}

		processLine(session, &raw, line)
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scanning session file: %w", err)
	}

	session.RawStats = model.RawStats{
		LineCount:   lineCount,
		ParseErrors: parseErrors,
	}

	return session, nil
}

func processLine(session *model.CodexSession, raw *rawLine, line []byte) {
	switch raw.Type {
	case "session_meta":
		handleSessionMeta(session, line)

	case "turn_context":
		handleTurnContext(session, raw, line)

	case "response_item":
		handleResponseItem(session, line)

	case "event_msg":
		handleEventMsg(session, raw)

	default:
		// Some events may have no type or unknown types — ignore
	}
}

func handleSessionMeta(session *model.CodexSession, line []byte) {
	var v struct {
		SessionID          string `json:"session_id"`
		ID                 string `json:"id"`
		Model              string `json:"model"`
		ModelContextWindow int    `json:"model_context_window"`
	}
	if err := json.Unmarshal(line, &v); err != nil {
		return
	}
	if v.SessionID != "" {
		session.SessionID = v.SessionID
	} else if v.ID != "" {
		session.SessionID = v.ID
	}
	if v.Model != "" {
		session.Model = v.Model
	}
	if v.ModelContextWindow > 0 {
		session.ContextWindowSize = v.ModelContextWindow
	}
}

func handleTurnContext(session *model.CodexSession, raw *rawLine, line []byte) {
	var v struct {
		Model              string `json:"model"`
		ReasoningEffort    string `json:"reasoning_effort"`
		ModelContextWindow int    `json:"model_context_window"`
	}
	if err := json.Unmarshal(line, &v); err != nil {
		return
	}
	if v.Model != "" {
		session.Model = v.Model
	}
	if v.ReasoningEffort != "" {
		session.ReasoningEffort = v.ReasoningEffort
	}
	if v.ModelContextWindow > 0 {
		session.ContextWindowSize = v.ModelContextWindow
	}
}

func handleResponseItem(session *model.CodexSession, line []byte) {
	var v struct {
		Role    string            `json:"role"`
		Content []rawContentItem  `json:"content"`
		// alternate: single content item
		Type      string `json:"item_type"`
		Text      string `json:"text"`
		Name      string `json:"name"`
		Arguments json.RawMessage `json:"arguments"`
		ID        string `json:"id"`
	}
	if err := json.Unmarshal(line, &v); err != nil {
		return
	}

	role := v.Role
	if role == "" {
		role = "assistant"
	}

	for _, item := range v.Content {
		switch item.Type {
		case "text", "output_text":
			session.TokenBuckets.Responses += model.EstimateTokens(item.Text)

		case "input_text":
			session.TokenBuckets.UserMsg += model.EstimateTokens(item.Text)

		case "function_call":
			name := item.Name
			argStr := argString(item.Arguments)
			tc := model.CodexToolCall{
				Name:          name,
				Arguments:     argStr,
				ID:            firstNonEmpty(item.ID, item.CallID),
				TokenEstimate: model.EstimateTokens(argStr),
			}
			if name == "apply_patch" {
				tc.PatchInfo = parseApplyPatch(argStr)
			}
			switch {
			case name == "spawn_agent" || strings.Contains(name, "agent"):
				session.SubagentSpawns = append(session.SubagentSpawns, tc)
				session.TokenBuckets.Subagent += tc.TokenEstimate
			case name == "update_plan" || name == "create_plan" || strings.Contains(name, "plan"):
				session.PlanUsage = append(session.PlanUsage, tc)
				session.TokenBuckets.Plan += tc.TokenEstimate
			default:
				session.ToolCalls = append(session.ToolCalls, tc)
			}

		case "function_call_output":
			content := item.Output
			if len(content) > 500 {
				content = content[:500]
			}
			tr := model.ToolResult{
				Content:       content,
				TokenEstimate: model.EstimateTokens(content),
			}
			session.ToolResults = append(session.ToolResults, tr)
			session.TokenBuckets.ToolResults += tr.TokenEstimate

		case "reasoning":
			session.TokenBuckets.Reasoning += model.EstimateTokens(item.Text)
		}
	}
}

func handleEventMsg(session *model.CodexSession, raw *rawLine) {
	if raw.Payload == nil {
		return
	}

	var p rawPayload
	if err := json.Unmarshal(raw.Payload, &p); err != nil {
		return
	}

	switch p.Type {
	case "token_count":
		handleTokenCount(session, &p)

	case "task_started":
		if p.ModelContextWindow > 0 {
			session.ContextWindowSize = p.ModelContextWindow
		}
		if p.Model != "" {
			session.Model = p.Model
		}

	case "turn_started":
		session.Turns = append(session.Turns, map[string]interface{}{
			"type":      "turn_started",
			"timestamp": p.Timestamp,
		})

	case "turn_completed":
		session.Turns = append(session.Turns, map[string]interface{}{
			"type":      "turn_completed",
			"timestamp": p.Timestamp,
		})

	case "context_compacted":
		ce := model.CompactionEvent{
			Timestamp: p.Timestamp,
			PreSize:   p.PreContextSize,
			PostSize:  p.PostContextSize,
		}
		session.CompactionEvents = append(session.CompactionEvents, ce)

	case "tool_call":
		handlePayloadToolCall(session, &p)

	case "tool_result":
		handlePayloadToolResult(session, &p)

	case "response_item":
		handlePayloadResponseItem(session, &p)

	case "session_meta":
		if p.SessionID != "" {
			session.SessionID = p.SessionID
		}
		if p.Model != "" {
			session.Model = p.Model
		}
		if p.ModelContextWindow > 0 {
			session.ContextWindowSize = p.ModelContextWindow
		}

	case "user_message", "input_text":
		// accumulate user message bucket
		var text string
		if p.ContentArr != nil {
			var items []rawContentItem
			if err := json.Unmarshal(p.ContentArr, &items); err == nil {
				for _, it := range items {
					text += it.Text
				}
			} else {
				// maybe it's a plain string
				_ = json.Unmarshal(p.ContentArr, &text)
			}
		}
		session.TokenBuckets.UserMsg += model.EstimateTokens(text)
	}
}

func handleTokenCount(session *model.CodexSession, p *rawPayload) {
	var usage rawTokenUsage

	// Try nested usage object first
	if p.Usage != nil {
		_ = json.Unmarshal(p.Usage, &usage)
	}
	// Try nested token_count object
	if p.TokenCount != nil {
		var nested rawTokenUsage
		if err := json.Unmarshal(p.TokenCount, &nested); err == nil {
			mergeTokenUsage(&usage, &nested)
		}
	}

	// Direct fields on payload
	if p.TotalTokens > 0 {
		usage.Total = p.TotalTokens
	}
	if p.InputTokens > 0 {
		usage.Input = p.InputTokens
	}
	if p.OutputTokens > 0 {
		usage.Output = p.OutputTokens
	}
	if p.CachedTokens > 0 {
		usage.Cached = p.CachedTokens
	}
	if p.ReasoningTokens > 0 {
		usage.Reasoning = p.ReasoningTokens
	}

	// Normalize alternate field names
	if usage.Input == 0 && usage.InputTotal > 0 {
		usage.Input = usage.InputTotal
	}
	if usage.Output == 0 && usage.OutputTotal > 0 {
		usage.Output = usage.OutputTotal
	}
	if usage.Cached == 0 && usage.CacheHit > 0 {
		usage.Cached = usage.CacheHit
	}

	// Derive total if missing
	if usage.Total == 0 {
		usage.Total = usage.Input + usage.Output + usage.Reasoning
	}

	cu := model.CodexTokenUsage{
		Total:     usage.Total,
		Input:     usage.Input,
		Cached:    usage.Cached,
		Output:    usage.Output,
		Reasoning: usage.Reasoning,
	}

	// Accumulate into overall usage
	session.TokenUsage.Total += cu.Total
	session.TokenUsage.Input += cu.Input
	session.TokenUsage.Cached += cu.Cached
	session.TokenUsage.Output += cu.Output
	session.TokenUsage.Reasoning += cu.Reasoning

	// Track last usage separately
	if cu.Total > 0 || cu.Input > 0 || cu.Output > 0 {
		session.LastTokenUsage = cu
	}
}

func handlePayloadToolCall(session *model.CodexSession, p *rawPayload) {
	argStr := argString(p.Arguments)
	tc := model.CodexToolCall{
		Name:          p.Name,
		Arguments:     argStr,
		ID:            firstNonEmpty(p.CallID, p.SessionID),
		Timestamp:     p.Timestamp,
		TokenEstimate: model.EstimateTokens(argStr),
	}
	if p.Name == "apply_patch" {
		tc.PatchInfo = parseApplyPatch(argStr)
	}
	switch {
	case p.Name == "spawn_agent" || strings.Contains(p.Name, "agent"):
		session.SubagentSpawns = append(session.SubagentSpawns, tc)
		session.TokenBuckets.Subagent += tc.TokenEstimate
	case p.Name == "update_plan" || p.Name == "create_plan" || strings.Contains(p.Name, "plan"):
		session.PlanUsage = append(session.PlanUsage, tc)
		session.TokenBuckets.Plan += tc.TokenEstimate
	default:
		session.ToolCalls = append(session.ToolCalls, tc)
	}
}

func handlePayloadToolResult(session *model.CodexSession, p *rawPayload) {
	// Content may be in ContentArr or Content field
	var content string
	if p.ContentArr != nil {
		var items []rawContentItem
		if err := json.Unmarshal(p.ContentArr, &items); err == nil {
			var sb strings.Builder
			for _, it := range items {
				if it.Text != "" {
					sb.WriteString(it.Text)
				} else if it.Output != "" {
					sb.WriteString(it.Output)
				}
			}
			content = sb.String()
		} else {
			_ = json.Unmarshal(p.ContentArr, &content)
		}
	}
	if content == "" {
		content = p.Content
	}
	if len(content) > 500 {
		content = content[:500]
	}
	tr := model.ToolResult{
		Content:       content,
		TokenEstimate: model.EstimateTokens(content),
	}
	session.ToolResults = append(session.ToolResults, tr)
	session.TokenBuckets.ToolResults += tr.TokenEstimate
}

func handlePayloadResponseItem(session *model.CodexSession, p *rawPayload) {
	if p.Item == nil {
		return
	}
	var item rawContentItem
	if err := json.Unmarshal(p.Item, &item); err != nil {
		return
	}
	switch item.Type {
	case "text", "output_text":
		session.TokenBuckets.Responses += model.EstimateTokens(item.Text)
	case "function_call":
		argStr := argString(item.Arguments)
		tc := model.CodexToolCall{
			Name:          item.Name,
			Arguments:     argStr,
			ID:            firstNonEmpty(item.ID, item.CallID),
			TokenEstimate: model.EstimateTokens(argStr),
		}
		if item.Name == "apply_patch" {
			tc.PatchInfo = parseApplyPatch(argStr)
		}
		session.ToolCalls = append(session.ToolCalls, tc)
	}
}

// parseApplyPatch extracts file paths and hunk headers from apply_patch content.
func parseApplyPatch(argStr string) *model.PatchInfo {
	info := &model.PatchInfo{}
	seenFiles := map[string]bool{}

	lines := strings.Split(argStr, "\n")
	for _, l := range lines {
		if m := patchFileRe.FindStringSubmatch(l); len(m) == 2 {
			f := strings.TrimSpace(m[1])
			if !seenFiles[f] {
				seenFiles[f] = true
				info.Files = append(info.Files, f)
			}
		}
		if m := patchHunkRe.FindStringSubmatch(l); len(m) == 5 {
			info.Hunks = append(info.Hunks, model.HunkInfo{
				OldStart: atoi(m[1]),
				OldCount: atoi(m[2]),
				NewStart: atoi(m[3]),
				NewCount: atoi(m[4]),
			})
		}
	}
	if len(info.Files) == 0 && len(info.Hunks) == 0 {
		return nil
	}
	return info
}

func mergeTokenUsage(dst, src *rawTokenUsage) {
	if src.Total > dst.Total {
		dst.Total = src.Total
	}
	if src.Input > dst.Input {
		dst.Input = src.Input
	}
	if src.Output > dst.Output {
		dst.Output = src.Output
	}
	if src.Cached > dst.Cached {
		dst.Cached = src.Cached
	}
	if src.Reasoning > dst.Reasoning {
		dst.Reasoning = src.Reasoning
	}
	if src.InputTotal > dst.InputTotal {
		dst.InputTotal = src.InputTotal
	}
	if src.OutputTotal > dst.OutputTotal {
		dst.OutputTotal = src.OutputTotal
	}
	if src.CacheHit > dst.CacheHit {
		dst.CacheHit = src.CacheHit
	}
}

func argString(raw json.RawMessage) string {
	if raw == nil {
		return ""
	}
	// If it's a JSON string, unwrap it
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}
	return string(raw)
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

func atoi(s string) int {
	n, _ := strconv.Atoi(s)
	return n
}
