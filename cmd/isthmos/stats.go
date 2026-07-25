// SPDX-License-Identifier: Apache-2.0

package main

import (
	"flag"
	"fmt"
	"log/slog"
	"os"
	"sort"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/sphragis-oss/isthmos"
)

// runStats aggregates the measurement log into a per-tool savings table
func runStats(args []string) {
	fs := flag.NewFlagSet("stats", flag.ExitOnError)
	file := fs.String("file", measurePath(), "measurement log to read")
	sinceFlag := fs.Duration("since", 0, "only include entries newer than this age, e.g. 168h")
	share := fs.Bool("share", false, "replace third-party tool names with placeholders, for pasting in public")
	if err := fs.Parse(args); err != nil {
		os.Exit(2)
	}
	f, err := os.Open(*file)
	if err != nil {
		slog.Error("open measurement log", "err", err)
		os.Exit(1)
	}
	defer f.Close()
	var since time.Time
	if *sinceFlag > 0 {
		since = time.Now().Add(-*sinceFlag)
	}
	stats := isthmos.Aggregate(f, since)
	if len(stats) == 0 {
		fmt.Println("no measurements yet")
		return
	}
	if *share {
		stats = redact(stats)
	}
	var tot isthmos.ToolStat
	for _, s := range stats {
		tot.Calls += s.Calls
		tot.InBytes += s.InBytes
		tot.OutBytes += s.OutBytes
		tot.Reveals += s.Reveals
	}
	w := tabwriter.NewWriter(os.Stdout, 2, 4, 2, ' ', 0)
	fmt.Fprintln(w, "TOOL\tCALLS\tIN\tOUT\tSAVED\tSAVED%\t%ALL\t~TOKENS\tREVEALS")
	for _, s := range stats {
		fmt.Fprintf(w, "%s\t%d\t%s\t%s\t%s\t%s\t%s\t%d\t%d\n",
			s.Tool, s.Calls, human(s.InBytes), human(s.OutBytes), human(s.Saved()), pct(s.Saved(), s.InBytes), pct(s.Saved(), tot.InBytes), isthmos.EstTokens(s.Saved()), s.Reveals)
	}
	fmt.Fprintf(w, "TOTAL\t%d\t%s\t%s\t%s\t%s\t%s\t%d\t%d\n",
		tot.Calls, human(tot.InBytes), human(tot.OutBytes), human(tot.Saved()), pct(tot.Saved(), tot.InBytes), pct(tot.Saved(), tot.InBytes), isthmos.EstTokens(tot.Saved()), tot.Reveals)
	if err := w.Flush(); err != nil {
		slog.Error("write stats", "err", err)
		os.Exit(1)
	}
	fmt.Println("scope: only tool calls that reached isthmos; whole-session context is a larger denominator")
	if *share {
		fmt.Println("share: third-party tool names replaced with placeholders; no paths or payloads are ever logged")
	}
}

// builtins are the agent's own tool names, which carry nothing private
var builtins = map[string]bool{
	"Bash": true, "Edit": true, "Glob": true, "Grep": true, "NotebookEdit": true,
	"Read": true, "Task": true, "TodoWrite": true, "WebFetch": true, "WebSearch": true, "Write": true,
}

// redact folds third-party tools into per-server placeholders, keeping only the bytes
func redact(stats []isthmos.ToolStat) []isthmos.ToolStat {
	alias := map[string]string{}
	agg := map[string]*isthmos.ToolStat{}
	for _, s := range stats {
		name := shareName(s.Tool, alias)
		t, ok := agg[name]
		if !ok {
			t = &isthmos.ToolStat{Tool: name}
			agg[name] = t
		}
		t.Calls += s.Calls
		t.InBytes += s.InBytes
		t.OutBytes += s.OutBytes
		t.Reveals += s.Reveals
	}
	out := make([]isthmos.ToolStat, 0, len(agg))
	for _, t := range agg {
		out = append(out, *t)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Saved() != out[j].Saved() {
			return out[i].Saved() > out[j].Saved()
		}
		return out[i].Tool < out[j].Tool
	})
	return out
}

// shareName keeps built-in names and reduces anything else to a stable placeholder
func shareName(tool string, alias map[string]string) string {
	if builtins[tool] {
		return tool
	}
	key, mcp := tool, false
	// an MCP tool name embeds a server name that may be private
	if p := strings.SplitN(tool, "__", 3); len(p) >= 2 && p[0] == "mcp" {
		key, mcp = "mcp__"+p[1], true
	}
	if a, ok := alias[key]; ok {
		return a
	}
	a := fmt.Sprintf("tool%d", len(alias)+1)
	if mcp {
		a = fmt.Sprintf("mcp__server%d__*", len(alias)+1)
	}
	alias[key] = a
	return a
}

func human(b int64) string {
	switch {
	case b >= 1<<20:
		return fmt.Sprintf("%.1fMB", float64(b)/(1<<20))
	case b >= 1<<10:
		return fmt.Sprintf("%.1fKB", float64(b)/(1<<10))
	default:
		return fmt.Sprintf("%dB", b)
	}
}

func pct(saved, in int64) string {
	if in == 0 {
		return "-"
	}
	return fmt.Sprintf("%.1f%%", 100*float64(saved)/float64(in))
}
