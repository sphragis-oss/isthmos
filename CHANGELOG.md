# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- `doctor` fails when the hook keeps firing on empty payloads (5+ recent calls, all 0 bytes), the signature of a wiring or input-field mismatch that silently disabled isthmos before v0.2.0.
- The measurement log is capped at 5MB; the oldest half is trimmed when it grows past that.

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
