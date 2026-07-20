// SPDX-License-Identifier: Apache-2.0

package isthmos

import "encoding/json"

// Apply returns the possibly pruned output and whether it shrank
func Apply(rs Rules, tool string, output json.RawMessage) (json.RawMessage, bool) {
	drop := rs.DropFor(tool)
	if len(drop) == 0 || len(output) == 0 {
		return output, false
	}
	pruned, err := PruneJSON(output, drop)
	if err != nil || len(pruned) >= len(output) {
		return output, false
	}
	return pruned, true
}

// PruneJSON drops keys recursively, unwrapping a JSON-encoded string payload
func PruneJSON(raw json.RawMessage, drop map[string]bool) ([]byte, error) {
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return nil, err
	}
	if s, ok := v.(string); ok {
		var inner any
		if err := json.Unmarshal([]byte(s), &inner); err != nil {
			return nil, err
		}
		b, err := json.Marshal(prune(inner, drop))
		if err != nil {
			return nil, err
		}
		return json.Marshal(string(b))
	}
	return json.Marshal(prune(v, drop))
}

func prune(v any, drop map[string]bool) any {
	switch t := v.(type) {
	case map[string]any:
		for k, val := range t {
			if drop[k] {
				delete(t, k)
				continue
			}
			t[k] = prune(val, drop)
		}
		return t
	case []any:
		for i, val := range t {
			t[i] = prune(val, drop)
		}
		return t
	default:
		return v
	}
}
