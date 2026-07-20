// SPDX-License-Identifier: Apache-2.0

package isthmos

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestPruneDropsKeysRecursively(t *testing.T) {
	in := json.RawMessage(`{"a":1,"node_id":"x","items":[{"node_id":"y","b":2}]}`)
	out, err := PruneJSON(in, map[string]bool{"node_id": true})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(out), "node_id") {
		t.Fatalf("node_id not dropped: %s", out)
	}
	if !strings.Contains(string(out), `"b":2`) {
		t.Fatalf("kept key lost: %s", out)
	}
}

func TestPruneJSONStringWrapped(t *testing.T) {
	in := json.RawMessage(`"{\"a\":1,\"self\":\"http://x\"}"`)
	out, err := PruneJSON(in, map[string]bool{"self": true})
	if err != nil {
		t.Fatal(err)
	}
	var s string
	if err := json.Unmarshal(out, &s); err != nil {
		t.Fatalf("output no longer a JSON string: %s", out)
	}
	if strings.Contains(s, "self") {
		t.Fatalf("self not dropped: %s", s)
	}
}

func TestApplyNoRuleNoChange(t *testing.T) {
	in := json.RawMessage(`{"node_id":"x"}`)
	out, changed := Apply(Rules{}, "mcp__github__search", in)
	if changed || string(out) != string(in) {
		t.Fatalf("expected passthrough, got %s", out)
	}
}

func TestApplyPassthroughOnNonJSON(t *testing.T) {
	rs := Rules{Rules: []Rule{{Tool: "mcp__*", DropKeys: []string{"self"}}}}
	in := json.RawMessage(`"plain text result"`)
	out, changed := Apply(rs, "mcp__x__y", in)
	if changed || string(out) != string(in) {
		t.Fatalf("expected passthrough, got %s", out)
	}
}

func TestApplyShrinks(t *testing.T) {
	rs := Rules{Rules: []Rule{{Tool: "mcp__atlassian__*", DropKeys: []string{"avatarUrls", "self"}}}}
	in := json.RawMessage(`{"key":"SRE-1","self":"http://x/rest/api/2/issue/1","avatarUrls":{"48x48":"http://y"}}`)
	out, changed := Apply(rs, "mcp__atlassian__searchJiraIssuesUsingJql", in)
	if !changed {
		t.Fatal("expected change")
	}
	if len(out) >= len(in) || strings.Contains(string(out), "avatarUrls") {
		t.Fatalf("not shrunk: %s", out)
	}
}
