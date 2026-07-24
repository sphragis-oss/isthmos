// SPDX-License-Identifier: Apache-2.0

package isthmos

import (
	"bufio"
	"encoding/json"
	"io"
	"sort"
	"time"
)

type Measure struct {
	TS       time.Time `json:"ts"`
	Tool     string    `json:"tool"`
	InBytes  int       `json:"in_bytes"`
	OutBytes int       `json:"out_bytes"`
	Reveal   bool      `json:"reveal,omitempty"`
}

type ToolStat struct {
	Tool     string
	Calls    int
	InBytes  int64
	OutBytes int64
	Reveals  int
}

func (s ToolStat) Saved() int64 { return s.InBytes - s.OutBytes }

// EstTokens is a rough 4-bytes-per-token estimate, not a tokenizer
func EstTokens(b int64) int64 { return b / 4 }

// Aggregate sums measurements per tool, skipping lines it cannot parse
func Aggregate(r io.Reader, since time.Time) []ToolStat {
	byTool := map[string]*ToolStat{}
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for sc.Scan() {
		var m Measure
		if err := json.Unmarshal(sc.Bytes(), &m); err != nil {
			continue
		}
		if !since.IsZero() && m.TS.Before(since) {
			continue
		}
		st, ok := byTool[m.Tool]
		if !ok {
			st = &ToolStat{Tool: m.Tool}
			byTool[m.Tool] = st
		}
		if m.Reveal {
			st.Reveals++
			continue
		}
		st.Calls++
		st.InBytes += int64(m.InBytes)
		st.OutBytes += int64(m.OutBytes)
	}
	out := make([]ToolStat, 0, len(byTool))
	for _, st := range byTool {
		out = append(out, *st)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Saved() != out[j].Saved() {
			return out[i].Saved() > out[j].Saved()
		}
		return out[i].Tool < out[j].Tool
	})
	return out
}
