// SPDX-License-Identifier: Apache-2.0

package isthmos

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// dedupMin is the floor below which a repeat is not worth a marker
const dedupMin = 4096

// seenMax bounds one session index; the oldest entries are dropped past it
const seenMax = 512

type seenEntry struct {
	TS    time.Time `json:"ts"`
	Lines int       `json:"lines"`
}

// Seen is a session-scoped index of payloads already sent to the agent
type Seen struct {
	path string
	dir  string
	ttl  time.Duration
}

// OpenSeen returns nil when there is no session to scope to, disabling dedup
func OpenSeen(dir, session string, ttl time.Duration) *Seen {
	if dir == "" || session == "" {
		return nil
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil
	}
	// the session id is external input, so it names nothing on disk
	sum := sha256.Sum256([]byte(session))
	return &Seen{path: filepath.Join(dir, hex.EncodeToString(sum[:8])+".json"), dir: dir, ttl: ttl}
}

// Check reports a payload already sent this session, recording it when new
func (s *Seen) Check(payload string) (seenEntry, bool) {
	if s == nil || len(payload) < dedupMin {
		return seenEntry{}, false
	}
	sum := sha256.Sum256([]byte(payload))
	key := hex.EncodeToString(sum[:16])
	m := s.load()
	if e, ok := m[key]; ok {
		return e, true
	}
	m[key] = seenEntry{TS: time.Now().UTC(), Lines: countLines(payload)}
	s.save(m)
	return seenEntry{}, false
}

// countLines follows the text path: a trailing newline is not a line
func countLines(s string) int {
	n := strings.Count(s, "\n") + 1
	if strings.HasSuffix(s, "\n") {
		n--
	}
	return n
}

func (s *Seen) load() map[string]seenEntry {
	m := map[string]seenEntry{}
	b, err := os.ReadFile(s.path)
	if err != nil {
		return m
	}
	if err := json.Unmarshal(b, &m); err != nil {
		return map[string]seenEntry{}
	}
	return m
}

// save rewrites the index atomically; a lost race only costs a missed hit
func (s *Seen) save(m map[string]seenEntry) {
	if len(m) > seenMax {
		m = newest(m, seenMax)
	}
	b, err := json.Marshal(m)
	if err != nil {
		return
	}
	tmp := s.path + ".tmp"
	if os.WriteFile(tmp, b, 0o600) != nil {
		return
	}
	if os.Rename(tmp, s.path) != nil {
		_ = os.Remove(tmp)
		return
	}
	s.gc()
}

func newest(m map[string]seenEntry, n int) map[string]seenEntry {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool { return m[keys[i]].TS.After(m[keys[j]].TS) })
	out := make(map[string]seenEntry, n)
	for _, k := range keys[:n] {
		out[k] = m[k]
	}
	return out
}

// gc drops indexes for sessions that ended long ago, best effort
func (s *Seen) gc() {
	if s.ttl <= 0 {
		return
	}
	cutoff := time.Now().Add(-s.ttl)
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return
	}
	for _, e := range entries {
		if filepath.Ext(e.Name()) != ".json" {
			continue
		}
		info, err := e.Info()
		if err != nil || !info.ModTime().Before(cutoff) {
			continue
		}
		_ = os.Remove(filepath.Join(s.dir, e.Name()))
	}
}
