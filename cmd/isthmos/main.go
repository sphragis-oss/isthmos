// SPDX-License-Identifier: Apache-2.0

package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/sphragis-oss/isthmos"
)

type hookInput struct {
	ToolName   string          `json:"tool_name"`
	ToolOutput json.RawMessage `json:"tool_output"`
}

type hookSpecificOutput struct {
	HookEventName     string          `json:"hookEventName"`
	UpdatedToolOutput json.RawMessage `json:"updatedToolOutput"`
}

type hookOutput struct {
	HookSpecificOutput hookSpecificOutput `json:"hookSpecificOutput"`
}

func configPath() string {
	if p := os.Getenv("ISTHMOS_RULES"); p != "" {
		return p
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "isthmos", "rules.json")
}

// shadowMode measures what pruning would save without rewriting anything
func shadowMode() bool {
	v := os.Getenv("ISTHMOS_SHADOW")
	return v == "1" || v == "true"
}

// openStore is fail-open: on any error pruning proceeds without reversibility
func openStore() *isthmos.Store {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}
	st, err := isthmos.OpenStore(filepath.Join(home, ".local", "state", "isthmos", "store"), 7*24*time.Hour)
	if err != nil {
		slog.Error("open store", "err", err)
		return nil
	}
	return st
}

// version is set by goreleaser via ldflags
var version = "dev"

func main() {
	mode, args := "hook", os.Args[1:]
	if len(args) > 0 {
		mode, args = args[0], args[1:]
	}
	switch mode {
	case "hook":
		runHook(os.Stdin, os.Stdout)
	case "filter":
		runFilter(args, os.Stdin, os.Stdout)
	case "stats":
		runStats(args)
	case "reveal":
		runReveal(args)
	case "version":
		fmt.Println(version)
	default:
		fmt.Fprintln(os.Stderr, "usage: isthmos [hook|filter -tool NAME|stats|reveal <id>|version]")
		os.Exit(2)
	}
}

// runHook is the Claude Code PostToolUse adapter, fail-open on any error
func runHook(stdin io.Reader, stdout io.Writer) {
	raw, err := io.ReadAll(stdin)
	if err != nil {
		slog.Error("read stdin", "err", err)
		return
	}
	var in hookInput
	if err := json.Unmarshal(raw, &in); err != nil {
		slog.Error("parse hook input", "err", err)
		return
	}
	rs := isthmos.LoadRules(configPath())
	var st *isthmos.Store
	if !shadowMode() {
		st = openStore()
	}
	out, changed := isthmos.ApplyWithStore(rs, in.ToolName, in.ToolOutput, st)
	logMeasure(in.ToolName, len(in.ToolOutput), len(out))
	if !changed || shadowMode() {
		return
	}
	res := hookOutput{hookSpecificOutput{"PostToolUse", out}}
	if err := json.NewEncoder(stdout).Encode(res); err != nil {
		slog.Error("write hook output", "err", err)
	}
}

// runFilter is the agent-agnostic mode: raw output on stdin, pruned on stdout
func runFilter(args []string, stdin io.Reader, stdout io.Writer) {
	fs := flag.NewFlagSet("filter", flag.ExitOnError)
	tool := fs.String("tool", "", "tool name matched against rule globs")
	if err := fs.Parse(args); err != nil {
		os.Exit(2)
	}
	raw, err := io.ReadAll(stdin)
	if err != nil {
		slog.Error("read stdin", "err", err)
		os.Exit(1)
	}
	rs := isthmos.LoadRules(configPath())
	var st *isthmos.Store
	if !shadowMode() {
		st = openStore()
	}
	out, _ := isthmos.ApplyWithStore(rs, *tool, raw, st)
	logMeasure(*tool, len(raw), len(out))
	if shadowMode() {
		out = raw
	}
	if _, err := stdout.Write(out); err != nil {
		slog.Error("write stdout", "err", err)
		os.Exit(1)
	}
}

// runReveal prints the original payload a truncation marker points at
func runReveal(args []string) {
	if len(args) != 1 {
		fmt.Fprintln(os.Stderr, "usage: isthmos reveal <id>")
		os.Exit(2)
	}
	st := openStore()
	if st == nil {
		os.Exit(1)
	}
	b, err := st.Load(args[0])
	if err != nil {
		slog.Error("reveal", "id", args[0], "err", err)
		os.Exit(1)
	}
	if _, err := os.Stdout.Write(b); err != nil {
		slog.Error("write stdout", "err", err)
		os.Exit(1)
	}
}
