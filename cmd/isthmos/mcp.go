// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"sync"

	"github.com/sphragis-oss/isthmos"
)

type rpcMsg struct {
	ID     json.RawMessage `json:"id"`
	Method string          `json:"method"`
	Params struct {
		Name string `json:"name"`
	} `json:"params"`
}

// calls maps in-flight tools/call request ids to their tool names
type calls struct {
	mu sync.Mutex
	m  map[string]string
}

func (c *calls) put(id, tool string) {
	c.mu.Lock()
	c.m[id] = tool
	c.mu.Unlock()
}

func (c *calls) take(id string) (string, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	tool, ok := c.m[id]
	delete(c.m, id)
	return tool, ok
}

// runMCP wraps a stdio MCP server, pruning its tool results on the way back
func runMCP(args []string) int {
	fs := flag.NewFlagSet("mcp", flag.ExitOnError)
	server := fs.String("server", "", "server name, rules see tools as mcp__SERVER__TOOL")
	if err := fs.Parse(args); err != nil || *server == "" || fs.NArg() == 0 {
		fmt.Fprintln(os.Stderr, "usage: isthmos mcp -server NAME -- COMMAND [ARGS...]")
		return 2
	}
	cmd := exec.Command(fs.Arg(0), fs.Args()[1:]...) //nolint:gosec // running the given server is the feature
	cmd.Stderr = os.Stderr
	childIn, err := cmd.StdinPipe()
	if err != nil {
		slog.Error("mcp stdin pipe", "err", err)
		return 1
	}
	childOut, err := cmd.StdoutPipe()
	if err != nil {
		slog.Error("mcp stdout pipe", "err", err)
		return 1
	}
	if err := cmd.Start(); err != nil {
		slog.Error("start mcp server", "err", err)
		return 1
	}
	inflight := &calls{m: map[string]string{}}
	go func() {
		trackCalls(os.Stdin, childIn, inflight)
		_ = childIn.Close()
	}()
	pruneResults(*server, childOut, os.Stdout, inflight)
	var exit *exec.ExitError
	if err := cmd.Wait(); errors.As(err, &exit) {
		return exit.ExitCode()
	} else if err != nil {
		return 1
	}
	return 0
}

// eachLine hands over every newline-delimited message, whatever its size
func eachLine(r io.Reader, fn func([]byte) error) {
	br := bufio.NewReader(r)
	for {
		line, err := br.ReadBytes('\n')
		if len(line) > 0 && fn(line) != nil {
			return
		}
		if err != nil {
			return
		}
	}
}

// trackCalls forwards client requests untouched, noting which ids are tool calls
func trackCalls(client io.Reader, server io.Writer, inflight *calls) {
	eachLine(client, func(line []byte) error {
		var m rpcMsg
		if json.Unmarshal(line, &m) == nil && m.Method == "tools/call" && len(m.ID) > 0 {
			inflight.put(string(m.ID), m.Params.Name)
		}
		_, err := server.Write(line)
		return err
	})
}

// pruneResults forwards server messages, rewriting the text of tracked tool results
func pruneResults(server string, from io.Reader, client io.Writer, inflight *calls) {
	rs := isthmos.LoadRules(configPath())
	var st *isthmos.Store
	if !shadowMode() {
		st = openStore()
	}
	eachLine(from, func(line []byte) error {
		_, err := client.Write(pruneResult(rs, st, server, line, inflight))
		return err
	})
}

// pruneResult is fail-open: anything it cannot parse goes through as received
func pruneResult(rs isthmos.Rules, st *isthmos.Store, server string, line []byte, inflight *calls) []byte {
	var msg map[string]json.RawMessage
	if json.Unmarshal(line, &msg) != nil || len(msg["id"]) == 0 || len(msg["result"]) == 0 {
		return line
	}
	name, ok := inflight.take(string(msg["id"]))
	if !ok {
		return line
	}
	var result map[string]json.RawMessage
	var content []map[string]json.RawMessage
	if json.Unmarshal(msg["result"], &result) != nil || json.Unmarshal(result["content"], &content) != nil {
		return line
	}
	tool := "mcp__" + server + "__" + name
	changed := false
	for _, item := range content {
		var text string
		if json.Unmarshal(item["text"], &text) != nil || text == "" {
			continue
		}
		logKeys(tool, json.RawMessage(text))
		out, ok := isthmos.ApplyWithStore(rs, tool, json.RawMessage(text), st)
		logMeasure(tool, len(text), len(out))
		if !ok || shadowMode() {
			continue
		}
		b, err := marshal(string(out))
		if err != nil {
			return line
		}
		item["text"], changed = b, true
	}
	if !changed {
		return line
	}
	var err error
	if result["content"], err = marshal(content); err != nil {
		return line
	}
	if msg["result"], err = marshal(result); err != nil {
		return line
	}
	b, err := marshal(msg)
	if err != nil {
		return line
	}
	return append(b, '\n')
}

// marshal skips HTML escaping, which would inflate the payload being shrunk
func marshal(v any) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return bytes.TrimSuffix(buf.Bytes(), []byte("\n")), nil
}
