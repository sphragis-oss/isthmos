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

func TestKeepLastPreservesTail(t *testing.T) {
	in := json.RawMessage(`{"items":["a","b","c","d","e"]}`)
	out, err := PruneJSON(in, nil, Limits{MaxItems: 3, KeepLast: 1})
	if err != nil {
		t.Fatal(err)
	}
	var got struct{ Items []any }
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatal(err)
	}
	want := []any{"a", "b", "e"}
	if len(got.Items) != 4 {
		t.Fatalf("expected 3 kept + marker, got %v", got.Items)
	}
	for i, w := range want {
		if got.Items[i] != w {
			t.Fatalf("expected %v at %d, got %v", w, i, got.Items)
		}
	}
}

func TestErrorItemsAlwaysKept(t *testing.T) {
	in := json.RawMessage(`{"items":[{"n":1},{"n":2},{"n":3,"status":"failed"},{"n":4,"error":"boom"},{"n":5},{"n":6}]}`)
	out, err := PruneJSON(in, nil, Limits{MaxItems: 2})
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	if !strings.Contains(s, `"status":"failed"`) || !strings.Contains(s, `"error":"boom"`) {
		t.Fatalf("error items dropped: %s", s)
	}
	if !strings.Contains(s, "2 of 6 items truncated") {
		t.Fatalf("bad marker: %s", s)
	}
}

func TestNullErrorFieldNotTreatedAsError(t *testing.T) {
	in := json.RawMessage(`{"items":[{"n":1},{"n":2},{"n":3,"error":null},{"n":4,"error":0}]}`)
	out, err := PruneJSON(in, nil, Limits{MaxItems: 2})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), "2 of 4 items truncated") {
		t.Fatalf("null/zero error fields should not pin items: %s", out)
	}
}

func TestMinBytesGatesRule(t *testing.T) {
	rs := Rules{Rules: []Rule{{Tool: "*", DropKeys: []string{"noise"}, MinBytes: 1000}}}
	small := json.RawMessage(`{"noise":"x","keep":"y"}`)
	if out, changed := Apply(rs, "anything", small); changed {
		t.Fatalf("small payload should pass through, got %s", out)
	}
	big, _ := json.Marshal(map[string]string{"noise": strings.Repeat("x", 2000), "keep": "y"})
	if _, changed := Apply(rs, "anything", big); !changed {
		t.Fatal("large payload should be pruned")
	}
}

func TestLimitsForMergesKeepLast(t *testing.T) {
	rs := Rules{Rules: []Rule{
		{Tool: "mcp__*", MaxItems: 50, KeepLast: 2},
		{Tool: "mcp__github__*", KeepLast: 5},
	}}
	if lim := rs.LimitsFor("mcp__github__list_issues"); lim.KeepLast != 5 {
		t.Fatalf("expected KeepLast 5, got %+v", lim)
	}
}

func TestLargeIntegersSurviveRoundTrip(t *testing.T) {
	in := json.RawMessage(`{"id":1234567890123456789,"ratio":1.0,"noise":"xxxxxxxxxxxxxxxx"}`)
	out, err := PruneJSON(in, map[string]bool{"noise": true}, Limits{})
	if err != nil {
		t.Fatal(err)
	}
	if string(out) != `{"id":1234567890123456789,"ratio":1.0}` {
		t.Fatalf("numbers were rewritten: %s", out)
	}
}

func TestHTMLCharsAreNotEscaped(t *testing.T) {
	rs := Rules{Rules: []Rule{{Tool: "t", DropKeys: []string{"noise"}}}}
	in := json.RawMessage(`{"noise":"xxxxxxxx","html":"<<<<<<>>>>>>&&"}`)
	out, changed := Apply(rs, "t", in)
	if !changed || string(out) != `{"html":"<<<<<<>>>>>>&&"}` {
		t.Fatalf("escaping ate the saving: %s", out)
	}
}

func TestErrorPinsAreBudgeted(t *testing.T) {
	items := make([]string, 200)
	for i := range items {
		items[i] = `{"status":"failed"}`
	}
	in := json.RawMessage("[" + strings.Join(items, ",") + "]")
	out, err := PruneJSON(in, nil, Limits{MaxItems: 5})
	if err != nil {
		t.Fatal(err)
	}
	if n := strings.Count(string(out), "failed"); n != 10 {
		t.Fatalf("kept %d error items, want 5 plus 5 pinned", n)
	}
}

func TestDottedDropKeyIsPathScoped(t *testing.T) {
	in := json.RawMessage(`{"url":"keep","user":{"url":"drop","login":"a"},"items":[{"user":{"url":"drop"}}]}`)
	out, err := PruneJSON(in, map[string]bool{"user.url": true}, Limits{})
	if err != nil {
		t.Fatal(err)
	}
	if string(out) != `{"items":[{"user":{}}],"url":"keep","user":{"login":"a"}}` {
		t.Fatalf("got %s", out)
	}
}

func TestKeepKeysIsAnAllowlist(t *testing.T) {
	rs := Rules{Rules: []Rule{{Tool: "t", KeepKeys: []string{"key", "fields"}}}}
	in := json.RawMessage(`{"total":2,"issues":[{"key":"A-1","id":"1","meta":{"x":1},"fields":{"summary":"s","n":3}},{"key":"A-2","id":"2","tags":["a"]}]}`)
	out, changed := Apply(rs, "t", in)
	if !changed || string(out) != `{"issues":[{"fields":{"n":3,"summary":"s"},"key":"A-1"},{"key":"A-2"}]}` {
		t.Fatalf("got %s", out)
	}
}

func TestDropEmptyRemovesBlankValues(t *testing.T) {
	in := json.RawMessage(`{"a":null,"b":"","c":[],"d":{},"e":0,"f":false,"g":{"h":null}}`)
	out, err := PruneJSON(in, nil, Limits{DropEmpty: true})
	if err != nil {
		t.Fatal(err)
	}
	if string(out) != `{"e":0,"f":false}` {
		t.Fatalf("got %s", out)
	}
}

func TestTabularCollapsesSameShapedObjects(t *testing.T) {
	in := json.RawMessage(`[{"id":1,"name":"a"},{"id":2,"name":"b"},{"id":3,"name":"c"}]`)
	out, err := PruneJSON(in, nil, Limits{Tabular: true})
	if err != nil {
		t.Fatal(err)
	}
	if string(out) != `{"isthmos_table":{"cols":["id","name"],"rows":[[1,"a"],[2,"b"],[3,"c"]]}}` {
		t.Fatalf("got %s", out)
	}
}

func TestTabularLeavesMixedShapesAlone(t *testing.T) {
	in := json.RawMessage(`[{"id":1,"name":"a"},{"id":2},{"id":3,"name":"c"}]`)
	out, err := PruneJSON(in, nil, Limits{Tabular: true})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(out), "isthmos_table") {
		t.Fatalf("mixed shapes must stay an array: %s", out)
	}
}

func TestTabularKeepsTruncationMarker(t *testing.T) {
	in := json.RawMessage(`[{"id":1,"n":"a"},{"id":2,"n":"b"},{"id":3,"n":"c"},{"id":4,"n":"d"},{"id":5,"n":"e"}]`)
	out, err := PruneJSON(in, nil, Limits{Tabular: true, MaxItems: 3})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), `"note":"[isthmos: 2 of 5 items truncated]"`) || !strings.Contains(string(out), `"rows":[[1,"a"],[2,"b"],[3,"c"]]`) {
		t.Fatalf("got %s", out)
	}
}
