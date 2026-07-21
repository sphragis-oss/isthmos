# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

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
