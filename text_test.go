// SPDX-License-Identifier: Apache-2.0

package isthmos

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"testing"
)

func textCtx(lim Limits) *pruneCtx { return &pruneCtx{lim: lim} }

func revealID(t *testing.T, s string) string {
	t.Helper()
	m := regexp.MustCompile(`isthmos reveal ([0-9a-f]{16})`).FindStringSubmatch(s)
	if m == nil {
		t.Fatalf("no reveal id in %q", s)
	}
	return m[1]
}

func TestDedupCollapsesRuns(t *testing.T) {
	in := "a\nb\nb\nb\nb\nc"
	got := compressText(in, textCtx(Limits{Dedup: true}))
	want := "a\nb\n[isthmos: previous line repeated 3 more times]\nc"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestDedupKeepsShortRuns(t *testing.T) {
	in := "a\nb\nb\nc"
	if got := compressText(in, textCtx(Limits{Dedup: true})); got != in {
		t.Fatalf("run of 2 must not collapse: %q", got)
	}
}

func TestCapLinesHeadTailAndMarker(t *testing.T) {
	var lines []string
	for i := 0; i < 20; i++ {
		lines = append(lines, fmt.Sprintf("line-%d", i))
	}
	c := textCtx(Limits{MaxLines: 5, KeepLast: 2})
	got := strings.Split(compressText(strings.Join(lines, "\n"), c), "\n")
	if !c.truncated {
		t.Fatal("expected truncated flag")
	}
	if got[0] != "line-0" || got[2] != "line-2" {
		t.Fatalf("head lost: %v", got)
	}
	if got[len(got)-1] != "line-19" || got[len(got)-2] != "line-18" {
		t.Fatalf("tail lost: %v", got)
	}
	if !strings.Contains(got[3], "isthmos: 15 of 20 lines truncated") {
		t.Fatalf("bad marker: %v", got)
	}
}

func TestCapLinesPinsErrorLines(t *testing.T) {
	var lines []string
	for i := 0; i < 30; i++ {
		lines = append(lines, fmt.Sprintf("ok-%d", i))
	}
	lines[17] = "ERROR: something exploded"
	c := textCtx(Limits{MaxLines: 5, KeepLast: 2})
	got := compressText(strings.Join(lines, "\n"), c)
	if !strings.Contains(got, "ERROR: something exploded") {
		t.Fatalf("error line dropped: %q", got)
	}
}

func TestCapLinesUntouchedWhenUnderLimit(t *testing.T) {
	in := "a\nb\nc"
	c := textCtx(Limits{MaxLines: 10})
	if got := compressText(in, c); got != in || c.truncated {
		t.Fatalf("small payload must pass through: %q truncated=%v", got, c.truncated)
	}
}

func TestCapLinesTrailingNewlineNotBudgeted(t *testing.T) {
	var lines []string
	for i := 0; i < 10; i++ {
		lines = append(lines, fmt.Sprintf("l%d", i))
	}
	in := strings.Join(lines, "\n") + "\n"
	c := textCtx(Limits{MaxLines: 4, KeepLast: 2})
	got := compressText(in, c)
	if !strings.HasSuffix(got, "l8\nl9\n") {
		t.Fatalf("trailing newline ate a keep_last slot: %q", got)
	}
	if !strings.Contains(got, "6 of 10 lines truncated") {
		t.Fatalf("marker counts the trailing newline: %q", got)
	}
}

func TestApplyRawTextPayload(t *testing.T) {
	rs := Rules{Rules: []Rule{{Tool: "Bash", MaxLines: 4, KeepLast: 1}}}
	in := []byte(strings.Repeat("log line with some padding\n", 50) + "final")
	out, changed := Apply(rs, "Bash", in)
	if !changed {
		t.Fatal("expected change")
	}
	if len(out) >= len(in) {
		t.Fatal("expected shrink")
	}
	s := string(out)
	if !strings.Contains(s, "lines truncated") || !strings.HasSuffix(s, "final") {
		t.Fatalf("bad text output: %q", s)
	}
}

func TestApplyJSONStringWrappedText(t *testing.T) {
	rs := Rules{Rules: []Rule{{Tool: "Read", Dedup: true}}}
	text := "top\n" + strings.Repeat("same line\n", 40) + "bottom"
	in, err := json.Marshal(text)
	if err != nil {
		t.Fatal(err)
	}
	out, changed := Apply(rs, "Read", in)
	if !changed {
		t.Fatal("expected change")
	}
	var got string
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("output is not a JSON string: %v", err)
	}
	if !strings.Contains(got, "repeated 39 more times") || !strings.HasSuffix(got, "bottom") {
		t.Fatalf("bad dedup: %q", got)
	}
}

func TestApplyTextLimitsReachEmbeddedStrings(t *testing.T) {
	rs := Rules{Rules: []Rule{{Tool: "Bash", MaxLines: 4, KeepLast: 1, Dedup: true}}}
	stdout := "start\n" + strings.Repeat("same line\n", 60) + "end"
	in, err := json.Marshal(map[string]any{"stdout": stdout, "stderr": "", "interrupted": false})
	if err != nil {
		t.Fatal(err)
	}
	out, changed := Apply(rs, "Bash", in)
	if !changed {
		t.Fatal("expected change")
	}
	s := string(out)
	if !strings.Contains(s, "repeated 59 more times") || !strings.Contains(s, `"interrupted":false`) {
		t.Fatalf("embedded string not compressed or structure lost: %s", s)
	}
}

func TestApplyNonJSONWithoutTextLimits(t *testing.T) {
	rs := Rules{Rules: []Rule{{Tool: "*", MaxItems: 2}}}
	in := []byte("plain text\nno json here\n")
	out, changed := Apply(rs, "anything", in)
	if changed || string(out) != string(in) {
		t.Fatal("non-JSON without text limits must pass through")
	}
}

func TestTextTruncationIsRevealable(t *testing.T) {
	st := testStore(t)
	rs := Rules{Rules: []Rule{{Tool: "*", MaxLines: 3}}}
	in := []byte(strings.Repeat("a fairly long log line for padding\n", 30))
	out, changed := ApplyWithStore(rs, "Bash", in, st)
	if !changed {
		t.Fatal("expected change")
	}
	id := revealID(t, string(out))
	orig, err := st.Load(id)
	if err != nil {
		t.Fatal(err)
	}
	if string(orig) != string(in) {
		t.Fatal("stored payload is not the original")
	}
	if st.Tool(id) != "Bash" {
		t.Fatalf("bad attribution: %q", st.Tool(id))
	}
}

func TestDedupAloneStoresNothing(t *testing.T) {
	st := testStore(t)
	rs := Rules{Rules: []Rule{{Tool: "*", Dedup: true}}}
	in := []byte(strings.Repeat("same\n", 30) + "end")
	out, changed := ApplyWithStore(rs, "Bash", in, st)
	if !changed {
		t.Fatal("expected change")
	}
	if strings.Contains(string(out), "reveal") {
		t.Fatalf("dedup-only output should have no reveal hint: %s", out)
	}
}
