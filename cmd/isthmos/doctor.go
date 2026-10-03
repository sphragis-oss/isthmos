// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/sphragis-oss/isthmos"
)

// runDoctor reports setup health, returns 1 when something is broken
func runDoctor(w io.Writer) int {
	code := 0
	fmt.Fprintf(w, "version: %s\n", version)

	var rules isthmos.Rules
	p := configPath()
	switch b, err := os.ReadFile(p); {
	case errors.Is(err, os.ErrNotExist):
		fmt.Fprintf(w, "rules:   %s: missing, isthmos is a no-op\n", p)
	case err != nil:
		code = 1
		fmt.Fprintf(w, "rules:   %s: FAIL %v\n", p, err)
	default:
		// strict here only: a typo'd field is a rule that silently does nothing
		dec := json.NewDecoder(bytes.NewReader(b))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&rules); err != nil {
			code = 1
			fmt.Fprintf(w, "rules:   %s: FAIL invalid rules: %v\n", p, err)
		} else if bad := badGlob(rules); bad != "" {
			code = 1
			fmt.Fprintf(w, "rules:   %s: FAIL tool glob %q is malformed and never matches\n", p, bad)
		} else {
			fmt.Fprintf(w, "rules:   %s: ok, %d rules\n", p, len(rules.Rules))
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

	var reveals int
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
				reveals += s.Reveals
			}
			_ = f.Close()
			// a firing hook that only ever sees empty payloads is miswired
			if calls >= 5 && in == 0 {
				code = 1
				fmt.Fprintf(w, "payload: FAIL %d recent calls all carried 0 bytes, hook input mismatch\n", calls)
			}
		}
	}

	var wired wiring
	for _, p := range settingsFiles() {
		if b, err := os.ReadFile(p); err == nil && wired.scan(b) {
			fmt.Fprintf(w, "hook:    wired in %s\n", p)
		}
	}
	if len(wired.cmds) == 0 {
		fmt.Fprintln(w, "hook:    not in any Claude Code settings file (fine if you use filter or mcp mode)")
	} else {
		for _, r := range deadRules(rules, wired.matchers) {
			fmt.Fprintf(w, "hook:    WARN rule %q not routed by any isthmos hook matcher\n", r)
		}
		if !wired.compact && !dedupOff() && !wired.all(noDedupEnv) {
			fmt.Fprintln(w, "hook:    WARN no SessionStart compact hook, cross-call dedup can point at content compaction removed")
		}
	}

	// the hook runs with the env its command sets, not with this shell's
	if shadowMode() || wired.all(shadowEnv) {
		fmt.Fprintln(w, "shadow:  ON, measuring only, nothing is rewritten")
		fmt.Fprintln(w, "next:    let it run, then read the numbers with: isthmos stats")
	} else {
		fmt.Fprintln(w, "shadow:  off, tool output is rewritten live")
		// every reveal is an extra tool call the agent had to make to undo a cut
		if reveals > 0 {
			fmt.Fprintf(w, "reveals: WARN %d truncation(s) recovered in the last 72h, pruning is too aggressive somewhere\n", reveals)
		}
	}
	return code
}

type hookSettings struct {
	Hooks map[string][]struct {
		Matcher string `json:"matcher"`
		Hooks   []struct {
			Command string `json:"command"`
		} `json:"hooks"`
	} `json:"hooks"`
}

var (
	shadowEnv  = regexp.MustCompile(`\bISTHMOS_SHADOW=(1|true)\b`)
	noDedupEnv = regexp.MustCompile(`\bISTHMOS_NO_DEDUP=(1|true)\b`)
)

// wiring is what the settings files say about how the hook is invoked
type wiring struct {
	matchers []*regexp.Regexp
	cmds     []string
	compact  bool
}

// settingsFiles lists the user and project settings a hook can live in
func settingsFiles() []string {
	home, _ := os.UserHomeDir()
	var out []string
	seen := map[string]bool{}
	for _, p := range []string{
		filepath.Join(home, ".claude", "settings.json"),
		filepath.Join(".claude", "settings.json"),
		filepath.Join(".claude", "settings.local.json"),
	} {
		if abs, err := filepath.Abs(p); err == nil && !seen[abs] {
			seen[abs] = true
			out = append(out, abs)
		}
	}
	return out
}

// scan collects isthmos hooks from one settings file, reporting whether it had any
func (w *wiring) scan(settings []byte) bool {
	var s hookSettings
	if json.Unmarshal(settings, &s) != nil {
		return false
	}
	found := false
	for event, entries := range s.Hooks {
		for _, e := range entries {
			for _, h := range e.Hooks {
				if !strings.Contains(h.Command, "isthmos") {
					continue
				}
				switch event {
				case "PostToolUse":
					found = true
					w.cmds = append(w.cmds, h.Command)
					if re, err := regexp.Compile("^(?:" + e.Matcher + ")$"); err == nil {
						w.matchers = append(w.matchers, re)
					}
				case "SessionStart":
					found = true
					w.compact = true
				}
				break
			}
		}
	}
	return found
}

// all reports whether every isthmos tool hook sets the given env assignment
func (w *wiring) all(env *regexp.Regexp) bool {
	for _, c := range w.cmds {
		if !env.MatchString(c) {
			return false
		}
	}
	return len(w.cmds) > 0
}

// badGlob returns the first tool pattern path.Match rejects
func badGlob(rs isthmos.Rules) string {
	for _, r := range rs.Rules {
		if _, err := path.Match(r.Tool, ""); err != nil {
			return r.Tool
		}
	}
	return ""
}

// deadRules lists rule globs no isthmos hook matcher routes to
func deadRules(rs isthmos.Rules, matchers []*regexp.Regexp) []string {
	if len(matchers) == 0 {
		return nil
	}
	var dead []string
	for _, r := range rs.Rules {
		// a representative tool name stands in for the glob
		sample := strings.ReplaceAll(r.Tool, "*", "x")
		routed := false
		for _, re := range matchers {
			if re.MatchString(sample) {
				routed = true
				break
			}
		}
		if !routed {
			dead = append(dead, r.Tool)
		}
	}
	return dead
}
