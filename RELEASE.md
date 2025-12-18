# Release v0.1.0

This repository includes automated release configuration.

Artifacts produced on release (tag `v*`):

- Source archives (tar.gz)
- Linux binaries for `amd64` and `arm64` (tar.gz)
- Docker images pushed to `ghcr.io/AnalyseDeCircuit/web-monitor` (tags: `vX.Y.Z`, `latest`) using `buildx` (linux/amd64, linux/arm64)

Pull example:

```bash
docker pull ghcr.io/AnalyseDeCircuit/web-monitor:v0.1.0
```

Notes for maintainers:
- Releases are triggered by pushing a tag like `v0.1.0` to the repository.
- The GitHub Actions workflow at `.github/workflows/release.yml` runs goreleaser to build archives and publish Docker images. It requires `GITHUB_TOKEN` (default) with `packages: write` permission.
- For local testing: `goreleaser release --snapshot --rm-dist`
