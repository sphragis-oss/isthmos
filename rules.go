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
