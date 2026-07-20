// SPDX-License-Identifier: Apache-2.0

package isthmos

import (
	"encoding/json"
	"fmt"
	"strings"
	"unicode/utf8"
)

type Limits struct {
	MaxItems int
	MaxStr   int
	KeepLast int
}

func (l Limits) empty() bool { return l.MaxItems == 0 && l.MaxStr == 0 }

// Apply returns the possibly pruned output and whether it shrank
func Apply(rs Rules, tool string, output json.RawMessage) (json.RawMessage, bool) {
	rs = rs.eligible(len(output))
	drop := rs.DropFor(tool)
	lim := rs.LimitsFor(tool)
	if (len(drop) == 0 && lim.empty()) || len(output) == 0 {
		return output, false
	}
	pruned, err := PruneJSON(output, drop, lim)
	if err != nil || len(pruned) >= len(output) {
		return output, false
	}
	return pruned, true
}

// PruneJSON drops keys and applies limits recursively, unwrapping a JSON-encoded string payload
func PruneJSON(raw json.RawMessage, drop map[string]bool, lim Limits) ([]byte, error) {
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return nil, err
	}
	if s, ok := v.(string); ok {
		var inner any
		if err := json.Unmarshal([]byte(s), &inner); err != nil {
			return nil, err
		}
		b, err := json.Marshal(prune(inner, drop, lim))
		if err != nil {
			return nil, err
		}
		return json.Marshal(string(b))
	}
	return json.Marshal(prune(v, drop, lim))
}

func prune(v any, drop map[string]bool, lim Limits) any {
	switch t := v.(type) {
	case map[string]any:
		for k, val := range t {
			if drop[k] {
				delete(t, k)
				continue
			}
			t[k] = prune(val, drop, lim)
		}
		return t
	case []any:
		for i, val := range t {
			t[i] = prune(val, drop, lim)
		}
		return capItems(t, lim)
	case string:
		return capStr(t, lim.MaxStr)
	default:
		return v
	}
}

// capItems keeps head, tail, and error-looking items, replacing the rest with a marker
func capItems(t []any, lim Limits) []any {
	if lim.MaxItems <= 0 || len(t) <= lim.MaxItems {
		return t
	}
	keepLast := lim.KeepLast
	if keepLast >= lim.MaxItems {
		keepLast = lim.MaxItems - 1
	}
	keep := make([]bool, len(t))
	for i := 0; i < lim.MaxItems-keepLast; i++ {
		keep[i] = true
	}
	for i := len(t) - keepLast; i < len(t); i++ {
		keep[i] = true
	}
	for i, v := range t {
		if !keep[i] && looksLikeError(v) {
			keep[i] = true
		}
	}
	out := make([]any, 0, lim.MaxItems+1)
	dropped := 0
	for i, v := range t {
		if keep[i] {
			out = append(out, v)
		} else {
			dropped++
		}
	}
	if dropped == 0 {
		return t
	}
	return append(out, fmt.Sprintf("[isthmos: %d of %d items truncated]", dropped, len(t)))
}

var errStates = map[string]bool{
	"error": true, "errors": true, "failed": true, "failure": true,
	"fatal": true, "critical": true, "unhealthy": true, "timeout": true,
}

// looksLikeError flags items truncation must never drop
func looksLikeError(v any) bool {
	m, ok := v.(map[string]any)
	if !ok {
		return false
	}
	for k, val := range m {
		switch strings.ToLower(k) {
		case "error", "errors", "err", "exception":
			if truthy(val) {
				return true
			}
		case "status", "state", "level", "severity", "result", "conclusion", "outcome":
			if s, ok := val.(string); ok && errStates[strings.ToLower(s)] {
				return true
			}
		}
	}
	return false
}

func truthy(v any) bool {
	switch t := v.(type) {
	case nil:
		return false
	case bool:
		return t
	case string:
		return t != "" && strings.ToLower(t) != "null"
	case float64:
		return t != 0
	case []any:
		return len(t) > 0
	case map[string]any:
		return len(t) > 0
	default:
		return true
	}
}

// capStr truncates a long string at a rune boundary, appending an explicit marker
func capStr(s string, maxStr int) string {
	if maxStr <= 0 || len(s) <= maxStr {
		return s
	}
	i := maxStr
	for i > 0 && !utf8.RuneStart(s[i]) {
		i--
	}
	return fmt.Sprintf("%s...[isthmos: %d bytes truncated]", s[:i], len(s)-i)
}
