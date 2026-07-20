# isthmos

isthmos (Greek: ισθμός), the narrow passage your tool outputs squeeze through.

A local context-compression layer for agent tool outputs. Runs as a Claude Code
PostToolUse hook: it reads the tool result, prunes and compresses it, and hands
the model a smaller payload via `updatedToolOutput`. Nothing leaves your machine
and nothing sits in the credential path.

## Status

Design phase. Single binary, Go, no runtime dependencies.

## Why

Agent context spend is dominated by tool outputs (verbose MCP JSON, log dumps,
API payloads), yet most token hygiene tooling only targets the input/prefix
side. isthmos targets the output side, at the only point where the payload is
visible before the model reads it.

## Planned phases

1. JSON field pruning for MCP tool outputs
2. Measurement: real before/after token accounting
3. Generic compressors per tool family
4. Reversibility store: recover full payloads on demand
5. Text compression

## Design constraints

- No proxy in the credential path
- One static binary, fast cold start (runs on every tool call)
- Lossy steps must be reversible or clearly labelled

## License

Apache-2.0
