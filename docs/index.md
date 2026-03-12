# clusterbook

Go microservice for GitOps-based IP address management across Kubernetes clusters. Provides a gRPC API for programmatic access, a REST API for integrations, and an HTMX web dashboard for visualization.

## Quick Start

```bash
# Run with disk backend
LOAD_CONFIG_FROM=disk CONFIG_LOCATION=tests CONFIG_NAME=config.yaml go run .

# Run with Kubernetes CR backend
LOAD_CONFIG_FROM=cr CONFIG_LOCATION=clusterbook CONFIG_NAME=networks-labul go run .
```

## Architecture

```
                    ┌─────────────────────────┐
                    │       clusterbook        │
                    │                          │
  gRPC :50051 ─────┤  internal/generate.go    │──── Kubernetes CR
                    │  internal/load.go        │     (NetworkConfig)
  REST :8080  ─────┤  internal/save.go        │
                    │  internal/web.go         │──── Disk (YAML)
  HTMX :8080  ─────┤  internal/transform.go   │
                    └─────────────────────────┘
```

## Interfaces

| Interface | Port | Description |
|-----------|------|-------------|
| gRPC | `:50051` | `GetIpAddressRange`, `SetClusterInfo` RPCs |
| REST API | `:8080` | JSON endpoints for networks and IP management |
| HTMX Dashboard | `:8080` | Web UI with pool visualization and inline assign/release |

## Configuration

All configuration is done via environment variables. See the main [README](https://github.com/stuttgart-things/clusterbook) for the full reference.
