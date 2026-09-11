package endpoints

import (
	"fmt"
	"sort"
	"strings"
)

// RenderCypher interpolates $param placeholders in a Cypher query with their
// actual values, for logging/debugging purposes only.
func RenderCypher(query string, params map[string]any) string {
	keys := make([]string, 0, len(params))
	for k := range params {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool { return len(keys[i]) > len(keys[j]) })

	rendered := query
	for _, k := range keys {
		rendered = strings.ReplaceAll(rendered, "$"+k, RenderCypherValue(params[k]))
	}
	return flatten(rendered)
}

// flatten collapses newlines, tabs, and redundant surrounding whitespace into
// a single readable line while preserving spaces inside string literals.
func flatten(s string) string {
	lines := strings.Split(s, "\n")
	out := make([]string, 0, len(lines))
	for _, l := range lines {
		if trimmed := strings.TrimSpace(l); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return strings.Join(out, " ")
}

func RenderCypherValue(v any) string {
	switch val := v.(type) {
	case string:
		return "'" + val + "'"
	case []string:
		parts := make([]string, len(val))
		for i, s := range val {
			parts[i] = "'" + s + "'"
		}
		return "[" + strings.Join(parts, ", ") + "]"
	case []any:
		parts := make([]string, len(val))
		for i, e := range val {
			parts[i] = RenderCypherValue(e)
		}
		return "[" + strings.Join(parts, ", ") + "]"
	default:
		return fmt.Sprint(v)
	}
}

// renderSQL interpolates ? placeholders in a SQL query with their actual
// values, for logging/debugging purposes only.
func RenderSQL(query string, args []any) string {
	rendered := query
	for _, a := range args {
		rendered = strings.Replace(rendered, "?", RenderSQLValue(a), 1)
	}
	return flatten(rendered)
}

func RenderSQLValue(v any) string {
	switch val := v.(type) {
	case string:
		return "'" + val + "'"
	default:
		return fmt.Sprint(v)
	}
}
