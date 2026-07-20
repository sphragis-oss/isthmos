package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
)

type hookInput struct {
	HookEventName string          `json:"hook_event_name"`
	ToolName      string          `json:"tool_name"`
	ToolInput     json.RawMessage `json:"tool_input"`
	ToolOutput    json.RawMessage `json:"tool_output"`
}

// passthrough stub: parse the PostToolUse payload, change nothing
func main() {
	raw, err := io.ReadAll(os.Stdin)
	if err != nil {
		slog.Error("read stdin", "err", err)
		os.Exit(1)
	}
	var in hookInput
	if err := json.Unmarshal(raw, &in); err != nil {
		slog.Error("parse hook input", "err", err)
		os.Exit(1)
	}
	fmt.Fprintf(os.Stderr, "isthmos: %s output %d bytes (passthrough)\n", in.ToolName, len(in.ToolOutput))
}
