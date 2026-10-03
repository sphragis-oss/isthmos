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

func TestDoctorWarnsOnDeadRule(t *testing.T) {
	home := setupEnv(t)
	rules := filepath.Join(home, "rules.json")
	if err := os.WriteFile(rules, []byte(`{"rules":[{"tool":"mcp__*","drop_keys":["noise"]},{"tool":"Read","max_lines":100}]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	settings := `{"hooks":{"PostToolUse":[{"matcher":"mcp__.*","hooks":[{"type":"command","command":"isthmos hook"}]}]}}`
	if err := os.MkdirAll(filepath.Join(home, ".claude"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, ".claude", "settings.json"), []byte(settings), 0o600); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if code := runDoctor(&out); code != 0 {
		t.Fatalf("dead rule is a warning, not a failure: %s", out.String())
	}
	if !strings.Contains(out.String(), `WARN rule "Read"`) {
		t.Fatalf("doctor missed the dead Read rule: %s", out.String())
	}
	if strings.Contains(out.String(), `WARN rule "mcp__*"`) {
		t.Fatalf("routed rule wrongly flagged: %s", out.String())
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

func dedupHook(t *testing.T, session, agent string) string {
	t.Helper()
	body, err := json.Marshal(strings.Repeat("some line of file content here\n", 300))
	if err != nil {
		t.Fatal(err)
	}
	in, err := json.Marshal(hookInput{SessionID: session, AgentID: agent, ToolName: "Read", ToolResponse: body})
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	runHook(bytes.NewBuffer(in), &out)
	return out.String()
}

func TestHookDedupIsScopedPerAgent(t *testing.T) {
	setupEnv(t)
	if out := dedupHook(t, "s1", "subagent-1"); out != "" {
		t.Fatalf("first sight must pass through: %s", out)
	}
	if out := dedupHook(t, "s1", ""); out != "" {
		t.Fatalf("the parent never saw the subagent's payload: %s", out)
	}
	if out := dedupHook(t, "s1", ""); !strings.Contains(out, "identical to an earlier") {
		t.Fatalf("a repeat in the same context must collapse: %s", out)
	}
}

func TestHookCompactionResetsDedup(t *testing.T) {
	home := setupEnv(t)
	dedupHook(t, "s1", "")
	runHook(strings.NewReader(`{"session_id":"s1","hook_event_name":"SessionStart","source":"compact"}`), &bytes.Buffer{})
	if out := dedupHook(t, "s1", ""); out != "" {
		t.Fatalf("after compaction the payload is gone from context: %s", out)
	}
	b, err := os.ReadFile(filepath.Join(home, ".local", "state", "isthmos", "measure.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if n := bytes.Count(b, []byte("\n")); n != 2 {
		t.Fatalf("SessionStart must not be measured as a tool call, got %d lines", n)
	}
}

func writeSettings(t *testing.T, home, settings string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(home, ".claude"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, ".claude", "settings.json"), []byte(settings), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestDoctorReadsShadowFromHookCommand(t *testing.T) {
	home := setupEnv(t)
	writeSettings(t, home, `{"hooks":{"PostToolUse":[{"matcher":"mcp__.*","hooks":[{"type":"command","command":"ISTHMOS_SHADOW=1 isthmos hook"}]}]}}`)
	var out bytes.Buffer
	runDoctor(&out)
	if !strings.Contains(out.String(), "shadow:  ON") {
		t.Fatalf("doctor ignored the hook's own env: %s", out.String())
	}
}

func TestDoctorWarnsWithoutCompactHook(t *testing.T) {
	home := setupEnv(t)
	tool := `"PostToolUse":[{"matcher":"mcp__.*","hooks":[{"type":"command","command":"isthmos hook"}]}]`
	writeSettings(t, home, `{"hooks":{`+tool+`}}`)
	var out bytes.Buffer
	runDoctor(&out)
	if !strings.Contains(out.String(), "WARN no SessionStart compact hook") {
		t.Fatalf("missing compact hook not flagged: %s", out.String())
	}
	writeSettings(t, home, `{"hooks":{`+tool+`,"SessionStart":[{"matcher":"compact","hooks":[{"type":"command","command":"isthmos hook"}]}]}}`)
	out.Reset()
	runDoctor(&out)
	if strings.Contains(out.String(), "WARN no SessionStart") {
		t.Fatalf("wired compact hook still flagged: %s", out.String())
	}
}

func TestDoctorFailsOnUnknownRuleField(t *testing.T) {
	home := setupEnv(t)
	if err := os.WriteFile(filepath.Join(home, "rules.json"), []byte(`{"rules":[{"tool":"t","drop_key":["x"]}]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if code := runDoctor(&out); code != 1 || !strings.Contains(out.String(), "drop_key") {
		t.Fatalf("typo'd field must fail doctor: %s", out.String())
	}
}

func TestDoctorFailsOnMalformedGlob(t *testing.T) {
	home := setupEnv(t)
	if err := os.WriteFile(filepath.Join(home, "rules.json"), []byte(`{"rules":[{"tool":"mcp__[","drop_keys":["x"]}]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if code := runDoctor(&out); code != 1 || !strings.Contains(out.String(), "malformed") {
		t.Fatalf("malformed glob must fail doctor: %s", out.String())
	}
}

func TestShadowLogsKeyProfile(t *testing.T) {
	home := setupEnv(t)
	t.Setenv("ISTHMOS_SHADOW", "1")
	runHook(hookStdin(t), &bytes.Buffer{})
	b, err := os.ReadFile(filepath.Join(home, ".local", "state", "isthmos", "keys.jsonl"))
	if err != nil {
		t.Fatalf("shadow mode must profile keys: %v", err)
	}
	if !strings.Contains(string(b), `"noise"`) || strings.Contains(string(b), "xxxx") {
		t.Fatalf("profile must hold key names and no values: %s", b)
	}
}

var zeroTime time.Time

func readState(t *testing.T, name string) string {
	t.Helper()
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(home, ".local", "state", "isthmos", name))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestHookKeepsBuiltinShape(t *testing.T) {
	home := setupEnv(t)
	if err := os.WriteFile(filepath.Join(home, "rules.json"), []byte(`{"rules":[{"tool":"*","drop_keys":["noise"]}]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	in, err := json.Marshal(hookInput{ToolName: "Bash", ToolResponse: json.RawMessage(`{"stdout":"ok","noise":"xxxxxxxxxxxxxxxxxxxxxxxx"}`)})
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	runHook(bytes.NewBuffer(in), &out)
	if out.Len() != 0 {
		t.Fatalf("a built-in tool must keep its keys: %s", out.String())
	}
	stats := isthmos.Aggregate(strings.NewReader(readState(t, "measure.jsonl")), zeroTime)
	if len(stats) != 1 || stats[0].Saved() != 0 {
		t.Fatalf("a rewrite Claude Code would discard must not be logged as a saving: %+v", stats)
	}
}

func TestDoctorWarnsOnShapeRuleForBuiltin(t *testing.T) {
	home := setupEnv(t)
	if err := os.WriteFile(filepath.Join(home, "rules.json"), []byte(`{"rules":[{"tool":"Bash","drop_empty":true},{"tool":"mcp__*","drop_keys":["noise"]},{"tool":"Read","max_lines":100}]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	writeSettings(t, home, `{"hooks":{"PostToolUse":[{"matcher":"mcp__.*|Bash|Read","hooks":[{"type":"command","command":"isthmos hook"}]}]}}`)
	var out bytes.Buffer
	runDoctor(&out)
	if !strings.Contains(out.String(), `WARN rule "Bash": drop_keys`) {
		t.Fatalf("shape rule on a built-in not flagged: %s", out.String())
	}
	if strings.Contains(out.String(), `WARN rule "mcp__*"`) || strings.Contains(out.String(), `WARN rule "Read"`) {
		t.Fatalf("a safe rule was flagged: %s", out.String())
	}
}
