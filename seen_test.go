// SPDX-License-Identifier: Apache-2.0

package isthmos

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"
)

func bigPayload(lines int) string {
	return strings.Repeat("some line of file content here\n", lines)
}

func TestSeenRepeatIsAHit(t *testing.T) {
	s := OpenSeen(t.TempDir(), "session-a", time.Hour)
	p := bigPayload(200)
	if _, ok := s.Check(p); ok {
		t.Fatal("first sight must not be a hit")
	}
	e, ok := s.Check(p)
	if !ok {
		t.Fatal("second sight must be a hit")
	}
	if e.Lines != 200 {
		t.Fatalf("lines = %d, want 200", e.Lines)
	}
}

func TestSeenCountsLinesLikeTheTextPath(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want int
	}{
		{"a\nb\nc", 3},
		{"a\nb\nc\n", 3},
		{"a", 1},
		{"", 1},
	} {
		if got := countLines(tc.in); got != tc.want {
			t.Errorf("countLines(%q) = %d, want %d", tc.in, got, tc.want)
		}
	}
}

func TestSeenIgnoresSmallPayloads(t *testing.T) {
	s := OpenSeen(t.TempDir(), "session-a", time.Hour)
	p := strings.Repeat("x", dedupMin-1)
	s.Check(p)
	if _, ok := s.Check(p); ok {
		t.Fatal("payload below dedupMin must never hit")
	}
}

func TestSeenIsSessionScoped(t *testing.T) {
	dir := t.TempDir()
	p := bigPayload(200)
	OpenSeen(dir, "session-a", time.Hour).Check(p)
	if _, ok := OpenSeen(dir, "session-b", time.Hour).Check(p); ok {
		t.Fatal("a different session must not see another session's payloads")
	}
}

func TestSeenNilIsSafe(t *testing.T) {
	var s *Seen
	if _, ok := s.Check(bigPayload(200)); ok {
		t.Fatal("nil Seen must not hit")
	}
	if OpenSeen("", "session-a", time.Hour) != nil {
		t.Fatal("empty dir must disable dedup")
	}
	if OpenSeen(t.TempDir(), "", time.Hour) != nil {
		t.Fatal("empty session must disable dedup")
	}
}

// the session id is attacker-shaped input; it must never name a path
func TestSeenSessionIDIsNotAPath(t *testing.T) {
	dir := t.TempDir()
	s := OpenSeen(dir, "../../etc/passwd", time.Hour)
	if s == nil {
		t.Fatal("open failed")
	}
	if filepath.Dir(s.path) != dir {
		t.Fatalf("index escaped its directory: %s", s.path)
	}
	s.Check(bigPayload(200))
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 1 {
		t.Fatalf("entries = %v, err = %v", entries, err)
	}
}

func TestSeenBoundsIndexSize(t *testing.T) {
	s := OpenSeen(t.TempDir(), "session-a", time.Hour)
	for i := 0; i < seenMax+20; i++ {
		s.Check(bigPayload(200) + string(rune(i)))
	}
	if n := len(s.load()); n > seenMax {
		t.Fatalf("index grew to %d, want <= %d", n, seenMax)
	}
}

func TestSeenSurvivesCorruptIndex(t *testing.T) {
	dir := t.TempDir()
	s := OpenSeen(dir, "session-a", time.Hour)
	if err := os.WriteFile(s.path, []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, ok := s.Check(bigPayload(200)); ok {
		t.Fatal("corrupt index must fail open, not hit")
	}
}

func TestSeenGCDropsOldSessions(t *testing.T) {
	dir := t.TempDir()
	stale := filepath.Join(dir, "0000000000000000.json")
	if err := os.WriteFile(stale, []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-2 * time.Hour)
	if err := os.Chtimes(stale, old, old); err != nil {
		t.Fatal(err)
	}
	OpenSeen(dir, "session-a", time.Hour).Check(bigPayload(200))
	if _, err := os.Stat(stale); !os.IsNotExist(err) {
		t.Fatal("stale session index should have been collected")
	}
}

func TestApplyWithSeenCollapsesRepeat(t *testing.T) {
	rs := Rules{Rules: []Rule{{Tool: "Read", MaxLines: 500, KeepLast: 50}}}
	s := OpenSeen(t.TempDir(), "session-a", time.Hour)
	payload, err := json.Marshal(map[string]any{"file": map[string]any{"content": bigPayload(200)}})
	if err != nil {
		t.Fatal(err)
	}

	first, changed := ApplyWithSeen(rs, "Read", payload, nil, s)
	if changed {
		t.Fatalf("first sight should pass through, got %s", first)
	}
	second, changed := ApplyWithSeen(rs, "Read", payload, nil, s)
	if !changed {
		t.Fatal("repeat should have been collapsed")
	}
	if !strings.Contains(string(second), "identical to an earlier Read in this session") {
		t.Fatalf("missing dedup marker: %s", second)
	}
	if !strings.Contains(string(second), "200 lines unchanged") {
		t.Fatalf("missing line count: %s", second)
	}
	if len(second) >= len(payload) {
		t.Fatalf("dedup did not shrink the payload: %d >= %d", len(second), len(payload))
	}
}

// a dedup marker is only safe if the original is recoverable
func TestApplyWithSeenIsReversible(t *testing.T) {
	rs := Rules{Rules: []Rule{{Tool: "Read", MaxLines: 500}}}
	s := OpenSeen(t.TempDir(), "session-a", time.Hour)
	st := testStore(t)
	payload, err := json.Marshal(map[string]any{"file": map[string]any{"content": bigPayload(200)}})
	if err != nil {
		t.Fatal(err)
	}

	ApplyWithSeen(rs, "Read", payload, st, s)
	out, changed := ApplyWithSeen(rs, "Read", payload, st, s)
	if !changed {
		t.Fatal("repeat should have been collapsed")
	}
	m := regexp.MustCompile(`isthmos reveal ([0-9a-f]{16})`).FindStringSubmatch(string(out))
	if m == nil {
		t.Fatalf("no reveal id in marker: %s", out)
	}
	got, err := st.Load(m[1])
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(payload) {
		t.Fatal("revealed payload does not match the original")
	}
}

func TestApplyWithStoreLeavesDedupOff(t *testing.T) {
	rs := Rules{Rules: []Rule{{Tool: "Read", MaxLines: 500}}}
	payload, err := json.Marshal(map[string]any{"file": map[string]any{"content": bigPayload(200)}})
	if err != nil {
		t.Fatal(err)
	}
	ApplyWithStore(rs, "Read", payload, nil)
	if _, changed := ApplyWithStore(rs, "Read", payload, nil); changed {
		t.Fatal("ApplyWithStore must not dedup across calls")
	}
}
