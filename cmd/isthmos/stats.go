// SPDX-License-Identifier: Apache-2.0

package main

import (
	"flag"
	"fmt"
	"log/slog"
	"os"
	"text/tabwriter"
	"time"

	"github.com/sphragis-oss/isthmos"
)

// runStats aggregates the measurement log into a per-tool savings table
func runStats(args []string) {
	fs := flag.NewFlagSet("stats", flag.ExitOnError)
	file := fs.String("file", measurePath(), "measurement log to read")
	sinceFlag := fs.Duration("since", 0, "only include entries newer than this age, e.g. 168h")
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
