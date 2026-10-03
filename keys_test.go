// SPDX-License-Identifier: Apache-2.0

package isthmos

import (
	"bytes"
	"encoding/json"
	"testing"
	"time"
)

func TestKeyBytesRanksHeavyKeys(t *testing.T) {
	kb := KeyBytes(json.RawMessage(`{"items":[{"id":1,"avatar_url":"https://example.com/a/very/long/url"},{"id":2,"avatar_url":"https://example.com/another/long/url"}]}`))
	if kb["avatar_url"] <= kb["id"] {
		t.Fatalf("avatar_url must outweigh id: %v", kb)
	}
	if kb["items"] <= kb["avatar_url"] {
		t.Fatalf("a parent key includes its children: %v", kb)
	}
}

func TestKeyBytesNonJSONIsEmpty(t *testing.T) {
	if kb := KeyBytes(json.RawMessage("plain text")); len(kb) != 0 {
		t.Fatalf("text has no keys: %v", kb)
	}
}

func TestTopKeysKeepsHeaviest(t *testing.T) {
	got := TopKeys(map[string]int64{"a": 1, "b": 30, "c": 20}, 2)
	if len(got) != 2 || got["b"] != 30 || got["c"] != 20 {
		t.Fatalf("got %v", got)
	}
}

func TestAggregateKeysFiltersByToolGlob(t *testing.T) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	now := time.Now().UTC()
	for _, m := range []KeyMeasure{
		{TS: now, Tool: "mcp__github__a", InBytes: 100, Keys: map[string]int64{"url": 40, "id": 5}},
		{TS: now, Tool: "mcp__github__b", InBytes: 100, Keys: map[string]int64{"url": 60}},
		{TS: now, Tool: "Bash", InBytes: 900, Keys: map[string]int64{"stdout": 800}},
	} {
		if err := enc.Encode(m); err != nil {
			t.Fatal(err)
		}
	}
	stats, total := AggregateKeys(&buf, time.Time{}, "mcp__github__*")
	if total != 200 {
		t.Fatalf("total = %d, want 200", total)
	}
	if len(stats) != 2 || stats[0].Key != "url" || stats[0].Bytes != 100 || stats[0].Calls != 2 {
		t.Fatalf("got %+v", stats)
	}
}
