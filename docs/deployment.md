# Deployment

## Container Image

Built with [ko](https://ko.build/) using a distroless base image (`cgr.dev/chainguard/static:latest`):

```bash
# Build locally
ko build .

# Build via Taskfile
task build-ko
```

## KCL Deployment (recommended)

Render and apply KCL manifests directly:

```bash
# Default deployment
cd kcl && kcl run | kubectl apply -f -

# Custom image version
kcl run -D config.image=ghcr.io/stuttgart-things/clusterbook/clusterbook:v1.5.1 \
  | kubectl apply -f -

# With HTTPRoute (Gateway API)
kcl run -D config.httpRouteEnabled=true \
        -D config.httpRouteParentRefName=my-gateway \
        -D config.httpRouteHostname=clusterbook.example.com \
  | kubectl apply -f -
```

See the [KCL README](https://github.com/stuttgart-things/clusterbook/tree/main/kcl) for all profile parameters.

## Rendered Resources

| Resource | Name | Description |
|----------|------|-------------|
| Namespace | `clusterbook` | Dedicated namespace |
| ServiceAccount | `clusterbook` | Pod identity |
| ConfigMap | `clusterbook-config` | Environment configuration |
| Role | `clusterbook` | NetworkConfig CR access |
| RoleBinding | `clusterbook` | Binds SA to Role |
| Deployment | `clusterbook` | Dual-port: gRPC + HTTP |
| Service (gRPC) | `clusterbook` | ClusterIP on port 80 |
| Service (HTTP) | `clusterbook-http` | ClusterIP on port 8080 |
| HTTPRoute | `clusterbook` | Gateway API route (optional) |

## Testing

```bash
# Unit tests
go test ./...

# Lint
task lint

# Build + scan image
task build-ko
```
