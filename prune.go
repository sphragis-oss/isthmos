// SPDX-License-Identifier: Apache-2.0

package isthmos

import (
	"encoding/json"
	"fmt"
	"unicode/utf8"
)

type Limits struct {
	MaxItems int
	MaxStr   int
}

func (l Limits) empty() bool { return l.MaxItems == 0 && l.MaxStr == 0 }

// Apply returns the possibly pruned output and whether it shrank
func Apply(rs Rules, tool string, output json.RawMessage) (json.RawMessage, bool) {
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
		return capItems(t, lim.MaxItems)
	case string:
		return capStr(t, lim.MaxStr)
	default:
		return v
	}
}

// capItems truncates a long array, replacing the tail with an explicit marker
func capItems(t []any, maxItems int) []any {
	if maxItems <= 0 || len(t) <= maxItems {
		return t
	}
	cut := len(t) - maxItems
	return append(t[:maxItems], fmt.Sprintf("[isthmos: %d of %d items truncated]", cut, len(t)))
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
