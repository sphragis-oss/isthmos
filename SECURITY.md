# Security Policy

## Threat model

Isthmos is a local filter. It reads a tool result on stdin, drops configured
JSON keys, and writes the smaller result back. It runs as your user, makes no
network calls, and holds no credentials.

Design properties that matter for security:

- **Never in the credential path.** Isthmos is not a proxy and does not sit
  between an agent and its LLM provider. It only sees tool output that has
  already been produced locally.
- **No network.** The binary makes no outbound connections. Rules are read from
  a local file, measurements are written to a local file.
- **Fail-open.** Any error (unreadable rules, malformed JSON, unparsable hook
  input) results in the original output passing through untouched. A broken
  isthmos degrades to a no-op, never to a blocked or corrupted tool result.
- **Shrink-only.** A replacement is emitted only when the pruned payload is
  strictly smaller than the original.

What Isthmos explicitly does not do:

- It is not a redaction or DLP tool. Dropping keys reduces tokens; it is not a
  security control and must not be relied on to strip secrets. For PII and
  secret redaction in the LLM path, use a gateway such as
  [Sphragis](https://github.com/sphragis-oss/sphragis).
- It does not sandbox, sign, or validate tool output.
- It does not protect against a malicious rules file. The rules file is trusted
  input: treat `~/.config/isthmos/rules.json` and `ISTHMOS_RULES` with the same
  care as any other local config that changes agent behaviour.

## Data handling

Tool output passes through process memory only. The measurement log at
`~/.local/state/isthmos/measure.jsonl` records byte counts and tool names, never
payload contents.

## Reporting a vulnerability

Please report security vulnerabilities privately. Do not open a public issue for
a suspected vulnerability.

- Preferred: open a private [GitHub Security Advisory](https://github.com/sphragis-oss/isthmos/security/advisories/new).
- Alternatively, email **nonicked@protonmail.com** with the details.

Please include:

- A description of the issue and its impact.
- Steps to reproduce, or a proof of concept.
- Affected version or commit.
- Any suggested remediation, if you have one.

You will receive an acknowledgement within 5 business days. We will keep you
informed as we investigate, agree a disclosure timeline, and ship a fix. We aim
to triage within 10 business days and to credit reporters who wish to be named.

## Verifying releases

Release archives are built by GitHub Actions and signed keyless with
[cosign](https://docs.sigstore.dev/); build provenance is attested with
GitHub artifact attestations.

```bash
# 1. verify the signed checksums file (cosign v2+)
cosign verify-blob \
  --bundle checksums.txt.bundle \
  --certificate-identity-regexp 'https://github.com/sphragis-oss/isthmos/\.github/workflows/release\.yml@.*' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com \
  checksums.txt

# 2. verify the archive against the checksums
shasum -a 256 --check --ignore-missing checksums.txt

# 3. verify build provenance (GitHub CLI)
gh attestation verify isthmos_darwin_arm64.tar.gz --repo sphragis-oss/isthmos
```

SBOMs (Syft) for every archive are attached to each release.

## Supported versions

Isthmos is pre-1.0. Until a stable release line exists, only the latest
released version receives security fixes.

| Version | Supported |
|---------|-----------|
| latest release | yes |
| older | no |

## Scope

The most security-relevant areas are:

- Hook input parsing and the fail-open guarantee (a crash or hang must not
  corrupt or block a tool result).
- Rules file loading and glob matching.
- Ensuring pruning never rewrites a payload into something that misleads the
  model about what a tool returned.

Reports in these areas are especially valued.
