package isthmos

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDropForGlob(t *testing.T) {
	rs := Rules{Rules: []Rule{
		{Tool: "mcp__github__*", DropKeys: []string{"node_id"}},
		{Tool: "mcp__*", DropKeys: []string{"self"}},
	}}
	drop := rs.DropFor("mcp__github__search_repos")
	if !drop["node_id"] || !drop["self"] {
		t.Fatalf("expected merged keys, got %v", drop)
	}
	if len(rs.DropFor("Bash")) != 0 {
		t.Fatal("Bash should match nothing")
	}
}

func TestLoadRulesFailOpen(t *testing.T) {
	if n := len(LoadRules("/nonexistent/rules.json").Rules); n != 0 {
		t.Fatalf("expected no rules, got %d", n)
	}
	p := filepath.Join(t.TempDir(), "rules.json")
	os.WriteFile(p, []byte(`{"rules":[{"tool":"mcp__*","drop_keys":["self"]}]}`), 0o644)
	if n := len(LoadRules(p).Rules); n != 1 {
		t.Fatalf("expected 1 rule, got %d", n)
	}
}
