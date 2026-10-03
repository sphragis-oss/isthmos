// SPDX-License-Identifier: Apache-2.0

package isthmos

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"slices"
	"strings"
	"unicode/utf8"
)

type Limits struct {
	MaxItems  int
	MaxStr    int
	MaxLines  int
	KeepLast  int
	Dedup     bool
	DropEmpty bool
	Tabular   bool
	KeepShape bool
}

func (l Limits) empty() bool {
	return l.MaxItems == 0 && l.MaxStr == 0 && l.MaxLines == 0 && !l.Dedup && !l.DropEmpty && !l.Tabular
}

func (l Limits) text() bool { return l.MaxLines > 0 || l.Dedup }

type pruneCtx struct {
	drop      map[string]bool
	keep      map[string]bool
	paths     map[string][][]string
	lim       Limits
	id        string
	tool      string
	seen      *Seen
	truncated bool
}

// newCtx indexes dotted drop keys by their last segment
func newCtx(drop, keep map[string]bool, lim Limits) *pruneCtx {
	c := &pruneCtx{drop: drop, keep: keep, lim: lim}
	for k := range drop {
		p := strings.Split(k, ".")
		if len(p) < 2 {
			continue
		}
		if c.paths == nil {
			c.paths = map[string][][]string{}
		}
		last := p[len(p)-1]
		c.paths[last] = append(c.paths[last], p[:len(p)-1])
	}
	return c
}

// dropped matches a bare key anywhere, or a dotted key under its named parents
func (c *pruneCtx) dropped(k string, path []string) bool {
	if c.drop[k] {
		return true
	}
	for _, parents := range c.paths[k] {
		if len(path) >= len(parents) && slices.Equal(path[len(path)-len(parents):], parents) {
			return true
		}
	}
	return false
}

func (c *pruneCtx) hint() string {
	if c.id == "" {
		return ""
	}
	return ", full: isthmos reveal " + c.id
}

// dedupHit replaces text already sent this session with a reference to it
func (c *pruneCtx) dedupHit(s string) (string, bool) {
	e, ok := c.seen.Check(s)
	if !ok {
		return "", false
	}
	c.truncated = true
	tool := c.tool
	if tool == "" {
		tool = "call"
	}
	return fmt.Sprintf("[isthmos: identical to an earlier %s in this session, %d lines unchanged%s]", tool, e.Lines, c.hint()), true
}

// Apply returns the possibly pruned output and whether it shrank
func Apply(rs Rules, tool string, output json.RawMessage) (json.RawMessage, bool) {
	return ApplyWithStore(rs, tool, output, nil)
}

// ApplyWithStore also spools the original payload so truncation markers are reversible
func ApplyWithStore(rs Rules, tool string, output json.RawMessage, st *Store) (json.RawMessage, bool) {
	return ApplyWithSeen(rs, tool, output, st, nil)
}

// ApplyWithSeen also collapses payloads this session already sent to the agent
func ApplyWithSeen(rs Rules, tool string, output json.RawMessage, st *Store, seen *Seen) (json.RawMessage, bool) {
	rs = rs.eligible(len(output))
	drop := rs.DropFor(tool)
	keep := rs.KeepFor(tool)
	lim := rs.LimitsFor(tool)
	if (len(drop) == 0 && len(keep) == 0 && lim.empty() && seen == nil) || len(output) == 0 {
		return output, false
	}
	c := newCtx(drop, keep, lim)
	c.tool, c.seen = tool, seen
	if st != nil {
		c.id = newID()
	}
	pruned, err := pruneJSON(output, c)
	if err != nil || len(pruned) >= len(output) {
		return output, false
	}
	if c.truncated && st != nil && c.id != "" {
		// fail-open: a marker must never point at a payload that was not stored
		if err := st.Save(c.id, output, tool); err != nil {
			return output, false
		}
	}
	return pruned, true
}

// PruneJSON drops keys and applies limits recursively, unwrapping a JSON-encoded string payload
func PruneJSON(raw json.RawMessage, drop map[string]bool, lim Limits) ([]byte, error) {
	return pruneJSON(raw, newCtx(drop, nil, lim))
}

// decode keeps numbers as written, so large integers survive the round trip
func decode(b []byte) (any, error) {
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return nil, err
	}
	if _, err := dec.Token(); err != io.EOF {
		return nil, errors.New("trailing data after JSON value")
	}
	return v, nil
}

// encode skips HTML escaping, which would inflate <, > and & sixfold
func encode(v any) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return bytes.TrimSuffix(buf.Bytes(), []byte("\n")), nil
}

func pruneJSON(raw json.RawMessage, c *pruneCtx) ([]byte, error) {
	v, err := decode(raw)
	if err != nil {
		// not JSON at all: the text path is the only one that can help
		if m, ok := c.dedupHit(string(raw)); ok {
			return []byte(m), nil
		}
		if c.lim.text() {
			return []byte(compressText(string(raw), c)), nil
		}
		return nil, err
	}
	if s, ok := v.(string); ok {
		inner, err := decode([]byte(s))
		if err != nil {
			// a JSON string carrying plain text (file contents, logs)
			if m, ok := c.dedupHit(s); ok {
				return encode(m)
			}
			if c.lim.text() {
				return encode(compressText(s, c))
			}
			return nil, err
		}
		b, err := encode(prune(inner, c, nil, false))
		if err != nil {
			return nil, err
		}
		return encode(string(b))
	}
	return encode(prune(v, c, nil, false))
}

// prune walks the value; kept is true once an allowlisted key is an ancestor
func prune(v any, c *pruneCtx, path []string, kept bool) any {
	// under keep_keys, anything outside a kept subtree must earn its place
	strict := len(c.keep) > 0 && !kept
	switch t := v.(type) {
	case map[string]any:
		for k, val := range t {
			in := kept || c.keep[k]
			if c.dropped(k, path) || (strict && !in && !isContainer(val)) {
				delete(t, k)
				continue
			}
			val = prune(val, c, append(path, k), in)
			if isEmpty(val) && (c.lim.DropEmpty || (strict && !in)) {
				delete(t, k)
				continue
			}
			t[k] = val
		}
		return t
	case []any:
		out := t[:0]
		for _, val := range t {
			if strict && !isContainer(val) {
				continue
			}
			val = prune(val, c, path, kept)
			if strict && isEmpty(val) {
				continue
			}
			out = append(out, val)
		}
		out = capItems(out, c)
		if c.lim.Tabular {
			if tb, ok := tabulate(out); ok {
				return tb
			}
		}
		return out
	case string:
		return capStr(t, c)
	default:
		return v
	}
}

func isContainer(v any) bool {
	switch v.(type) {
	case map[string]any, []any:
		return true
	}
	return false
}

func isEmpty(v any) bool {
	switch t := v.(type) {
	case nil:
		return true
	case string:
		return t == ""
	case []any:
		return len(t) == 0
	case map[string]any:
		return len(t) == 0
	}
	return false
}

// tableMin is the row count below which repeating the keys is cheaper
const tableMin = 3

// tabulate rewrites same-shaped objects as one column list plus value rows, losslessly
func tabulate(t []any) (any, bool) {
	rows := t
	var note string
	if n := len(t); n > 0 {
		if s, ok := t[n-1].(string); ok && strings.HasPrefix(s, "[isthmos:") {
			rows, note = t[:n-1], s
		}
	}
	if len(rows) < tableMin {
		return nil, false
	}
	first, ok := rows[0].(map[string]any)
	if !ok || len(first) < 2 {
		return nil, false
	}
	cols := slices.Sorted(maps.Keys(first))
	out := make([]any, len(rows))
	for i, r := range rows {
		m, ok := r.(map[string]any)
		if !ok || len(m) != len(cols) {
			return nil, false
		}
		row := make([]any, len(cols))
		for j, k := range cols {
			val, ok := m[k]
			if !ok {
				return nil, false
			}
			row[j] = val
		}
		out[i] = row
	}
	tb := map[string]any{"cols": cols, "rows": out}
	if note != "" {
		tb["note"] = note
	}
	return map[string]any{"isthmos_table": tb}, true
}

// capItems keeps head, tail, and error-looking items, replacing the rest with a marker
func capItems(t []any, c *pruneCtx) []any {
	lim := c.lim
	if lim.MaxItems <= 0 || len(t) <= lim.MaxItems {
		return t
	}
	// the marker is a string, so it only fits an array that already holds strings
	if lim.KeepShape && slices.ContainsFunc(t, func(v any) bool { _, ok := v.(string); return !ok }) {
		return t
	}
	keepLast := lim.KeepLast
	if keepLast >= lim.MaxItems {
		keepLast = lim.MaxItems - 1
	}
	keep := make([]bool, len(t))
	for i := 0; i < lim.MaxItems-keepLast; i++ {
		keep[i] = true
	}
	for i := len(t) - keepLast; i < len(t); i++ {
		keep[i] = true
	}
	// pins get their own budget, or an all-errors payload would never shrink
	pins := lim.MaxItems
	for i, v := range t {
		if pins > 0 && !keep[i] && looksLikeError(v) {
			keep[i] = true
			pins--
		}
	}
	out := make([]any, 0, lim.MaxItems+1)
	dropped := 0
	for i, v := range t {
		if keep[i] {
			out = append(out, v)
		} else {
			dropped++
		}
	}
	if dropped == 0 {
		return t
	}
	c.truncated = true
	return append(out, fmt.Sprintf("[isthmos: %d of %d items truncated%s]", dropped, len(t), c.hint()))
}

var errStates = map[string]bool{
	"error": true, "errors": true, "failed": true, "failure": true,
	"fatal": true, "critical": true, "unhealthy": true, "timeout": true,
}

// looksLikeError flags items truncation must never drop
func looksLikeError(v any) bool {
	m, ok := v.(map[string]any)
	if !ok {
		return false
	}
	for k, val := range m {
		switch strings.ToLower(k) {
		case "error", "errors", "err", "exception":
			if truthy(val) {
				return true
			}
		case "status", "state", "level", "severity", "result", "conclusion", "outcome":
			if s, ok := val.(string); ok && errStates[strings.ToLower(s)] {
				return true
			}
		}
	}
	return false
}

func truthy(v any) bool {
	switch t := v.(type) {
	case nil:
		return false
	case bool:
		return t
	case string:
		return t != "" && strings.ToLower(t) != "null"
	case json.Number:
		f, err := t.Float64()
		return err != nil || f != 0
	case []any:
		return len(t) > 0
	case map[string]any:
		return len(t) > 0
	default:
		return true
	}
}

// capStr truncates a long string at a rune boundary, appending an explicit marker
func capStr(s string, c *pruneCtx) string {
	if m, ok := c.dedupHit(s); ok {
		return m
	}
	// real hook payloads carry text inside JSON strings (stdout, file.content)
	if c.lim.text() {
		s = compressText(s, c)
	}
	maxStr := c.lim.MaxStr
	if maxStr <= 0 || len(s) <= maxStr {
		return s
	}
	i := maxStr
	for i > 0 && !utf8.RuneStart(s[i]) {
		i--
	}
	c.truncated = true
	return fmt.Sprintf("%s...[isthmos: %d bytes truncated%s]", s[:i], len(s)-i, c.hint())
}
