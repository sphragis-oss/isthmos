package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"

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

func main() {
	mode, args := "hook", os.Args[1:]
	if len(args) > 0 {
		mode, args = args[0], args[1:]
	}
	switch mode {
	case "hook":
		runHook()
	case "filter":
		runFilter(args)
	default:
		fmt.Fprintln(os.Stderr, "usage: isthmos [hook|filter -tool NAME]")
		os.Exit(2)
	}
}

// runHook is the Claude Code PostToolUse adapter, fail-open on any error
func runHook() {
	raw, err := io.ReadAll(os.Stdin)
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
	out, changed := isthmos.Apply(rs, in.ToolName, in.ToolOutput)
	logMeasure(in.ToolName, len(in.ToolOutput), len(out))
	if !changed {
		return
	}
	res := hookOutput{hookSpecificOutput{"PostToolUse", out}}
	if err := json.NewEncoder(os.Stdout).Encode(res); err != nil {
		slog.Error("write hook output", "err", err)
	}
}

// runFilter is the agent-agnostic mode: raw output on stdin, pruned on stdout
func runFilter(args []string) {
	fs := flag.NewFlagSet("filter", flag.ExitOnError)
	tool := fs.String("tool", "", "tool name matched against rule globs")
	fs.Parse(args)
	raw, err := io.ReadAll(os.Stdin)
	if err != nil {
		slog.Error("read stdin", "err", err)
		os.Exit(1)
	}
	rs := isthmos.LoadRules(configPath())
	out, _ := isthmos.Apply(rs, *tool, raw)
	logMeasure(*tool, len(raw), len(out))
	os.Stdout.Write(out)
}
