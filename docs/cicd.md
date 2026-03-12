# CI/CD

## GitHub Actions Workflows

| Workflow | Trigger | Description |
|----------|---------|-------------|
| `build-scan-image` | Push to main, PRs | Build container image with ko, push to ghcr.io, scan with Trivy |
| `release` | After image build | semantic-release: changelog, GitHub release, kustomize OCI push |
| `pages` | After release | Deploy TechDocs to GitHub Pages |

## Release Process

Releases are fully automated via [semantic-release](https://semantic-release.gitbook.io/):

- `fix:` commits trigger a **patch** bump
- `feat:` commits trigger a **minor** bump
- Each release publishes the container image and kustomize OCI artifact to `ghcr.io`

## Workflow Chain

```
push to main → build-scan-image → release → pages
                     │                │         │
               ko build + push   semantic    techdocs
               + trivy scan      release     deploy
                                 + stage
                                 image tag
                                 + kustomize
                                 OCI push
```

## Taskfile Commands

```bash
task build          # Build Go binary
task build-ko       # Build, push, scan container image
task test           # Run unit tests
task lint           # Run Go linter
task proto          # Generate Go code from proto
task run            # Run locally
```
