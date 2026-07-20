// SPDX-License-Identifier: Apache-2.0

package isthmos

import (
	"encoding/json"
	"log/slog"
	"os"
	"path"
)

type Rule struct {
	Tool     string   `json:"tool"`
	DropKeys []string `json:"drop_keys"`
	MaxItems int      `json:"max_items,omitempty"`
	MaxStr   int      `json:"max_str,omitempty"`
	KeepLast int      `json:"keep_last,omitempty"`
	MinBytes int      `json:"min_bytes,omitempty"`
}

type Rules struct {
	Rules []Rule `json:"rules"`
}

// LoadRules is fail-open: missing or bad config means no rules
func LoadRules(p string) Rules {
	var rs Rules
	b, err := os.ReadFile(p)
	if err != nil {
		return rs
	}
	if err := json.Unmarshal(b, &rs); err != nil {
		slog.Error("parse rules", "path", p, "err", err)
	}
	return rs
}

// DropFor merges drop keys from every rule whose glob matches the tool name
func (rs Rules) DropFor(tool string) map[string]bool {
	drop := map[string]bool{}
	for _, r := range rs.Rules {
		if ok, _ := path.Match(r.Tool, tool); ok {
			for _, k := range r.DropKeys {
				drop[k] = true
			}
		}
	}
	return drop
}

// LimitsFor takes the strictest positive limit across matching rules
func (rs Rules) LimitsFor(tool string) Limits {
	var lim Limits
	for _, r := range rs.Rules {
		if ok, _ := path.Match(r.Tool, tool); !ok {
			continue
		}
		if r.MaxItems > 0 && (lim.MaxItems == 0 || r.MaxItems < lim.MaxItems) {
			lim.MaxItems = r.MaxItems
		}
		if r.MaxStr > 0 && (lim.MaxStr == 0 || r.MaxStr < lim.MaxStr) {
			lim.MaxStr = r.MaxStr
		}
		if r.KeepLast > lim.KeepLast {
			lim.KeepLast = r.KeepLast
		}
	}
	return lim
}

// eligible keeps only rules whose min_bytes gate the payload passes
func (rs Rules) eligible(size int) Rules {
	var out Rules
	for _, r := range rs.Rules {
		if r.MinBytes == 0 || size >= r.MinBytes {
			out.Rules = append(out.Rules, r)
		}
	}
	return out
}
