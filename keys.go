// SPDX-License-Identifier: Apache-2.0

package isthmos

import (
	"bufio"
	"encoding/json"
	"io"
	"path"
	"sort"
	"time"
)

// KeyMeasure is one call's byte weight per key name; names only, never values
type KeyMeasure struct {
	TS      time.Time        `json:"ts"`
	Tool    string           `json:"tool"`
	InBytes int              `json:"in_bytes"`
	Keys    map[string]int64 `json:"keys"`
}

type KeyStat struct {
	Key   string
	Calls int
	Bytes int64
}

// KeyBytes estimates the bytes each key name accounts for, nested values included
func KeyBytes(raw json.RawMessage) map[string]int64 {
	v, err := decode(raw)
	if err != nil {
		return nil
	}
	if s, ok := v.(string); ok {
		if v, err = decode([]byte(s)); err != nil {
			return nil
		}
	}
	acc := map[string]int64{}
	keySize(v, acc)
	return acc
}

func keySize(v any, acc map[string]int64) int64 {
	switch t := v.(type) {
	case map[string]any:
		n := int64(2)
		for k, val := range t {
			// quotes, colon and comma around the key
			sz := int64(len(k)) + 4 + keySize(val, acc)
			acc[k] += sz
			n += sz
		}
		return n
	case []any:
		n := int64(2)
		for _, val := range t {
			n += keySize(val, acc) + 1
		}
		return n
	case string:
		return int64(len(t)) + 2
	case json.Number:
		return int64(len(t))
	default:
		return 4
	}
}

// TopKeys keeps the n heaviest keys, so one log line stays small
func TopKeys(m map[string]int64, n int) map[string]int64 {
	if len(m) <= n {
		return m
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool { return m[keys[i]] > m[keys[j]] })
	out := make(map[string]int64, n)
	for _, k := range keys[:n] {
		out[k] = m[k]
	}
	return out
}

// AggregateKeys sums key weights for tools matching the glob, with their total input bytes
func AggregateKeys(r io.Reader, since time.Time, toolGlob string) ([]KeyStat, int64) {
	byKey := map[string]*KeyStat{}
	var total int64
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for sc.Scan() {
		var m KeyMeasure
		if err := json.Unmarshal(sc.Bytes(), &m); err != nil {
			continue
		}
		if !since.IsZero() && m.TS.Before(since) {
			continue
		}
		if ok, _ := path.Match(toolGlob, m.Tool); !ok {
			continue
		}
		total += int64(m.InBytes)
		for k, b := range m.Keys {
			st, ok := byKey[k]
			if !ok {
				st = &KeyStat{Key: k}
				byKey[k] = st
			}
			st.Calls++
			st.Bytes += b
		}
	}
	out := make([]KeyStat, 0, len(byKey))
	for _, st := range byKey {
		out = append(out, *st)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Bytes != out[j].Bytes {
			return out[i].Bytes > out[j].Bytes
		}
		return out[i].Key < out[j].Key
	})
	return out, total
}
