// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/sphragis-oss/isthmos"
)

func mcpRoundTrip(t *testing.T, request, response string) string {
	t.Helper()
	inflight := &calls{m: map[string]string{}}
	var toServer, toClient bytes.Buffer
	trackCalls(strings.NewReader(request), &toServer, inflight)
	if toServer.String() != request {
		t.Fatalf("requests must reach the server untouched: %s", toServer.String())
	}
	pruneResults("github", strings.NewReader(response), &toClient, inflight)
	return toClient.String()
}

const mcpCall = `{"jsonrpc":"2.0","id":7,"method":"tools/call","params":{"name":"x","arguments":{}}}` + "\n"

func TestMCPPrunesToolResultText(t *testing.T) {
	setupEnv(t)
	text, err := json.Marshal(`{"noise":"xxxxxxxxxxxxxxxxxxxxxxxx","keep":"y"}`)
	if err != nil {
		t.Fatal(err)
	}
	resp := `{"jsonrpc":"2.0","id":7,"result":{"content":[{"type":"text","text":` + string(text) + `}],"isError":false}}` + "\n"
	out := mcpRoundTrip(t, mcpCall, resp)
	var msg struct {
		ID     int `json:"id"`
		Result struct {
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
			IsError *bool `json:"isError"`
		} `json:"result"`
	}
	if err := json.Unmarshal([]byte(out), &msg); err != nil {
		t.Fatalf("rewritten message is not valid JSON: %v: %s", err, out)
	}
	if msg.ID != 7 || msg.Result.IsError == nil || len(msg.Result.Content) != 1 {
		t.Fatalf("envelope fields lost: %s", out)
	}
	if msg.Result.Content[0].Text != `{"keep":"y"}` {
		t.Fatalf("text not pruned: %s", msg.Result.Content[0].Text)
	}
}

func TestMCPLeavesOtherMessagesAlone(t *testing.T) {
	setupEnv(t)
	for _, line := range []string{
		`{"jsonrpc":"2.0","id":99,"result":{"content":[{"type":"text","text":"{\"noise\":\"xxxxxxxxxxxxxxxx\"}"}]}}` + "\n",
		`{"jsonrpc":"2.0","method":"notifications/progress","params":{}}` + "\n",
		"not json at all\n",
	} {
		if out := mcpRoundTrip(t, mcpCall, line); out != line {
			t.Fatalf("untracked message was rewritten: %s", out)
		}
	}
}

func TestMCPShadowMeasuresWithoutRewriting(t *testing.T) {
	setupEnv(t)
	t.Setenv("ISTHMOS_SHADOW", "1")
	resp := `{"jsonrpc":"2.0","id":7,"result":{"content":[{"type":"text","text":"{\"noise\":\"xxxxxxxxxxxxxxxx\",\"keep\":\"y\"}"}]}}` + "\n"
	if out := mcpRoundTrip(t, mcpCall, resp); out != resp {
		t.Fatalf("shadow mode rewrote the result: %s", out)
	}
	f := strings.NewReader(readState(t, "measure.jsonl"))
	stats := isthmos.Aggregate(f, zeroTime)
	if len(stats) != 1 || stats[0].Tool != "mcp__github__x" || stats[0].Saved() <= 0 {
		t.Fatalf("shadow saving not measured: %+v", stats)
	}
}
