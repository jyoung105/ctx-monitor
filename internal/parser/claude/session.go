package claude

import (
	"bufio"
	"encoding/json"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/tonylee/ctx-monitor/internal/model"
)

// SessionInfo describes a discovered session file.
type SessionInfo struct {
	ID       string
	FilePath string
	Mtime    time.Time
	Size     int64
}

// GetSessionDir returns the base Claude projects directory.
func GetSessionDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".claude", "projects")
}

// FindProjectDir finds the Claude project directory that corresponds to cwd.
// Claude encodes paths by replacing "/" with "-" (e.g. /Users/foo/bar → -Users-foo-bar).
func FindProjectDir(cwd string) string {
	base := GetSessionDir()

	// Strategy 1: dash-encoded
	dashEncoded := strings.ReplaceAll(cwd, "/", "-")
	candidate := filepath.Join(base, dashEncoded)
	if info, err := os.Stat(candidate); err == nil && info.IsDir() {
		return candidate
	}

	// Strategy 2: URL-encoded then check
	urlEncoded := url.PathEscape(cwd)
	candidate2 := filepath.Join(base, urlEncoded)
	if info, err := os.Stat(candidate2); err == nil && info.IsDir() {
		return candidate2
	}

	// Strategy 3: scan all subdirs and find best match
	entries, err := os.ReadDir(base)
	if err != nil {
		return ""
	}
	// Prefer exact dash-encoding match first, then suffix match
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		// Reverse the dash encoding and check if it matches cwd
		decoded := strings.ReplaceAll(e.Name(), "-", "/")
		if decoded == cwd {
			return filepath.Join(base, e.Name())
		}
	}
	// Fallback: suffix match (last path segment encoded)
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		if strings.HasSuffix(e.Name(), strings.ReplaceAll(filepath.Base(cwd), "/", "-")) {
			return filepath.Join(base, e.Name())
		}
	}

	return ""
}

// FindAllSessions lists all .jsonl session files in projectPath, sorted by mtime descending.
func FindAllSessions(projectPath string) []SessionInfo {
	entries, err := os.ReadDir(projectPath)
	if err != nil {
		return nil
	}

	var sessions []SessionInfo
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".jsonl") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		id := strings.TrimSuffix(e.Name(), ".jsonl")
		sessions = append(sessions, SessionInfo{
			ID:       id,
			FilePath: filepath.Join(projectPath, e.Name()),
			Mtime:    info.ModTime(),
			Size:     info.Size(),
		})
	}

	sort.Slice(sessions, func(i, j int) bool {
		return sessions[i].Mtime.After(sessions[j].Mtime)
	})
	return sessions
}

// FindLatestSession returns the most recently modified session in projectPath.
func FindLatestSession(projectPath string) *SessionInfo {
	sessions := FindAllSessions(projectPath)
	if len(sessions) == 0 {
		return nil
	}
	s := sessions[0]
	return &s
}

// tool bucket classification
const (
	bucketSkill    = "skill"
	bucketSubagent = "subagent"
	bucketPlan     = "plan"
	bucketDefault  = ""
)

var skillTools = map[string]bool{
	"Skill": true,
}

var subagentTools = map[string]bool{
	"Agent":       true,
	"Task":        true,
	"Explore":     true,
	"Plan":        true,
	"TaskCreate":  true,
	"SendMessage": true,
}

var planTools = map[string]bool{
	"TodoRead":  true,
	"TodoWrite": true,
	"todo_read": true,
	"todo_write": true,
	"plan":      true,
}

func classifyTool(name string) string {
	if skillTools[name] {
		return bucketSkill
	}
	if subagentTools[name] {
		return bucketSubagent
	}
	if planTools[name] {
		return bucketPlan
	}
	return bucketDefault
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

// isSystemMessage returns true if the text starts with a system/notification prefix.
func isSystemMessage(text string) bool {
	prefixes := []string{
		"<task-notification",
		"<system-reminder",
		"<available-deferred-tools",
	}
	for _, p := range prefixes {
		if strings.HasPrefix(strings.TrimSpace(text), p) {
			return true
		}
	}
	return false
}

// extractText extracts text from a content field that may be a string or array of blocks.
func extractText(content interface{}) string {
	switch v := content.(type) {
	case string:
		return v
	case []interface{}:
		var parts []string
		for _, item := range v {
			block, ok := item.(map[string]interface{})
			if !ok {
				continue
			}
			if t, _ := block["type"].(string); t == "text" {
				if txt, ok := block["text"].(string); ok {
					parts = append(parts, txt)
				}
			}
		}
		return strings.Join(parts, "\n")
	}
	return ""
}

// extractBlocks extracts all blocks from a content array.
func extractBlocks(content interface{}) []map[string]interface{} {
	arr, ok := content.([]interface{})
	if !ok {
		return nil
	}
	var blocks []map[string]interface{}
	for _, item := range arr {
		block, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		blocks = append(blocks, block)
	}
	return blocks
}

// getStr is a helper to pull a string from a generic map.
func getStr(m map[string]interface{}, key string) string {
	v, _ := m[key].(string)
	return v
}

// getFloat is a helper to pull a float64 from a generic map.
func getFloat(m map[string]interface{}, key string) float64 {
	v, _ := m[key].(float64)
	return v
}

// ParseSession parses a Claude Code JSONL session file.
func ParseSession(filePath string) (*model.ClaudeSession, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	// Derive UUID from filename (without extension).
	fileUUID := strings.TrimSuffix(filepath.Base(filePath), ".jsonl")

	session := &model.ClaudeSession{}
	var firstSessionID string
	turnIndex := 0
	msgIndex := 0

	scanner := bufio.NewScanner(f)
	scanner.Buffer(nil, 10<<20) // 10 MB max line

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var rec map[string]interface{}
		if err := json.Unmarshal(line, &rec); err != nil {
			continue
		}

		recType := getStr(rec, "type")
		timestamp := getStr(rec, "timestamp")

		// Track session metadata from any record.
		if sid := getStr(rec, "sessionId"); sid != "" && session.SessionID == "" {
			session.SessionID = sid
			firstSessionID = sid
			if sid != fileUUID {
				session.Continuation = true
			}
		}
		if m := getStr(rec, "model"); m != "" && session.Model == "" {
			session.Model = m
		}
		if v := getStr(rec, "version"); v != "" && session.Version == "" {
			session.Version = v
		}
		if c := getStr(rec, "cwd"); c != "" && session.Cwd == "" {
			session.Cwd = c
		}

		// Track timestamps.
		if timestamp != "" {
			if session.Timestamps.First == "" {
				session.Timestamps.First = timestamp
			}
			session.Timestamps.Last = timestamp
		}

		// Suppress unused variable warning for firstSessionID.
		_ = firstSessionID

		switch recType {
		case "compact_boundary", "summary":
			session.CompactionEvents.Count++
			session.CompactionEvents.Timestamps = append(session.CompactionEvents.Timestamps, timestamp)

		case "user":
			content := rec["message"]
			if content == nil {
				// Some records store content directly.
				content = rec["content"]
			}
			// Unwrap message object if present.
			var msgContent interface{}
			if msgMap, ok := content.(map[string]interface{}); ok {
				msgContent = msgMap["content"]
			} else {
				msgContent = content
			}

			text := extractText(msgContent)
			blocks := extractBlocks(msgContent)

			// Check if this is a system message (skip for turn counting).
			isSystem := isSystemMessage(text)

			// Process tool_result blocks and attachments.
			var toolResultCount int
			var attachmentCount int
			var attachmentTokens int

			for _, block := range blocks {
				bt := getStr(block, "type")
				switch bt {
				case "tool_result":
					toolResultCount++
					resultContent := extractText(block["content"])
					tok := model.EstimateTokens(resultContent)
					tr := model.ToolResult{
						ToolUseID:     getStr(block, "tool_use_id"),
						Content:       truncate(resultContent, 500),
						TokenEstimate: tok,
					}
					session.ToolResults = append(session.ToolResults, tr)
					session.TokenBuckets.ToolResults += tok

				case "image":
					attachmentCount++
					source, _ := block["source"].(map[string]interface{})
					mediaType := ""
					sizeBytes := 0
					if source != nil {
						mediaType = getStr(source, "media_type")
						if data, ok := source["data"].(string); ok {
							sizeBytes = len(data) * 3 / 4 // base64 decode estimate
						}
					}
					imgTokens := 1000 // rough estimate for image
					attachmentTokens += imgTokens
					session.Attachments = append(session.Attachments, model.Attachment{
						Type:      "image",
						MediaType: mediaType,
						Tokens:    imgTokens,
						SizeBytes: sizeBytes,
						Timestamp: timestamp,
					})

				case "document":
					attachmentCount++
					source, _ := block["source"].(map[string]interface{})
					name := getStr(block, "title")
					sizeBytes := 0
					docText := ""
					if source != nil {
						if data, ok := source["data"].(string); ok {
							sizeBytes = len(data)
							docText = data
						} else if txt, ok := source["text"].(string); ok {
							docText = txt
							sizeBytes = len(docText)
						}
					}
					docTokens := model.EstimateTokens(docText)
					attachmentTokens += docTokens
					session.Attachments = append(session.Attachments, model.Attachment{
						Type:      "document",
						Name:      name,
						Tokens:    docTokens,
						SizeBytes: sizeBytes,
						Timestamp: timestamp,
					})
				}
			}

			if !isSystem {
				// Record user turn.
				turnText := truncate(text, 120)
				session.Turns = append(session.Turns, model.Turn{
					Index:        turnIndex,
					Timestamp:    timestamp,
					Text:         turnText,
					Attachments:  attachmentCount,
					MessageIndex: msgIndex,
				})
				turnIndex++

				userTokens := model.EstimateTokens(text)
				session.TokenBuckets.UserMsg += userTokens

				userType := "human"
				if isSystem {
					userType = "system"
				}

				msg := model.Message{
					Role:            "user",
					Type:            recType,
					UUID:            getStr(rec, "uuid"),
					ParentUUID:      getStr(rec, "parentUuid"),
					Timestamp:       timestamp,
					Text:            truncate(text, 1000),
					TokenEstimate:   userTokens,
					ToolResultCount: toolResultCount,
					AttachmentCount: attachmentCount,
					AttachmentTokens: attachmentTokens,
					UserType:        userType,
				}
				session.Messages = append(session.Messages, msg)
				msgIndex++
			}

		case "assistant":
			content := rec["message"]
			var msgContent interface{}
			if msgMap, ok := content.(map[string]interface{}); ok {
				// Capture model and usage from message object.
				if m := getStr(msgMap, "model"); m != "" {
					session.Model = m
				}
				if usageMap, ok := msgMap["usage"].(map[string]interface{}); ok {
					usage := &model.UsageData{
						InputTokens:              int(getFloat(usageMap, "input_tokens")),
						CacheCreationInputTokens: int(getFloat(usageMap, "cache_creation_input_tokens")),
						CacheReadInputTokens:     int(getFloat(usageMap, "cache_read_input_tokens")),
						OutputTokens:             int(getFloat(usageMap, "output_tokens")),
					}
					session.Usage = usage
				}
				msgContent = msgMap["content"]
			} else {
				msgContent = content
			}

			blocks := extractBlocks(msgContent)
			var textParts []string
			var toolCallCount int
			var thinkingCount int
			var thinkingTokens int

			for _, block := range blocks {
				bt := getStr(block, "type")
				switch bt {
				case "text":
					txt := getStr(block, "text")
					textParts = append(textParts, txt)

				case "thinking":
					thinkingCount++
					thinkingText := getStr(block, "thinking")
					tok := model.EstimateTokens(thinkingText)
					thinkingTokens += tok
					session.TokenBuckets.Thinking += tok

				case "tool_use":
					toolCallCount++
					toolName := getStr(block, "name")
					toolID := getStr(block, "id")
					inputRaw := block["input"]

					tc := model.ToolCall{
						Name:      toolName,
						Input:     inputRaw,
						ID:        toolID,
						Timestamp: timestamp,
					}

					// Estimate tokens from input JSON.
					inputJSON, _ := json.Marshal(inputRaw)
					tok := model.EstimateTokens(string(inputJSON))
					tc.TokenEstimate = tok

					session.ToolCalls = append(session.ToolCalls, tc)

					bucket := classifyTool(toolName)
					switch bucket {
					case bucketSkill:
						session.TokenBuckets.SkillBody += tok
						// Extract skill name from input.
						skillName := ""
						skillArgs := ""
						if inputMap, ok := inputRaw.(map[string]interface{}); ok {
							skillName = getStr(inputMap, "skill")
							skillArgs = getStr(inputMap, "args")
						}
						session.SkillActivations = append(session.SkillActivations, model.SkillActivation{
							Skill:     skillName,
							Args:      skillArgs,
							Timestamp: timestamp,
							ID:        toolID,
						})

					case bucketSubagent:
						session.TokenBuckets.Subagent += tok
						subType := ""
						desc := ""
						if inputMap, ok := inputRaw.(map[string]interface{}); ok {
							subType = getStr(inputMap, "subagent_type")
							if subType == "" {
								subType = getStr(inputMap, "type")
							}
							desc = getStr(inputMap, "description")
							if desc == "" {
								desc = getStr(inputMap, "prompt")
							}
							desc = truncate(desc, 200)
						}
						session.SubagentSpawns = append(session.SubagentSpawns, model.SubagentSpawn{
							Tool:         toolName,
							SubagentType: subType,
							Description:  desc,
							Timestamp:    timestamp,
							ID:           toolID,
						})

					case bucketPlan:
						session.TokenBuckets.Plan += tok
						session.PlanUsage = append(session.PlanUsage, model.PlanEvent{
							Tool:      toolName,
							Input:     inputRaw,
							Timestamp: timestamp,
							ID:        toolID,
						})

					default:
						session.TokenBuckets.Responses += tok
					}
				}
			}

			fullText := strings.Join(textParts, "\n")
			responseTokens := model.EstimateTokens(fullText)
			session.TokenBuckets.Responses += responseTokens

			if thinkingCount > 0 {
				session.ThinkingStats.Count += thinkingCount
				session.ThinkingStats.TotalEstimatedTokens += thinkingTokens
			}

			msg := model.Message{
				Role:               "assistant",
				Type:               recType,
				UUID:               getStr(rec, "uuid"),
				ParentUUID:         getStr(rec, "parentUuid"),
				Timestamp:          timestamp,
				Text:               truncate(fullText, 1000),
				TokenEstimate:      responseTokens,
				Model:              session.Model,
				ToolCallCount:      toolCallCount,
				ThinkingBlockCount: thinkingCount,
			}
			session.Messages = append(session.Messages, msg)
			msgIndex++
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return session, nil
}
