# Security Policy

## Supported Versions

| Version | Supported          |
| ------- | ------------------ |
| latest  | :white_check_mark: |
| < latest | :x:               |

Only the latest release of camspeak receives security fixes. Users are encouraged to
always run the most recent tagged release.

## Reporting a Vulnerability

**Do not open a public GitHub issue for security vulnerabilities.**

Please report vulnerabilities using [GitHub Security Advisories](https://github.com/jeeftor/camspeak/security/advisories/new)
(Private Vulnerability Reporting). This keeps the report private and visible only to
the repository maintainers.

When reporting, please include:

- A clear description of the vulnerability and its impact
- Steps to reproduce the issue
- Affected versions (if known)
- Any suggested mitigations or fixes

## Response Timeline

| Stage | Target |
| ----- | ------ |
| Acknowledgment of report | Within 72 hours |
| Initial assessment & triage | Within 7 days |
| Fix or mitigation | Best effort, severity-dependent |

## Supply-Chain Security

camspeak publishes the following supply-chain artifacts with each release:

- **SBOM** — A Software Bill of Materials (SPDX JSON format) is generated for every
  tagged release and attached to the GitHub release as an asset (`sbom.spdx.json`).
- **Build Provenance Attestations** — Each published Docker image on
  `ghcr.io/jeeftor/camspeak` is attested using GitHub Artifact Attestations
  (`actions/attest-build-provenance`). The attestation cryptographically ties the
  image to the workflow and commit that built it.
- **SBOM Attestations** — An SBOM attestation is also published for each image,
  linking the SPDX SBOM to the image digest.

You can verify attestations locally using the [GitHub CLI](https://cli.github.com/):

```bash
# Verify build provenance for a specific image
gh attestation verify ghcr.io/jeeftor/camspeak:vX.Y.Z \
  --repo jeeftor/camspeak
```

## Automated Scanning

The project runs automated security scans on every push, pull request, and weekly:

- **Trivy** — filesystem scan for HIGH and CRITICAL vulnerabilities (SARIF uploaded
  to GitHub Security tab)
- **govulncheck** — Go vulnerability database scan of the module and its dependencies
- **bun audit** — frontend dependency audit
