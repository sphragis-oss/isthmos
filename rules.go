// SPDX-License-Identifier: Apache-2.0

package isthmos

import (
	"encoding/json"
	"log/slog"
	"os"
	"path"
)

type Rule struct {
	Tool      string   `json:"tool"`
	DropKeys  []string `json:"drop_keys"`
	KeepKeys  []string `json:"keep_keys,omitempty"`
	MaxItems  int      `json:"max_items,omitempty"`
	MaxStr    int      `json:"max_str,omitempty"`
	MaxLines  int      `json:"max_lines,omitempty"`
	KeepLast  int      `json:"keep_last,omitempty"`
	MinBytes  int      `json:"min_bytes,omitempty"`
	Dedup     bool     `json:"dedup,omitempty"`
	DropEmpty bool     `json:"drop_empty,omitempty"`
	Tabular   bool     `json:"tabular,omitempty"`
}

type Rules struct {
	Rules     []Rule `json:"rules"`
	keepShape bool
}

// KeepShape returns the rules without any edit that changes a payload's structure
func (rs Rules) KeepShape() Rules {
	out := Rules{keepShape: true}
	for _, r := range rs.Rules {
		r.DropKeys, r.KeepKeys, r.DropEmpty, r.Tabular = nil, nil, false, false
		out.Rules = append(out.Rules, r)
	}
	return out
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

// KeepFor merges allowlisted keys from every rule whose glob matches the tool name
func (rs Rules) KeepFor(tool string) map[string]bool {
	keep := map[string]bool{}
	for _, r := range rs.Rules {
		if ok, _ := path.Match(r.Tool, tool); ok {
			for _, k := range r.KeepKeys {
				keep[k] = true
			}
		}
	}
	return keep
}

// LimitsFor takes the strictest positive limit across matching rules
func (rs Rules) LimitsFor(tool string) Limits {
	lim := Limits{KeepShape: rs.keepShape}
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
		if r.MaxLines > 0 && (lim.MaxLines == 0 || r.MaxLines < lim.MaxLines) {
			lim.MaxLines = r.MaxLines
		}
		if r.KeepLast > lim.KeepLast {
			lim.KeepLast = r.KeepLast
		}
		if r.Dedup {
			lim.Dedup = true
		}
		if r.DropEmpty {
			lim.DropEmpty = true
		}
		if r.Tabular {
			lim.Tabular = true
		}
	}
	return lim
}

// eligible keeps only rules whose min_bytes gate the payload passes
func (rs Rules) eligible(size int) Rules {
	out := Rules{keepShape: rs.keepShape}
	for _, r := range rs.Rules {
		if r.MinBytes == 0 || size >= r.MinBytes {
			out.Rules = append(out.Rules, r)
		}
	}
	return out
}
