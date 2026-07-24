// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/sphragis-oss/isthmos"
)

// measureCap bounds the log; var so tests can shrink it
var measureCap = int64(5 << 20)

func measurePath() string {
	d := stateDir()
	if d == "" {
		return ""
	}
	return filepath.Join(d, "measure.jsonl")
}

// logMeasure appends one JSONL line, best effort
func logMeasure(tool string, in, out int) {
	appendMeasure(isthmos.Measure{TS: time.Now().UTC(), Tool: tool, InBytes: in, OutBytes: out})
}

// logReveal records that a truncated payload had to be recovered
func logReveal(tool string) {
	if tool == "" {
		tool = "(unknown)"
	}
	appendMeasure(isthmos.Measure{TS: time.Now().UTC(), Tool: tool, Reveal: true})
}

func appendMeasure(m isthmos.Measure) {
	p := measurePath()
	if p == "" {
		return
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return
	}
	if fi, err := os.Stat(p); err == nil && fi.Size() > measureCap {
		trimMeasure(p, fi.Size())
	}
	f, err := os.OpenFile(p, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	_ = json.NewEncoder(f).Encode(m)
}

// trimMeasure keeps the newest half of an oversized log, best effort
func trimMeasure(p string, size int64) {
	f, err := os.Open(p)
	if err != nil {
		return
	}
	defer f.Close()
	if _, err := f.Seek(size-measureCap/2, io.SeekStart); err != nil {
		return
	}
	b, err := io.ReadAll(f)
	if err != nil {
		return
	}
	if i := bytes.IndexByte(b, '\n'); i >= 0 {
		b = b[i+1:]
	}
	tmp := p + ".tmp"
	if os.WriteFile(tmp, b, 0o644) != nil {
		return
	}
	_ = os.Rename(tmp, p)
}
