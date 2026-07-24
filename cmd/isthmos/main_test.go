// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sphragis-oss/isthmos"
)

func setupEnv(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	rules := filepath.Join(home, "rules.json")
	if err := os.WriteFile(rules, []byte(`{"rules":[{"tool":"mcp__*","drop_keys":["noise"]}]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("ISTHMOS_RULES", rules)
	t.Setenv("ISTHMOS_SHADOW", "")
	return home
}

func hookStdin(t *testing.T) *bytes.Buffer {
	t.Helper()
	in, err := json.Marshal(hookInput{
		ToolName:     "mcp__github__x",
		ToolResponse: json.RawMessage(`{"noise":"xxxxxxxxxxxxxxxxxxxxxxxx","keep":"y"}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	return bytes.NewBuffer(in)
}

func TestHookRewrites(t *testing.T) {
	home := setupEnv(t)
	var out bytes.Buffer
	runHook(hookStdin(t), &out)
	var res hookOutput
	if err := json.Unmarshal(out.Bytes(), &res); err != nil {
		t.Fatalf("hook emitted no valid output: %v", err)
	}
	if strings.Contains(string(res.HookSpecificOutput.UpdatedToolOutput), "noise") {
		t.Fatalf("dropped key survived: %s", res.HookSpecificOutput.UpdatedToolOutput)
	}
	if _, err := os.Stat(filepath.Join(home, ".local", "state", "isthmos", "measure.jsonl")); err != nil {
		t.Fatalf("measurement not logged: %v", err)
	}
}

func TestHookShadowEmitsNothing(t *testing.T) {
	home := setupEnv(t)
	t.Setenv("ISTHMOS_SHADOW", "1")
	var out bytes.Buffer
	runHook(hookStdin(t), &out)
	if out.Len() != 0 {
		t.Fatalf("shadow mode must not rewrite: %s", out.String())
	}
	b, err := os.ReadFile(filepath.Join(home, ".local", "state", "isthmos", "measure.jsonl"))
	if err != nil {
		t.Fatalf("shadow mode must still measure: %v", err)
	}
	var m isthmos.Measure
	if err := json.Unmarshal(bytes.TrimSpace(b), &m); err != nil {
		t.Fatalf("bad measure line: %s", b)
	}
	if m.OutBytes >= m.InBytes {
		t.Fatalf("shadow measure shows no savings: %s", b)
	}
	if _, err := os.Stat(filepath.Join(home, ".local", "state", "isthmos", "store")); !os.IsNotExist(err) {
		t.Fatal("shadow mode must not touch the store")
	}
}

func TestHookFailOpenOnGarbage(t *testing.T) {
	setupEnv(t)
	var out bytes.Buffer
	runHook(bytes.NewBufferString("not json"), &out)
	if out.Len() != 0 {
		t.Fatalf("garbage input must produce no output: %s", out.String())
	}
}

func TestFilterShadowPassthrough(t *testing.T) {
	setupEnv(t)
	t.Setenv("ISTHMOS_SHADOW", "1")
	in := `{"noise":"xxxxxxxxxxxxxxxxxxxxxxxx","keep":"y"}`
	var out bytes.Buffer
	runFilter([]string{"-tool", "mcp__github__x"}, bytes.NewBufferString(in), &out)
	if out.String() != in {
		t.Fatalf("shadow filter must pass through untouched: %s", out.String())
	}
}

func TestDoctorHealthy(t *testing.T) {
	setupEnv(t)
	var out bytes.Buffer
	if code := runDoctor(&out); code != 0 {
		t.Fatalf("healthy setup reported code %d: %s", code, out.String())
	}
	if !strings.Contains(out.String(), "ok, 1 rules") {
		t.Fatalf("doctor missed the rules file: %s", out.String())
	}
}

func TestDoctorFailsOnBadRules(t *testing.T) {
	home := setupEnv(t)
	rules := filepath.Join(home, "rules.json")
	if err := os.WriteFile(rules, []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if code := runDoctor(&out); code != 1 {
		t.Fatalf("broken rules must fail doctor: %s", out.String())
	}
	if !strings.Contains(out.String(), "FAIL") {
		t.Fatalf("doctor output has no FAIL line: %s", out.String())
	}
}

func TestDoctorFailsOnAllZeroPayloads(t *testing.T) {
	home := setupEnv(t)
	dir := filepath.Join(home, ".local", "state", "isthmos")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	var lines bytes.Buffer
	enc := json.NewEncoder(&lines)
	for i := 0; i < 6; i++ {
		if err := enc.Encode(isthmos.Measure{TS: time.Now().UTC(), Tool: "Bash"}); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "measure.jsonl"), lines.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if code := runDoctor(&out); code != 1 {
		t.Fatalf("all-zero payloads must fail doctor: %s", out.String())
	}
	if !strings.Contains(out.String(), "payload: FAIL") {
		t.Fatalf("doctor output has no payload FAIL line: %s", out.String())
	}
}

func TestMeasureTrimKeepsNewestLines(t *testing.T) {
	setupEnv(t)
	old := measureCap
	measureCap = 2048
	t.Cleanup(func() { measureCap = old })
	for i := 0; i < 100; i++ {
		logMeasure("Bash", i, i)
	}
	fi, err := os.Stat(measurePath())
	if err != nil {
		t.Fatal(err)
	}
	if fi.Size() > measureCap+256 {
		t.Fatalf("log not trimmed: %d bytes", fi.Size())
	}
	b, err := os.ReadFile(measurePath())
	if err != nil {
		t.Fatal(err)
	}
	var m isthmos.Measure
	if err := json.Unmarshal(bytes.SplitN(b, []byte("\n"), 2)[0], &m); err != nil {
		t.Fatalf("first line after trim is not valid JSON: %v", err)
	}
	if !bytes.Contains(b, []byte(`"in_bytes":99`)) {
		t.Fatalf("newest line lost by trim")
	}
}

func TestFilterRewrites(t *testing.T) {
	setupEnv(t)
	var out bytes.Buffer
	runFilter([]string{"-tool", "mcp__github__x"}, bytes.NewBufferString(`{"noise":"xxxxxxxxxxxxxxxxxxxxxxxx","keep":"y"}`), &out)
	if strings.Contains(out.String(), "noise") {
		t.Fatalf("dropped key survived: %s", out.String())
	}
}
