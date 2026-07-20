// SPDX-License-Identifier: Apache-2.0

package isthmos

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestPruneDropsKeysRecursively(t *testing.T) {
	in := json.RawMessage(`{"a":1,"node_id":"x","items":[{"node_id":"y","b":2}]}`)
	out, err := PruneJSON(in, map[string]bool{"node_id": true}, Limits{})
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
	out, err := PruneJSON(in, map[string]bool{"self": true}, Limits{})
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

func TestMaxItemsTruncatesWithMarker(t *testing.T) {
	in := json.RawMessage(`{"items":["a","b","c","d","e"]}`)
	out, err := PruneJSON(in, nil, Limits{MaxItems: 2})
	if err != nil {
		t.Fatal(err)
	}
	var got struct{ Items []any }
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Items) != 3 {
		t.Fatalf("expected 2 kept + marker, got %v", got.Items)
	}
	marker, ok := got.Items[2].(string)
	if !ok || !strings.Contains(marker, "3 of 5 items truncated") {
		t.Fatalf("bad marker: %v", got.Items[2])
	}
}

func TestMaxStrTruncatesAtRuneBoundary(t *testing.T) {
	long := strings.Repeat("α", 100)
	in, _ := json.Marshal(map[string]string{"log": long})
	out, err := PruneJSON(in, nil, Limits{MaxStr: 51})
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]string
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("truncation produced invalid JSON or invalid UTF-8: %v", err)
	}
	s := got["log"]
	if !strings.Contains(s, "bytes truncated]") {
		t.Fatalf("missing marker: %q", s)
	}
	if strings.ContainsRune(s, '�') {
		t.Fatalf("split a rune: %q", s)
	}
}

func TestShortValuesUntouchedByLimits(t *testing.T) {
	in := json.RawMessage(`{"items":["a"],"s":"ok"}`)
	rs := Rules{Rules: []Rule{{Tool: "*", MaxItems: 10, MaxStr: 100}}}
	out, changed := Apply(rs, "anything", in)
	if changed {
		t.Fatalf("expected passthrough, got %s", out)
	}
}

func TestLimitsForTakesStrictest(t *testing.T) {
	rs := Rules{Rules: []Rule{
		{Tool: "mcp__*", MaxItems: 50, MaxStr: 8000},
		{Tool: "mcp__github__*", MaxItems: 20},
	}}
	lim := rs.LimitsFor("mcp__github__search_repos")
	if lim.MaxItems != 20 || lim.MaxStr != 8000 {
		t.Fatalf("bad merge: %+v", lim)
	}
	if got := rs.LimitsFor("Bash"); !got.empty() {
		t.Fatalf("Bash should have no limits, got %+v", got)
	}
}
