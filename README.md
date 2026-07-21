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

Early but working: rule-based JSON field pruning plus byte-level measurement.

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
    {"tool": "mcp__github__*", "drop_keys": ["node_id", "avatar_url"], "max_items": 30},
    {"tool": "mcp__*", "max_str": 8000}
  ]
}
```

Besides `drop_keys`, a rule can cap payload size generically: `max_items`
truncates any array beyond N elements and `max_str` truncates any string beyond
N bytes (at a rune boundary). Both replace the removed tail with an explicit
`[isthmos: ... truncated]` marker so the model knows the payload is partial;
truncation is never silent. When several rules match, the strictest positive
limit wins.

Truncation is head-and-tail, not naive: `keep_last` reserves part of the
`max_items` budget for the newest entries, and items that look like errors
(a truthy `error` field, or `status`/`level`/`conclusion` values such as
`failed` or `fatal`) are always kept regardless of position, because those are
the items an agent is usually looking for. `min_bytes` gates a whole rule:
payloads smaller than it pass through untouched, so tiny outputs are never
rewritten.

See `rules.example.json` for a starter set covering Atlassian and GitHub MCP
noise fields. No config means no rewriting: isthmos is fail-open and only ever
emits a replacement when the result is strictly smaller.

## Measurement

Every invocation appends one line to `~/.local/state/isthmos/measure.jsonl`
with before/after byte counts per tool, so pruning rules are driven by real
data, not guesses. `isthmos stats` turns that log into a savings table:

```
$ isthmos stats -since 168h
TOOL                                      CALLS  IN     OUT    SAVED  SAVED%  ~TOKENS
mcp__atlassian__searchJiraIssuesUsingJql  42     1.9MB  0.6MB  1.3MB  68.4%   340787
mcp__github__get_me                       7      12.3KB 4.1KB  8.2KB  66.7%   2099
TOTAL                                     49     1.9MB  0.6MB  1.3MB  68.4%   342886
```

`-file` points at a different log, `-since` bounds the window. The `~TOKENS`
column is a rough 4-bytes-per-token estimate, not a tokenizer.

### Shadow mode

Set `ISTHMOS_SHADOW=1` to measure without rewriting: isthmos computes what the
rules would save and logs it, but the hook emits nothing and `filter` passes
stdin through untouched. Nothing is written to the reversibility store. Use it
to trial rules on a new machine, then unset it once `isthmos stats` shows the
savings are worth it:

```json
{"type": "command", "command": "ISTHMOS_SHADOW=1 $HOME/.local/bin/isthmos hook", "timeout": 5}
```

## Reversibility

Truncation is reversible. When items or bytes are cut, the original payload is
encrypted (AES-256-GCM) into `~/.local/state/isthmos/store/` and the marker
carries the recovery command:

```
[isthmos: 17 of 20 items truncated, full: isthmos reveal de6ac0410501901b]
```

An agent that needs the full payload can simply run that command; a human can
too. Entries expire after 7 days. If the store cannot be written, isthmos does
not truncate at all: a marker must never point at a payload that was not
stored. Field-pruned payloads (no truncation) are not stored.

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
