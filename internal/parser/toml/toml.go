// Package toml provides a minimal TOML parser with zero external dependencies.
// Supports: key=value pairs, quoted/bare strings, numbers, booleans,
// single-line and multi-line arrays, [table] and [table.subtable] headers,
// and # comments.
// Does NOT support: inline tables, datetime, multi-line basic strings, literal strings.
package toml

import (
	"strconv"
	"strings"
	"unicode"
)

// Parse parses a TOML document and returns a nested map.
// Values are typed as: string, int, float64, bool, []interface{}, map[string]interface{}.
func Parse(text string) map[string]interface{} {
	root := make(map[string]interface{})
	current := root

	lines := strings.Split(text, "\n")

	// We may need to accumulate lines for multi-line arrays.
	i := 0
	for i < len(lines) {
		line := lines[i]

		// Strip inline comment (outside quotes) and trim whitespace.
		line = stripComment(line)
		line = strings.TrimSpace(line)

		if line == "" {
			i++
			continue
		}

		// Table header: [table] or [table.subtable]
		if strings.HasPrefix(line, "[") && !strings.HasPrefix(line, "[[") {
			end := strings.Index(line, "]")
			if end == -1 {
				i++
				continue
			}
			header := strings.TrimSpace(line[1:end])
			current = navigateTo(root, header)
			i++
			continue
		}

		// Key = value
		eq := strings.Index(line, "=")
		if eq == -1 {
			i++
			continue
		}

		key := strings.TrimSpace(line[:eq])
		rawVal := strings.TrimSpace(line[eq+1:])

		// Check if this is the start of a multi-line array.
		if strings.HasPrefix(rawVal, "[") && !isCompleteArray(rawVal) {
			// Accumulate lines until brackets balance.
			accumulated := rawVal
			i++
			for i < len(lines) && !isCompleteArray(accumulated) {
				nextLine := stripComment(lines[i])
				nextLine = strings.TrimSpace(nextLine)
				accumulated += " " + nextLine
				i++
			}
			rawVal = accumulated
		} else {
			i++
		}

		val := parseValue(rawVal)
		setKey(current, key, val)
	}

	return root
}

// navigateTo traverses/creates nested maps for a dotted table path.
func navigateTo(root map[string]interface{}, path string) map[string]interface{} {
	parts := splitDotted(path)
	current := root
	for _, part := range parts {
		part = strings.Trim(part, `"`)
		existing, ok := current[part]
		if !ok {
			child := make(map[string]interface{})
			current[part] = child
			current = child
		} else if child, ok := existing.(map[string]interface{}); ok {
			current = child
		} else {
			// Conflict: overwrite with a new map.
			child := make(map[string]interface{})
			current[part] = child
			current = child
		}
	}
	return current
}

// setKey sets a (possibly dotted) key in the current map.
func setKey(m map[string]interface{}, key string, val interface{}) {
	parts := splitDotted(key)
	if len(parts) == 1 {
		k := strings.Trim(key, `"`)
		m[k] = val
		return
	}
	// Dotted key: navigate/create intermediate maps.
	current := m
	for idx, part := range parts {
		part = strings.Trim(part, `"`)
		if idx == len(parts)-1 {
			current[part] = val
		} else {
			existing, ok := current[part]
			if !ok {
				child := make(map[string]interface{})
				current[part] = child
				current = child
			} else if child, ok := existing.(map[string]interface{}); ok {
				current = child
			} else {
				child := make(map[string]interface{})
				current[part] = child
				current = child
			}
		}
	}
}

// splitDotted splits a key by dots, respecting quoted segments.
func splitDotted(s string) []string {
	var parts []string
	var buf strings.Builder
	inQuote := false
	for _, ch := range s {
		switch {
		case ch == '"':
			inQuote = !inQuote
			buf.WriteRune(ch)
		case ch == '.' && !inQuote:
			parts = append(parts, buf.String())
			buf.Reset()
		default:
			buf.WriteRune(ch)
		}
	}
	if buf.Len() > 0 {
		parts = append(parts, buf.String())
	}
	return parts
}

// isCompleteArray returns true if the brackets in s are balanced (and s starts with '[').
func isCompleteArray(s string) bool {
	depth := 0
	inStr := false
	for i := 0; i < len(s); i++ {
		ch := s[i]
		if inStr {
			if ch == '\\' {
				i++ // skip escaped char
			} else if ch == '"' {
				inStr = false
			}
			continue
		}
		switch ch {
		case '"':
			inStr = true
		case '[':
			depth++
		case ']':
			depth--
			if depth == 0 {
				return true
			}
		}
	}
	return false
}

// parseValue parses a raw TOML value string into a Go value.
func parseValue(s string) interface{} {
	s = strings.TrimSpace(s)

	// Quoted string
	if strings.HasPrefix(s, `"`) {
		return parseQuotedString(s)
	}

	// Array
	if strings.HasPrefix(s, "[") {
		return parseArray(s)
	}

	// Boolean
	if s == "true" {
		return true
	}
	if s == "false" {
		return false
	}

	// Number: try int, then float
	if i, err := strconv.Atoi(s); err == nil {
		return i
	}
	if f, err := strconv.ParseFloat(s, 64); err == nil {
		return f
	}

	// Bare string fallback
	return s
}

// parseQuotedString parses a double-quoted TOML string with basic escapes.
func parseQuotedString(s string) string {
	if len(s) < 2 {
		return s
	}
	// Strip surrounding quotes.
	inner := s[1 : len(s)-1]
	var buf strings.Builder
	i := 0
	for i < len(inner) {
		ch := inner[i]
		if ch == '\\' && i+1 < len(inner) {
			next := inner[i+1]
			switch next {
			case 'n':
				buf.WriteByte('\n')
			case 't':
				buf.WriteByte('\t')
			case 'r':
				buf.WriteByte('\r')
			case '\\':
				buf.WriteByte('\\')
			case '"':
				buf.WriteByte('"')
			default:
				buf.WriteByte('\\')
				buf.WriteByte(next)
			}
			i += 2
		} else {
			buf.WriteByte(ch)
			i++
		}
	}
	return buf.String()
}

// parseArray parses a TOML array value (possibly multi-line, already joined).
func parseArray(s string) []interface{} {
	// Strip outer brackets.
	s = strings.TrimSpace(s)
	if len(s) < 2 {
		return []interface{}{}
	}
	inner := s[1 : len(s)-1]
	inner = strings.TrimSpace(inner)

	if inner == "" {
		return []interface{}{}
	}

	// Split on commas, respecting nested brackets and quoted strings.
	items := splitArrayItems(inner)
	result := make([]interface{}, 0, len(items))
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		result = append(result, parseValue(item))
	}
	return result
}

// splitArrayItems splits comma-separated array items, respecting nesting and quotes.
func splitArrayItems(s string) []string {
	var items []string
	var buf strings.Builder
	depth := 0
	inStr := false

	for i := 0; i < len(s); i++ {
		ch := s[i]
		if inStr {
			if ch == '\\' && i+1 < len(s) {
				buf.WriteByte(ch)
				i++
				buf.WriteByte(s[i])
			} else if ch == '"' {
				inStr = false
				buf.WriteByte(ch)
			} else {
				buf.WriteByte(ch)
			}
			continue
		}
		switch ch {
		case '"':
			inStr = true
			buf.WriteByte(ch)
		case '[':
			depth++
			buf.WriteByte(ch)
		case ']':
			depth--
			buf.WriteByte(ch)
		case ',':
			if depth == 0 {
				items = append(items, buf.String())
				buf.Reset()
			} else {
				buf.WriteByte(ch)
			}
		default:
			buf.WriteByte(ch)
		}
	}
	if buf.Len() > 0 {
		items = append(items, buf.String())
	}
	return items
}

// stripComment removes a trailing # comment from a line, but only outside quoted strings.
func stripComment(line string) string {
	inStr := false
	for i := 0; i < len(line); i++ {
		ch := line[i]
		if inStr {
			if ch == '\\' {
				i++ // skip escaped char
			} else if ch == '"' {
				inStr = false
			}
			continue
		}
		switch ch {
		case '"':
			inStr = true
		case '#':
			return line[:i]
		}
	}
	return line
}

// isSpace reports whether r is ASCII whitespace. Kept for potential future use.
var _ = unicode.IsSpace
