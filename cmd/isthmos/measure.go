package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

type measure struct {
	TS       string `json:"ts"`
	Tool     string `json:"tool"`
	InBytes  int    `json:"in_bytes"`
	OutBytes int    `json:"out_bytes"`
}

// logMeasure appends one JSONL line, best effort
func logMeasure(tool string, in, out int) {
	home, err := os.UserHomeDir()
	if err != nil {
		return
	}
	dir := filepath.Join(home, ".local", "state", "isthmos")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return
	}
	f, err := os.OpenFile(filepath.Join(dir, "measure.jsonl"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	_ = json.NewEncoder(f).Encode(measure{time.Now().UTC().Format(time.RFC3339), tool, in, out})
}
