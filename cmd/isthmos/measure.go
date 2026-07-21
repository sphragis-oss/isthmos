// SPDX-License-Identifier: Apache-2.0

package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"github.com/sphragis-oss/isthmos"
)

func measurePath() string {
	d := stateDir()
	if d == "" {
		return ""
	}
	return filepath.Join(d, "measure.jsonl")
}

// logMeasure appends one JSONL line, best effort
func logMeasure(tool string, in, out int) {
	p := measurePath()
	if p == "" {
		return
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return
	}
	f, err := os.OpenFile(p, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	_ = json.NewEncoder(f).Encode(isthmos.Measure{TS: time.Now().UTC(), Tool: tool, InBytes: in, OutBytes: out})
}
