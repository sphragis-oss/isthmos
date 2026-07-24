// SPDX-License-Identifier: Apache-2.0

package isthmos

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"
)

func testStore(t *testing.T) *Store {
	t.Helper()
	st, err := OpenStore(filepath.Join(t.TempDir(), "store"), time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	return st
}

func TestStoreRoundtrip(t *testing.T) {
	st := testStore(t)
	id := newID()
	payload := []byte(`{"secret":"original payload"}`)
	if err := st.Save(id, payload, "mcp__x__y"); err != nil {
		t.Fatal(err)
	}
	got, err := st.Load(id)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, payload) {
		t.Fatalf("roundtrip mismatch: %s", got)
	}
}

func TestStoreEncryptsAtRest(t *testing.T) {
	st := testStore(t)
	id := newID()
	payload := []byte("plaintext-marker-abcdef")
	if err := st.Save(id, payload, "mcp__x__y"); err != nil {
		t.Fatal(err)
	}
	onDisk, err := os.ReadFile(filepath.Join(st.dir, id+".bin"))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(onDisk, []byte("plaintext-marker")) {
		t.Fatal("payload stored in plaintext")
	}
}

func TestStoreRejectsBadIDs(t *testing.T) {
	st := testStore(t)
	for _, id := range []string{"", "short", "../../etc/passwd!", "ZZZZZZZZZZZZZZZZ", strings.Repeat("a", 17)} {
		if _, err := st.Load(id); err == nil {
			t.Fatalf("id %q should be rejected", id)
		}
	}
}

func TestStoreGCExpiresOldEntries(t *testing.T) {
	st := testStore(t)
	old, fresh := newID(), newID()
	if err := st.Save(old, []byte("old"), "mcp__x__y"); err != nil {
		t.Fatal(err)
	}
	stale := time.Now().Add(-2 * time.Hour)
	for _, ext := range []string{".bin", ".meta"} {
		if err := os.Chtimes(filepath.Join(st.dir, old+ext), stale, stale); err != nil {
			t.Fatal(err)
		}
	}
	if err := st.Save(fresh, []byte("fresh"), "mcp__x__y"); err != nil {
		t.Fatal(err)
	}
	if _, err := st.Load(old); err == nil {
		t.Fatal("expired entry survived gc")
	}
	if st.Tool(old) != "" {
		t.Fatal("expired meta survived gc")
	}
	if _, err := st.Load(fresh); err != nil {
		t.Fatalf("fresh entry lost: %v", err)
	}
}

func TestStoreToolAttribution(t *testing.T) {
	st := testStore(t)
	id := newID()
	if err := st.Save(id, []byte("payload"), "mcp__atlassian__search"); err != nil {
		t.Fatal(err)
	}
	if got := st.Tool(id); got != "mcp__atlassian__search" {
		t.Fatalf("expected tool attribution, got %q", got)
	}
	if got := st.Tool(newID()); got != "" {
		t.Fatalf("missing meta should mean empty tool, got %q", got)
	}
	if got := st.Tool("../../etc/passwd!"); got != "" {
		t.Fatalf("bad id should mean empty tool, got %q", got)
	}
}

func bigItemsPayload(t *testing.T, n int) json.RawMessage {
	t.Helper()
	items := make([]map[string]string, n)
	for i := range items {
		items[i] = map[string]string{"name": strings.Repeat("x", 40), "url": "https://example.com/very/long/path"}
	}
	b, err := json.Marshal(map[string]any{"items": items})
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestApplyWithStoreMarkersAreRevealable(t *testing.T) {
	st := testStore(t)
	rs := Rules{Rules: []Rule{{Tool: "*", MaxItems: 2}}}
	in := bigItemsPayload(t, 8)
	out, changed := ApplyWithStore(rs, "anything", in, st)
	if !changed {
		t.Fatal("expected change")
	}
	m := regexp.MustCompile(`isthmos reveal ([0-9a-f]{16})`).FindStringSubmatch(string(out))
	if m == nil {
		t.Fatalf("marker has no reveal hint: %s", out)
	}
	orig, err := st.Load(m[1])
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(orig, in) {
		t.Fatalf("stored payload is not the original: %s", orig)
	}
}

func TestApplyWithStoreDropKeysOnlyStoresNothing(t *testing.T) {
	st := testStore(t)
	rs := Rules{Rules: []Rule{{Tool: "*", DropKeys: []string{"noise"}}}}
	in := json.RawMessage(`{"noise":"xxxxxxxxxxxxxxxx","keep":"y"}`)
	out, changed := ApplyWithStore(rs, "anything", in, st)
	if !changed {
		t.Fatal("expected change")
	}
	if strings.Contains(string(out), "reveal") {
		t.Fatalf("drop-keys-only output should have no reveal hint: %s", out)
	}
	entries, _ := os.ReadDir(st.dir)
	for _, e := range entries {
		if filepath.Ext(e.Name()) == ".bin" {
			t.Fatal("nothing should be stored for a lossless-by-rules change")
		}
	}
}

func TestApplyNilStoreHasNoHint(t *testing.T) {
	rs := Rules{Rules: []Rule{{Tool: "*", MaxItems: 2}}}
	in := bigItemsPayload(t, 8)
	out, changed := Apply(rs, "anything", in)
	if !changed {
		t.Fatal("expected change")
	}
	if strings.Contains(string(out), "reveal") {
		t.Fatalf("nil store must not produce reveal hints: %s", out)
	}
}
