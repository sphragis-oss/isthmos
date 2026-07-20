# Contributing to Isthmos

Thanks for your interest in contributing. This document covers how to set up,
make a change, and get it merged.

By participating you agree to the [Code of Conduct](CODE_OF_CONDUCT.md).

## Prerequisites

- Go 1.26 or newer.
- `git`.

## Local setup

```bash
git clone https://github.com/sphragis-oss/isthmos.git
cd isthmos
make build
make test
```

Common targets:

```bash
make test     # go test ./...
make vet      # go vet ./...
make fmt      # go fmt ./...
make build    # build the isthmos binary
make lint     # golangci-lint run
```

Try the filter end to end without installing anything:

```bash
echo '{"a":1,"node_id":"x"}' | ISTHMOS_RULES=rules.example.json ./isthmos filter -tool mcp__github__get_me
```

## Making a change

1. Open an issue first for anything non-trivial, so the approach can be agreed
   before you invest time.
2. Create a branch from `main`.
3. Keep changes focused. One logical change per pull request.
4. Add or update tests. New behaviour needs test coverage; bug fixes need a test
   that fails before the fix and passes after.
5. Run `make fmt`, `make vet`, and `make test` before pushing.

## Contributing pruning rules

Rules are the most valuable contribution: each one needs evidence, not a guess.
When proposing keys for `rules.example.json`, include in the PR description
which tool produced the payload and roughly how many bytes the keys account
for. `~/.local/state/isthmos/measure.jsonl` gives you before/after byte counts
for your own runs.

A key belongs in the drop list only if the model never needs it. When in doubt,
leave it in: a wrong rule silently removes information the agent may need.

## Coding style

- Follow [Effective Go](https://go.dev/doc/effective_go) and standard `gofmt`.
- Handle errors explicitly; use the standard library `log/slog` for logging.
- Preserve the fail-open contract: any error path must leave the tool output
  untouched rather than blocking or truncating it.
- Keep comments to a single line, explaining *why* rather than *what*. Prefer no
  comment over a redundant one.
- Every Go source file starts with the SPDX header:
  `// SPDX-License-Identifier: Apache-2.0`.

## Commit messages and DCO sign-off

This project requires the [Developer Certificate of Origin](https://developercertificate.org/)
(DCO) on every commit. Sign off your commits with:

```bash
git commit -s -m "your message"
```

This adds a `Signed-off-by` trailer certifying you have the right to submit the
work under the project's license. The DCO check in CI enforces this.

Write clear commit messages: a short imperative summary line, then a body
explaining the why if it is not obvious.

## Pull requests

- Fill out the pull request template.
- Make sure CI is green (lint, tests, DCO).
- A maintainer will review. Address feedback by pushing additional commits;
  avoid force-pushing during review so reviewers can see incremental changes.
- PRs are merged by a maintainer once approved and green.

## License

Isthmos is licensed under [Apache 2.0](LICENSE). By contributing, you agree
that your contributions are licensed under the same terms.

## Reporting security issues

Do not file security vulnerabilities as public issues. Follow
[SECURITY.md](SECURITY.md) instead.
