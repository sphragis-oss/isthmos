// SPDX-License-Identifier: Apache-2.0

package isthmos

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
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

func TestKeepShapeStripsStructuralEdits(t *testing.T) {
	rs := Rules{Rules: []Rule{{Tool: "*", DropKeys: []string{"stderr"}, DropEmpty: true, MaxItems: 2, MaxLines: 3}}}.KeepShape()
	line := strings.Repeat("x", 40)
	text := strings.Repeat(line+`\n`, 20)
	names := strings.Repeat(`"`+line+`",`, 20) + `"z"`
	in := json.RawMessage(`{"stdout":"` + text + `","stderr":"","objs":[{"a":1},{"a":2},{"a":3}],"names":[` + names + `]}`)
	out, changed := Apply(rs, "Bash", in)
	if !changed {
		t.Fatalf("shape-preserving limits must still apply: %s", out)
	}
	var got map[string]json.RawMessage
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatal(err)
	}
	if len(got) != 4 || string(got["stderr"]) != `""` {
		t.Fatalf("keys must survive: %s", out)
	}
	if string(got["objs"]) != `[{"a":1},{"a":2},{"a":3}]` {
		t.Fatalf("an array of objects cannot take a string marker: %s", got["objs"])
	}
	if !strings.Contains(string(got["names"]), "items truncated") || !strings.Contains(string(got["stdout"]), "lines truncated") {
		t.Fatalf("string arrays and text must still be capped: %s", out)
	}
}
