// SPDX-License-Identifier: Apache-2.0

package isthmos

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestCorpusSavings guards the starter rules against regressions on synthetic payloads
func TestCorpusSavings(t *testing.T) {
	rs := LoadRules("rules.example.json")
	files, err := filepath.Glob("testdata/corpus/*.json")
	if err != nil || len(files) == 0 {
		t.Fatalf("no corpus files: %v", err)
	}
	for _, f := range files {
		in, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		tool := strings.TrimSuffix(filepath.Base(f), ".json")
		out, changed := Apply(rs, tool, in)
		saved := 100 * float64(len(in)-len(out)) / float64(len(in))
		t.Logf("%-45s in=%-7d out=%-7d saved=%.1f%%", tool, len(in), len(out), saved)
		if !changed || saved < 30 {
			t.Errorf("%s: starter rules saved %.1f%%, want at least 30%%", tool, saved)
		}
		if _, err := decode(out); err != nil {
			t.Errorf("%s: output is not valid JSON: %v", tool, err)
		}
	}
}
