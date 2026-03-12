# clusterbook / KCL Deployment

KCL-based Kubernetes manifests for clusterbook. Renders Namespace, ServiceAccount, ConfigMap, Role/RoleBinding, Deployment (dual-port: gRPC + HTTP), Services, and optional HTTPRoute (Gateway API).

## Render Manifests

### Via Dagger (recommended)

```bash
# render with a profile file
dagger call -m github.com/stuttgart-things/dagger/kcl@v0.82.0 run \
  --source kcl \
  --parameters-file tests/kcl-deploy-profile.yaml \
  export --path /tmp/rendered-clusterbook.yaml

# render with inline parameters
dagger call -m github.com/stuttgart-things/dagger/kcl@v0.82.0 run \
  --source kcl \
  --parameters 'config.image=ghcr.io/stuttgart-things/clusterbook/clusterbook:v1.5.1,config.namespace=clusterbook' \
  export --path /tmp/rendered-clusterbook.yaml
```

### Via kcl CLI

```bash
kcl run kcl/main.k \
  -D 'config.image=ghcr.io/stuttgart-things/clusterbook/clusterbook:v1.5.1' \
  -D 'config.namespace=clusterbook'
```

## Deploy to Cluster

```bash
# render + apply
cd kcl && kcl run | kubectl apply -f -

# or with custom config
kcl run -D 'config.image=ghcr.io/stuttgart-things/clusterbook/clusterbook:v1.5.1' \
        -D 'config.configName=networks-labul' \
  | kubectl apply -f -
```

## Deploy with HTTPRoute (Gateway API)

```bash
kcl run -D 'config.httpRouteEnabled=true' \
        -D 'config.httpRouteParentRefName=my-gateway' \
        -D 'config.httpRouteHostname=clusterbook.example.com' \
  | kubectl apply -f -
```

## Profile Parameters

| Parameter | Default | Description |
|---|---|---|
| `config.name` | `clusterbook` | Resource name |
| `config.namespace` | `clusterbook` | Target namespace |
| `config.image` | `ghcr.io/stuttgart-things/clusterbook/clusterbook:latest` | Container image |
| `config.imagePullPolicy` | `Always` | Image pull policy |
| `config.replicas` | `1` | Replica count |
| `config.grpcPort` | `50051` | gRPC container port |
| `config.httpPort` | `8080` | HTTP/HTMX container port |
| `config.grpcServicePort` | `80` | gRPC service port |
| `config.httpServicePort` | `8080` | HTTP service port |
| `config.serviceType` | `ClusterIP` | Service type |
| `config.loadConfigFrom` | `cr` | Config source: `disk` or `cr` |
| `config.configLocation` | `clusterbook` | K8s namespace or file path |
| `config.configName` | `networks-labul` | Resource name or file name |
| `config.serverPort` | `50051` | gRPC server port env var |
| `config.networkConfigApiGroup` | `github.stuttgart-things.com` | CRD API group for RBAC |
| `config.httpRouteEnabled` | `false` | Enable HTTPRoute (Gateway API) |
| `config.httpRouteParentRefName` | *(empty)* | Gateway name for HTTPRoute |
| `config.httpRouteParentRefNamespace` | *(empty)* | Gateway namespace |
| `config.httpRouteHostname` | *(empty)* | Hostname for HTTPRoute |
| `config.httpRouteAnnotations` | `{}` | Extra annotations for HTTPRoute |
| `config.cpuRequest` | `50m` | CPU request |
| `config.cpuLimit` | `100m` | CPU limit |
| `config.memoryRequest` | `64Mi` | Memory request |
| `config.memoryLimit` | `128Mi` | Memory limit |
| `config.labels` | `{}` | Additional labels |
| `config.annotations` | `{}` | Additional annotations |

## Example Profiles

### Default (CR backend)

```yaml
---
config.image: ghcr.io/stuttgart-things/clusterbook/clusterbook:v1.5.1
config.namespace: clusterbook
config.configName: networks-labul
```

### Disk backend (development)

```yaml
---
config.image: ghcr.io/stuttgart-things/clusterbook/clusterbook:v1.5.1
config.namespace: clusterbook-dev
config.loadConfigFrom: disk
config.configLocation: /config
config.configName: config.yaml
```

### With HTTPRoute (production)

```yaml
---
config.image: ghcr.io/stuttgart-things/clusterbook/clusterbook:v1.5.1
config.namespace: clusterbook
config.configName: networks-labul
config.httpRouteEnabled: true
config.httpRouteParentRefName: sthings-gateway
config.httpRouteParentRefNamespace: ingress-system
config.httpRouteHostname: clusterbook.sthings-vsphere.labul.sva.de
```

## Rendered Resources

| Resource | Name | Conditional |
|----------|------|-------------|
| Namespace | `{namespace}` | Always |
| ServiceAccount | `{name}` | Always |
| ConfigMap | `{name}-config` | Always |
| Role | `{name}` | Always |
| RoleBinding | `{name}` | Always |
| Deployment | `{name}` | Always |
| Service (gRPC) | `{name}` | Always |
| Service (HTTP) | `{name}-http` | Always |
| HTTPRoute | `{name}` | Only if `httpRouteEnabled=true` |
