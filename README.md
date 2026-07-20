# isthmos

<p align="center">
  <a href="https://github.com/sphragis-oss/isthmos/actions/workflows/ci.yml?query=branch%3Amain">
    <img alt="Build Status" src="https://img.shields.io/github/actions/workflow/status/sphragis-oss/isthmos/ci.yml?branch=main&style=for-the-badge&label=tests">
  </a>
  <a href="https://github.com/sphragis-oss/isthmos/releases">
    <img alt="Latest Release" src="https://img.shields.io/github/v/release/sphragis-oss/isthmos?include_prereleases&style=for-the-badge">
  </a>
  <a href="https://scorecard.dev/viewer/?uri=github.com/sphragis-oss/isthmos">
    <img alt="OpenSSF Scorecard" src="https://img.shields.io/ossf-scorecard/github.com/sphragis-oss/isthmos?label=openssf%20scorecard&style=for-the-badge">
  </a>
  <a href="LICENSE">
    <img alt="License" src="https://img.shields.io/badge/License-Apache%202.0-blue.svg?style=for-the-badge">
  </a>
</p>

isthmos (Greek: ισθμός), the narrow passage your tool outputs squeeze through.

A local context-compression layer for agent tool outputs. The core is
agent-agnostic: JSON field pruning driven by per-tool rules, importable as a Go
package. Adapters connect it to whatever runs your LLM: a native Claude Code
PostToolUse hook that rewrites `tool_output` via `updatedToolOutput`, and a
generic `filter` mode that works with any agent or CLI that can pipe through a
command. Nothing leaves your machine and nothing sits in the credential path.

## Status

Phase 1 shipped: rule-based JSON field pruning plus byte-level measurement.

## Why

Agent context spend is dominated by tool outputs (verbose MCP JSON, log dumps,
API payloads), yet most token hygiene tooling only targets the input/prefix
side. isthmos targets the output side, at the only point where the payload is
visible before the model reads it.

## Usage

### Claude Code (native hook)

```json
{
  "hooks": {
    "PostToolUse": [
      {
        "matcher": "mcp__.*",
        "hooks": [
          {"type": "command", "command": "$HOME/.local/bin/isthmos hook", "timeout": 5}
        ]
      }
    ]
  }
}
```

### Any other agent (generic filter)

```sh
some-tool --json | isthmos filter -tool mcp__github__search_repos
```

Stdin in, pruned stdout out. Wire it into any wrapper, shell function, or
orchestrator that can interpose a pipe. Non-JSON payloads pass through
untouched.

### As a Go library

```go
import "github.com/sphragis-oss/isthmos"

rs := isthmos.LoadRules(path)
out, changed := isthmos.Apply(rs, toolName, rawOutput)
```

## Configuration

Rules live in `~/.config/isthmos/rules.json` (override with `ISTHMOS_RULES`).
Tool names are glob-matched, listed keys are dropped recursively:

```json
{
  "rules": [
    {"tool": "mcp__github__*", "drop_keys": ["node_id", "avatar_url"]}
  ]
}
```

See `rules.example.json` for a starter set covering Atlassian and GitHub MCP
noise fields. No config means no rewriting: isthmos is fail-open and only ever
emits a replacement when the result is strictly smaller.

## Measurement

Every invocation appends one line to `~/.local/state/isthmos/measure.jsonl`
with before/after byte counts per tool, so pruning rules are driven by real
data, not guesses.

## Planned phases

1. ~~JSON field pruning for MCP tool outputs~~ (shipped)
2. Measurement: real before/after token accounting
3. Generic compressors per tool family
4. Reversibility store: recover full payloads on demand
5. Text compression

## Design constraints

- No proxy in the credential path
- One static binary, fast cold start (runs on every tool call)
- Fail-open: any error means untouched passthrough
- Lossy steps must be reversible or clearly labelled

## Contributing

Issues and pull requests are welcome. See [CONTRIBUTING.md](CONTRIBUTING.md) for
setup, style, and the DCO sign-off requirement, and
[GOVERNANCE.md](GOVERNANCE.md) for how decisions get made. Security reports go
through [SECURITY.md](SECURITY.md), never a public issue.

Pruning rules are the highest-value contribution: bring one backed by real
before/after byte counts.

## License

Apache-2.0
