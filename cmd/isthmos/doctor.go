// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/sphragis-oss/isthmos"
)

// runDoctor reports setup health, returns 1 when something is broken
func runDoctor(w io.Writer) int {
	code := 0
	fmt.Fprintf(w, "version: %s\n", version)

	p := configPath()
	switch b, err := os.ReadFile(p); {
	case errors.Is(err, os.ErrNotExist):
		fmt.Fprintf(w, "rules:   %s: missing, isthmos is a no-op\n", p)
	case err != nil:
		code = 1
		fmt.Fprintf(w, "rules:   %s: FAIL %v\n", p, err)
	default:
		var rs isthmos.Rules
		if err := json.Unmarshal(b, &rs); err != nil {
			code = 1
			fmt.Fprintf(w, "rules:   %s: FAIL invalid JSON: %v\n", p, err)
		} else {
			fmt.Fprintf(w, "rules:   %s: ok, %d rules\n", p, len(rs.Rules))
		}
	}

	if openStore() == nil {
		code = 1
		fmt.Fprintln(w, "store:   FAIL cannot open, truncation would be irreversible")
	} else {
		n := 0
		entries, _ := os.ReadDir(filepath.Join(stateDir(), "store"))
		for _, e := range entries {
			if filepath.Ext(e.Name()) == ".bin" {
				n++
			}
		}
		fmt.Fprintf(w, "store:   ok, %d entries\n", n)
	}

	if fi, err := os.Stat(measurePath()); err != nil {
		fmt.Fprintf(w, "measure: %s: no data yet\n", measurePath())
	} else {
		fmt.Fprintf(w, "measure: %s: %s, last write %s\n", measurePath(), human(fi.Size()), fi.ModTime().Format(time.RFC3339))
		if f, err := os.Open(measurePath()); err == nil {
			var calls int
			var in int64
			for _, s := range isthmos.Aggregate(f, time.Now().Add(-72*time.Hour)) {
				calls += s.Calls
				in += s.InBytes
			}
			_ = f.Close()
			// a firing hook that only ever sees empty payloads is miswired
			if calls >= 5 && in == 0 {
				code = 1
				fmt.Fprintf(w, "payload: FAIL %d recent calls all carried 0 bytes, hook input mismatch\n", calls)
			}
		}
	}

	home, _ := os.UserHomeDir()
	if b, err := os.ReadFile(filepath.Join(home, ".claude", "settings.json")); err == nil && bytes.Contains(b, []byte("isthmos")) {
		fmt.Fprintln(w, "hook:    wired in ~/.claude/settings.json")
	} else {
		fmt.Fprintln(w, "hook:    not in ~/.claude/settings.json (fine if you use filter mode)")
	}

	if shadowMode() {
		fmt.Fprintln(w, "shadow:  ON, measuring only, no rewriting")
	} else {
		fmt.Fprintln(w, "shadow:  off")
	}
	return code
}
