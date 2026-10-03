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
PostToolUse hook that rewrites `tool_response` via `updatedToolOutput`, an `mcp`
wrapper that prunes the results of any stdio MCP server for any client, and a
generic `filter` mode that works with any agent or CLI that can pipe through a
command. Nothing leaves your machine. The hook and the filter never see a
credential; the `mcp` wrapper starts the server itself, so the server inherits
its environment through isthmos, which reads none of it.

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

## Install

Via Homebrew (macOS / Linux):

```sh
brew install --cask sphragis-oss/sphragis/isthmos
```

Or with Go:

```sh
go install github.com/sphragis-oss/isthmos/cmd/isthmos@latest
```

Or from source:

```sh
git clone https://github.com/sphragis-oss/isthmos.git
cd isthmos
make install PREFIX=$HOME/.local
```

Release artifacts ship with checksums, a cosign-signed bundle, SBOMs, and build
provenance; [SECURITY.md](SECURITY.md) shows how to verify them. The hook
example below points at `$HOME/.local/bin/isthmos`; adjust it to wherever your
install landed (`which isthmos`).

## Usage

### Claude Code (native hook)

Start in shadow mode. isthmos measures what its rules would save on your own
traffic and rewrites nothing, so the first week costs you no risk:

```json
{
  "hooks": {
    "PostToolUse": [
      {
        "matcher": "mcp__.*|Read|Bash|WebFetch|Grep",
        "hooks": [
          {"type": "command", "command": "ISTHMOS_SHADOW=1 $HOME/.local/bin/isthmos hook", "timeout": 5}
        ]
      }
    ],
    "SessionStart": [
      {
        "matcher": "compact",
        "hooks": [
          {"type": "command", "command": "$HOME/.local/bin/isthmos hook", "timeout": 5}
        ]
      }
    ]
  }
}
```

The `SessionStart` entry tells isthmos when the context was compacted, so
cross-call dedup forgets what the agent can no longer see. `doctor` warns when
it is missing.

Let it run, then read `isthmos stats`. Whether pruning is worth anything on
your machine is an empirical question and the answer varies a lot by workload:
MCP-heavy traffic is where field pruning pays, while a session dominated by
short `Bash` outputs is close to the worst case.

Only once the numbers justify it, drop `ISTHMOS_SHADOW=1` so isthmos rewrites
live:

```json
{"type": "command", "command": "$HOME/.local/bin/isthmos hook", "timeout": 5}
```

Narrow the matcher to the tools your stats show are actually worth pruning.
The text limits under [Configuration](#configuration) are truncation, not
compression, so treat them as the opt-in step: they are the fastest way to
save bytes and the only way to lose something you wanted.

### Any MCP client (stdio wrapper)

Put isthmos in front of a stdio MCP server in the client's own config, and
every tool result is pruned before the client sees it:

```json
{"command": "isthmos", "args": ["mcp", "-server", "github", "--", "github-mcp-server", "stdio"]}
```

Rules see the tools as `mcp__<server>__<tool>`, the same names the Claude Code
hook uses, so one rules file serves both. Requests and every other message
pass through byte for byte; only the text of `tools/call` results is
rewritten, and `ISTHMOS_SHADOW=1` measures without rewriting here too. A
result's `structuredContent` is left as the server sent it.

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
version: 0.3.0
rules:   /Users/you/.config/isthmos/rules.json: ok, 4 rules
store:   ok, 12 entries
measure: /Users/you/.local/state/isthmos/measure.jsonl: 84.2KB, last write 2026-07-21T09:14:02Z
hook:    wired in ~/.claude/settings.json
shadow:  ON, measuring only, nothing is rewritten
next:    let it run, then read the numbers with: isthmos stats
```

`doctor` looks for the hook in `~/.claude/settings.json` and in the current
project's `.claude/settings.json` and `.claude/settings.local.json`, and reads
shadow mode from the hook command itself, not from your shell.

Exits non-zero when something is actually broken (unreadable or invalid rules,
a misspelled rule field or malformed tool glob, an
unusable store, or a hook that fires but only ever receives empty payloads,
which means the wiring or input field is wrong); a missing rules file is just
reported, since no rules means isthmos is a deliberate no-op. Once shadow mode
is off, `doctor` also warns when reveals are piling up, the signal that a rule
is cutting more than the agent can do without.

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

A bare `drop_keys` entry matches that key at any depth. A dotted entry is
scoped to its parents: `user.url` drops `url` only directly under `user`, and
leaves every other `url` alone.

`keep_keys` is the allowlist form, for payloads where naming what you want is
shorter than naming the noise. A listed key keeps its whole subtree; outside a
kept subtree, plain values are dropped and objects and arrays survive only as
far as they lead to a kept key:

```json
{"tool": "mcp__atlassian__search*", "keep_keys": ["key", "summary", "status", "displayName"]}
```

Two lossless steps cost nothing to turn on. `drop_empty` removes object
fields whose value is `null`, `""`, `[]` or `{}`. `tabular` rewrites an array
of 3 or more same-shaped objects as `{"isthmos_table": {"cols": [...], "rows":
[[...], ...]}}`, so the key names are sent once, not once per item. Objects
whose key sets differ are left as they are, which is what `drop_empty` tends
to produce, so pick one of the two per tool.

In the Claude Code hook, `drop_keys`, `keep_keys`, `drop_empty` and `tabular`
apply to MCP tools only. Claude Code checks a built-in tool's replacement
against that tool's output schema and silently keeps the original when the
shape differs, so for `Bash`, `Read` and the other built-ins isthmos restricts
itself to edits that keep the shape: the text limits, `max_str`, cross-call
dedup, and `max_items` on arrays of strings. `doctor` warns about a rule that
asks for more. `filter` and `mcp` mode apply every rule as written.

Besides `drop_keys`, a rule can cap payload size generically: `max_items`
truncates any array beyond N elements and `max_str` truncates any string beyond
N bytes (at a rune boundary). Both replace the removed tail with an explicit
`[isthmos: ... truncated]` marker so the model knows the payload is partial;
truncation is never silent. When several rules match, the strictest positive
limit wins.

Text payloads get their own limits: `max_lines` keeps head and tail lines
with the same reversible marker, and `dedup` collapses runs of 3 or more
identical lines into a labelled count. Error-looking lines (`error`, `fatal`,
`panic`, `traceback`, ...) are pinned past the `max_lines` budget, up to
`max_lines` extra lines, so a log made of nothing but errors still shrinks.
Both apply to
raw non-JSON payloads, to a JSON string carrying text, and to long strings
embedded in JSON objects, which is where real hook payloads keep their text
(`stdout` for Bash, `file.content` for Read).

Truncation is head-and-tail, not naive: `keep_last` reserves part of the
`max_items` budget for the newest entries, and items that look like errors
(a truthy `error` field, or `status`/`level`/`conclusion` values such as
`failed` or `fatal`) are kept regardless of position, up to `max_items` extra
items, because those are the items an agent is usually looking for.
`min_bytes` gates a whole rule:
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

### Which keys to drop

The savings table says which tool is heavy, not which fields. In shadow mode
isthmos also profiles each JSON payload by key name into
`~/.local/state/isthmos/keys.jsonl`: the 20 heaviest keys per call with their
byte weight, names only, never values. `isthmos stats -keys` turns that into
the list a `drop_keys` or `keep_keys` rule should be written from
(illustrative output):

```
$ isthmos stats -keys 'mcp__github__*'
KEY         CALLS  BYTES    SHARE
user        42     61.3KB   58.3%
avatar_url  42     6.9KB    6.6%
scope: a key's bytes include everything nested under it, so rows overlap and do not sum to 100%
```

### Reproducible numbers

`make bench` applies `rules.example.json` to the payloads in
`testdata/corpus/` and prints bytes in, bytes out and the saving per tool. The
corpus is synthetic, shaped like GitHub and Jira MCP responses, so it guards
the starter rules against regressions and says nothing about your traffic;
shadow mode is what measures that.

### Shadow mode

`ISTHMOS_SHADOW=1` is the recommended way to start, as
[above](#claude-code-native-hook). isthmos computes what the rules would save
and logs it, but the hook emits nothing and `filter` passes stdin through
untouched. Nothing is written to the reversibility store, and nothing can be
lost. Unset it once `isthmos stats` shows the savings are worth it.

### Sharing your numbers

`isthmos stats -share` replaces third-party tool names with stable placeholders
(`mcp__server1__*`), so a report can be pasted into a public issue. The
measurement log never contains file paths, arguments, or payload contents, only
byte counts, so the redacted table is the whole of what leaves your machine.

```
$ isthmos stats -share
TOOL              CALLS  IN     OUT    SAVED  SAVED%  %ALL   ~TOKENS  REVEALS
mcp__server1__*   42     1.9MB  0.6MB  1.3MB  68.4%   68.0%  340787   3
Read              93     0.9MB  0.5MB  0.4MB  41.1%   27.3%  99264    0
```

Workload is the thing nobody can guess for you, so results from a workload
unlike the maintainer's are the most useful thing you can contribute.

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
stored. Field-pruned payloads (no truncation) are not stored: `drop_keys` and
`keep_keys` remove exactly the fields you configured, with no marker and no
way back short of re-running the tool.

Cross-call dedup follows the same rule. A payload the agent was already sent
is replaced by a reference, and the reference carries a reveal id. The index
is kept per session and per subagent, since a subagent shares the session id
but not the parent's context, and it is cleared when the session is compacted.

## Design constraints

- No network proxy and no credential handling; the `mcp` wrapper is a local stdio pipe
- One static binary, fast cold start (runs on every tool call)
- Fail-open: any error means untouched passthrough
- Truncation and dedup are reversible and labelled; field removal is explicit configuration
- Values are never altered: numbers are re-emitted exactly as written

## Contributing

Issues and pull requests are welcome. See [CONTRIBUTING.md](CONTRIBUTING.md) for
setup, style, and the DCO sign-off requirement, and
[GOVERNANCE.md](GOVERNANCE.md) for how decisions get made. Security reports go
through [SECURITY.md](SECURITY.md), never a public issue.

Pruning rules are the highest-value contribution: bring one backed by real
before/after byte counts.

## License

Apache-2.0
