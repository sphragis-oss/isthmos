# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- `isthmos mcp -server NAME -- COMMAND` wraps a stdio MCP server and prunes the text of its `tools/call` results, so the rules work in any MCP client. Everything else passes through byte for byte.
- `keep_keys`: an allowlist rule. A listed key keeps its subtree; outside one, plain values are dropped and containers survive only where they lead to a kept key.
- Dotted `drop_keys` entries are path-scoped: `user.url` drops `url` only directly under `user`.
- `drop_empty` removes `null`, `""`, `[]` and `{}` object fields. `tabular` rewrites arrays of 3 or more same-shaped objects as one column list plus value rows. Both are lossless.
- Key profiling in shadow mode and `isthmos stats -keys GLOB`: the heaviest key names per tool, names only, to write `drop_keys` and `keep_keys` from.
- `make bench` and a synthetic corpus in `testdata/corpus/`; a test fails if the starter rules save less than 30% on it.
- `doctor` also reads the project's `.claude/settings.json` and `.claude/settings.local.json`, fails on a misspelled rule field or malformed tool glob, and warns when no `SessionStart` compact hook is wired.
- Library: `Rules.KeepFor`, `KeyBytes`, `TopKeys`, `AggregateKeys`, `Seen.Reset`, and `Limits.DropEmpty` / `Limits.Tabular`.

### Fixed

- The hook applied shape-changing rules (`drop_keys`, `keep_keys`, `drop_empty`, `tabular`, a `max_items` marker in an array of objects) to built-in tools. Claude Code ignores a built-in rewrite that does not match the tool's output schema, so the rewrite was discarded while the saving was still logged. Built-in tools now get shape-preserving edits only, and `doctor` warns about rules that ask for more. Library: `Rules.KeepShape`, `Limits.KeepShape`.
- Integers beyond 2^53 were silently rounded by the JSON round trip (`1234567890123456789` became `1234567890123456800`). Numbers are now re-emitted exactly as written.
- `<`, `>` and `&` were re-encoded as 6-byte `\u00XX` escapes, which could cancel a rule's whole saving on code and HTML payloads.
- Cross-call dedup was keyed on `session_id` alone, which subagents share and compaction does not change, so a reference could point at content the agent never saw or no longer had. The index is now per subagent and is cleared by a `SessionStart` hook with matcher `compact`.
- `doctor` read shadow mode from its own environment, so a hook wired with `ISTHMOS_SHADOW=1` was reported as rewriting live. It now reads the hook command.
- Error pinning was unbounded, so a payload of nothing but error lines or items was never truncated. Pins are capped at `max_lines` / `max_items` extra entries.
- The dedup index was written through a fixed temp path that parallel hooks could clobber.

### Changed

- The hook returns without measuring on `SessionStart` events.
- README: `drop_keys` is documented as unlabelled and irreversible, where the design constraints previously implied every lossy step was labelled or reversible.

## [0.4.0] - 2026-07-26

### Added

- `isthmos stats -share` replaces third-party tool names with stable placeholders (`mcp__server1__*`), so a savings table can be pasted into a public issue. Built-in tool names are kept, since they carry nothing private.
- `doctor` warns when reveals are piling up while shadow mode is off, the signal that a rule cuts more than the agent can do without.
- Cross-call dedup: a payload the agent has already been sent in the same session is replaced by a reference to it instead of being resent. The index is scoped to the hook's `session_id`, so a hit means the content is genuinely still in context and the reference cannot dangle; it holds content hashes only, never payloads. Runs in shadow mode too, where it is measured but not applied, and `ISTHMOS_NO_DEDUP=1` disables it outright.
- Library: `ApplyWithSeen` takes a `*Seen` session index alongside the store. `Apply` and `ApplyWithStore` are unchanged and never dedup.

### Changed

- Shadow mode is now the documented way to install: the README wires the hook with `ISTHMOS_SHADOW=1` first and promotes live rewriting to a second step, once `isthmos stats` justifies it. Whether pruning pays is workload-dependent, so the front door no longer asks for trust ahead of evidence.
- `doctor` reports shadow mode as a normal state and points at `isthmos stats`, rather than listing it as a status flag.

## [0.3.0] - 2026-07-24

### Added

- `doctor` fails when the hook keeps firing on empty payloads (5+ recent calls, all 0 bytes), the signature of a wiring or input-field mismatch that silently disabled isthmos before v0.2.0.
- The measurement log is capped at 5MB; the oldest half is trimmed when it grows past that.
- `reveal` prints a clear "expired or unknown" message for a missing store entry instead of a raw file error, so an agent can recover by re-running the tool.
- `doctor` warns about dead rules: rule globs no isthmos PostToolUse matcher routes to.

### Fixed

- A trailing newline no longer consumes a `keep_last` slot or counts as a truncated line in `max_lines` markers.

## [0.2.0] - 2026-07-24

### Added

- `%ALL` column and scope note in `isthmos stats`: each tool's saving shown against all measured traffic, not just its own denominator.
- Reveal tracking: `isthmos reveal` logs a per-tool event and `stats` reports a `REVEALS` column, an over-pruning signal (each reveal is an extra tool call recovering truncated data).
- Text compression for text payloads: `max_lines` head-and-tail line truncation with error-line pinning and the same reversible markers, and `dedup` collapsing runs of 3+ identical lines into a labelled count. Applies to raw non-JSON payloads, JSON strings carrying text, and long strings embedded in JSON objects (`stdout` for Bash, `file.content` for Read).

### Fixed

- Hook adapter now reads the tool payload from `tool_response`, the field Claude Code actually sends on PostToolUse. It previously read a nonexistent `tool_output` field, so on real Claude Code traffic the hook always saw an empty payload and never rewrote anything.

### Changed

- Library: `Store.Save` now takes the tool name for reveal attribution; store entries gain a `.meta` sidecar holding only the tool name, the payload stays encrypted.

## [0.1.0] - 2026-07-21

### Added

- Rule-based JSON field pruning: glob-matched tool names, recursively dropped keys.
- `isthmos hook`, a Claude Code PostToolUse adapter that rewrites tool output via `updatedToolOutput`.
- `isthmos filter -tool NAME`, an agent-agnostic stdin/stdout mode.
- Go library API (`Apply`, `PruneJSON`, `LoadRules`) for embedding in other tools.
- Measurement log with before/after byte counts at `~/.local/state/isthmos/measure.jsonl`.
- Starter rules for Atlassian and GitHub MCP servers in `rules.example.json`.
- `isthmos stats`, a per-tool savings table over the measurement log (`-file`, `-since`), with a rough token estimate.
- `isthmos version`, wired to the goreleaser ldflags version.
- Generic size caps per rule: `max_items` for arrays and `max_str` for strings, always with an explicit truncation marker; the strictest matching limit wins.
- Smarter array truncation: `keep_last` keeps the newest items and error-looking items are never dropped.
- `min_bytes` per rule: payloads below the threshold pass through untouched.
- Reversibility store: truncated originals are AES-256-GCM encrypted under `~/.local/state/isthmos/store/` with a 7-day TTL, markers carry `isthmos reveal <id>`, and a new `reveal` subcommand recovers the full payload.
- Shadow mode (`ISTHMOS_SHADOW=1`): measure what the rules would save without rewriting anything, for safe rollout on a new machine.
- `isthmos doctor`: one-look health check of rules, store, measurement log, hook wiring, and shadow status.
- End-to-end smoke test (`make e2e`) exercising the built binary, wired into CI on Linux and macOS.
