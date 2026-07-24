# isthmos

<p align="center">
  <picture>
    <source media="(prefers-color-scheme: dark)" srcset="assets/logo-dark.svg">
    <img alt="isthmos" src="assets/logo-light.svg" width="128">
  </picture>
</p>

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
PostToolUse hook that rewrites `tool_response` via `updatedToolOutput`, and a
generic `filter` mode that works with any agent or CLI that can pipe through a
command. Nothing leaves your machine and nothing sits in the credential path.

## Status

Early but working: rule-based JSON field pruning, text compression for
plain-text payloads, plus byte-level measurement.

## Why

Verbose tool outputs (fat MCP JSON, log dumps, API payloads) can be a large
share of an agent's context on some workflows and a rounding error on others.
Which one your machine has is an empirical question, so isthmos ships
measurement first: shadow mode and a per-call byte log show what pruning would
save on your traffic before anything is rewritten. A per-tool saving is a
local percentage, not a whole-task cost reduction; isthmos claims neither and
reports both the local number and its share of everything it measured.

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
untouched unless a matching rule sets the text limits below.

### Checking your setup

```
$ isthmos doctor
version: 0.1.0
rules:   /Users/you/.config/isthmos/rules.json: ok, 4 rules
store:   ok, 12 entries
measure: /Users/you/.local/state/isthmos/measure.jsonl: 84.2KB, last write 2026-07-21T09:14:02Z
hook:    wired in ~/.claude/settings.json
shadow:  off
```

Exits non-zero when something is actually broken (unreadable or invalid rules,
unusable store, or a hook that fires but only ever receives empty payloads,
which means the wiring or input field is wrong); a missing rules file is just
reported, since no rules means isthmos is a deliberate no-op.

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

Text payloads get their own limits: `max_lines` keeps head and tail lines
with the same reversible marker, and `dedup` collapses runs of 3 or more
identical lines into a labelled count. Error-looking lines (`error`, `fatal`,
`panic`, `traceback`, ...) are never dropped by `max_lines`. Both apply to
raw non-JSON payloads, to a JSON string carrying text, and to long strings
embedded in JSON objects, which is where real hook payloads keep their text
(`stdout` for Bash, `file.content` for Read).

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
with before/after byte counts per tool, including calls the rules left
untouched, so pruning rules are driven by real data, not guesses. The log is
capped at 5MB; when it grows past that, the oldest half is trimmed.
`isthmos stats` turns that log into a savings table (illustrative output):

```
$ isthmos stats -since 168h
TOOL                                      CALLS  IN     OUT    SAVED  SAVED%  %ALL   ~TOKENS  REVEALS
mcp__atlassian__searchJiraIssuesUsingJql  42     1.9MB  0.6MB  1.3MB  68.4%   68.0%  340787   3
mcp__github__get_me                       7      12.3KB 4.1KB  8.2KB  66.7%   0.4%   2099     0
TOTAL                                     49     1.9MB  0.6MB  1.3MB  68.4%   68.4%  342886   3
scope: only tool calls that reached isthmos; whole-session context is a larger denominator
```

`SAVED%` is local to that tool; `%ALL` is the same saving as a share of every
byte isthmos measured in the window, so a flashy local percentage cannot pose
as an overall one. Neither is a session-level or dollar figure: tools your
hook matcher never routes to isthmos are not in the log, and published agent
traces show repeated context (system prompt, history) is typically the far
larger consumer.
`-file` points at a different log, `-since` bounds the window. The `~TOKENS`
column is a rough 4-bytes-per-token estimate, not a tokenizer.

`REVEALS` counts `isthmos reveal` recoveries attributed to each tool. A reveal
means a rule cut something the agent then had to fetch back, paying an extra
tool call, so a tool with a rising reveal count is over-pruned: loosen its
rule instead of celebrating its `SAVED%`.

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
