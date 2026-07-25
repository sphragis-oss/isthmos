// SPDX-License-Identifier: Apache-2.0

package main

import (
	"strings"
	"testing"

	"github.com/sphragis-oss/isthmos"
)

func TestRedactKeepsBuiltinNames(t *testing.T) {
	in := []isthmos.ToolStat{{Tool: "Read", Calls: 2, InBytes: 100, OutBytes: 40}}
	got := redact(in)
	if len(got) != 1 || got[0].Tool != "Read" {
		t.Fatalf("built-in name should survive: %+v", got)
	}
	if got[0].InBytes != 100 || got[0].OutBytes != 40 {
		t.Fatalf("bytes must be untouched: %+v", got[0])
	}
}

func TestRedactFoldsToolsOfOneServer(t *testing.T) {
	in := []isthmos.ToolStat{
		{Tool: "mcp__acmecorp__search", Calls: 1, InBytes: 300, OutBytes: 100, Reveals: 1},
		{Tool: "mcp__acmecorp__create", Calls: 2, InBytes: 200, OutBytes: 100},
	}
	got := redact(in)
	if len(got) != 1 {
		t.Fatalf("one server should be one row: %+v", got)
	}
	if got[0].Calls != 3 || got[0].InBytes != 500 || got[0].OutBytes != 200 || got[0].Reveals != 1 {
		t.Fatalf("totals not summed: %+v", got[0])
	}
}

func TestRedactSeparatesServers(t *testing.T) {
	in := []isthmos.ToolStat{
		{Tool: "mcp__acmecorp__search", InBytes: 300, OutBytes: 100},
		{Tool: "mcp__othercorp__list", InBytes: 200, OutBytes: 150},
		{Tool: "some_custom_tool", InBytes: 100, OutBytes: 90},
	}
	got := redact(in)
	if len(got) != 3 {
		t.Fatalf("distinct sources should stay distinct: %+v", got)
	}
	names := map[string]bool{}
	for _, s := range got {
		names[s.Tool] = true
	}
	if len(names) != 3 {
		t.Fatalf("placeholders collided: %v", names)
	}
}

// the whole point of -share: no private name may survive redaction
func TestRedactLeaksNoOriginalName(t *testing.T) {
	secrets := []string{"acmecorp", "internal-jira", "some_custom_tool"}
	in := []isthmos.ToolStat{
		{Tool: "mcp__acmecorp__search_tickets", InBytes: 300, OutBytes: 100},
		{Tool: "mcp__internal-jira__jql", InBytes: 200, OutBytes: 150},
		{Tool: "some_custom_tool", InBytes: 100, OutBytes: 90},
		{Tool: "Read", InBytes: 100, OutBytes: 90},
	}
	var out strings.Builder
	for _, s := range redact(in) {
		out.WriteString(s.Tool + "\n")
	}
	for _, secret := range secrets {
		if strings.Contains(out.String(), secret) {
			t.Fatalf("redaction leaked %q:\n%s", secret, out.String())
		}
	}
	if !strings.Contains(out.String(), "Read") {
		t.Fatal("built-in name should not have been redacted")
	}
}

func TestShareNameIsStable(t *testing.T) {
	alias := map[string]string{}
	a := shareName("mcp__acmecorp__search", alias)
	b := shareName("mcp__acmecorp__other", alias)
	if a != b {
		t.Fatalf("same server got two placeholders: %s vs %s", a, b)
	}
	if c := shareName("mcp__acmecorp__search", alias); c != a {
		t.Fatalf("placeholder not stable across calls: %s vs %s", c, a)
	}
}
