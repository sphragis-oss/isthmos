// SPDX-License-Identifier: Apache-2.0

package isthmos

import (
	"strings"
	"testing"
	"time"
)

const sampleLog = `{"ts":"2026-07-18T10:00:00Z","tool":"mcp__atlassian__searchJiraIssuesUsingJql","in_bytes":1000,"out_bytes":400}
{"ts":"2026-07-19T10:00:00Z","tool":"mcp__atlassian__searchJiraIssuesUsingJql","in_bytes":2000,"out_bytes":800}
not json at all
{"ts":"2026-07-19T11:00:00Z","tool":"mcp__github__get_me","in_bytes":500,"out_bytes":450}
`

func TestAggregateSumsAndSorts(t *testing.T) {
	stats := Aggregate(strings.NewReader(sampleLog), time.Time{})
	if len(stats) != 2 {
		t.Fatalf("expected 2 tools, got %d", len(stats))
	}
	first := stats[0]
	if first.Tool != "mcp__atlassian__searchJiraIssuesUsingJql" {
		t.Fatalf("expected biggest saver first, got %s", first.Tool)
	}
	if first.Calls != 2 || first.InBytes != 3000 || first.OutBytes != 1200 {
		t.Fatalf("bad sums: %+v", first)
	}
	if first.Saved() != 1800 {
		t.Fatalf("expected 1800 saved, got %d", first.Saved())
	}
}

func TestAggregateSkipsMalformedLines(t *testing.T) {
	stats := Aggregate(strings.NewReader("garbage\n{broken\n"), time.Time{})
	if len(stats) != 0 {
		t.Fatalf("expected no stats from garbage, got %d", len(stats))
	}
}

func TestAggregateSinceFilter(t *testing.T) {
	since := time.Date(2026, 7, 19, 0, 0, 0, 0, time.UTC)
	stats := Aggregate(strings.NewReader(sampleLog), since)
	for _, s := range stats {
		if s.Tool == "mcp__atlassian__searchJiraIssuesUsingJql" && s.Calls != 1 {
			t.Fatalf("since filter kept %d calls, expected 1", s.Calls)
		}
	}
}

func TestAggregateCountsReveals(t *testing.T) {
	log := `{"ts":"2026-07-19T10:00:00Z","tool":"a","in_bytes":1000,"out_bytes":400}
{"ts":"2026-07-19T11:00:00Z","tool":"a","reveal":true}
{"ts":"2026-07-19T12:00:00Z","tool":"b","reveal":true}
`
	stats := Aggregate(strings.NewReader(log), time.Time{})
	byTool := map[string]ToolStat{}
	for _, s := range stats {
		byTool[s.Tool] = s
	}
	a := byTool["a"]
	if a.Calls != 1 || a.InBytes != 1000 || a.Reveals != 1 {
		t.Fatalf("reveal line must not count as a call: %+v", a)
	}
	b := byTool["b"]
	if b.Calls != 0 || b.Reveals != 1 {
		t.Fatalf("reveal-only tool should still surface: %+v", b)
	}
}

func TestEstTokens(t *testing.T) {
	if EstTokens(4000) != 1000 {
		t.Fatalf("expected 1000, got %d", EstTokens(4000))
	}
}
