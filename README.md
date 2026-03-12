# stuttgart-things/clusterbook

gitops cluster configuration management

<div align="center">
  <p>
    <img src="https://github.com/stuttgart-things/docs/blob/main/hugo/sthings-argo.png" alt="sthings" width="450" />
  </p>
  <p>
    <strong>[/ˈklʌstəʳbʊk/]</strong>- gitops cluster configuration management

  </p>
</div>

## FEATURES

| Feature | Description |
|---------|-------------|
| IP Address Management | Allocate and track IPs across Kubernetes clusters |
| gRPC API | Programmatic access on port `:50051` |
| REST API | JSON endpoints on port `:8080` |
| HTMX Dashboard | Web UI for IP pool visualization and management |
| Dual Storage | Filesystem (YAML) or Kubernetes CRD backend |
| KCL Manifests | Type-safe Kubernetes deployment with KCL |

## DEPLOYMENT

<details><summary>KCL (RECOMMENDED)</summary>

### Render manifests

```bash
cd kcl && kcl run
```

### Render with custom config

```bash
kcl run -D config.image=ghcr.io/stuttgart-things/clusterbook:v1.6.0 \
        -D config.namespace=clusterbook \
        -D config.configName=networks-labul
```

### Render with HTTPRoute (Gateway API)

```bash
kcl run -D config.httpRouteEnabled=true \
        -D config.httpRouteParentRefName=my-gateway \
        -D config.httpRouteHostname=clusterbook.example.com
```

### Apply to cluster

```bash
cd kcl && kcl run | kubectl apply -f -
```

</details>

## USAGE

<details><summary>WEB UI (HTMX)</summary>

The HTMX dashboard is available on port `:8080` (configurable via `HTTP_PORT`).

- **Dashboard**: `http://localhost:8080/` - overview of all network pools
- **Network Detail**: `http://localhost:8080/network/10.31.103` - IP table with inline assign/release

</details>

<details><summary>REST API</summary>

### List all network pools

```bash
curl http://localhost:8080/api/v1/networks
```

### List IPs in a network

```bash
curl http://localhost:8080/api/v1/networks/10.31.103/ips
```

### Assign an IP

```bash
curl -X POST http://localhost:8080/api/v1/networks/10.31.103/assign \
  -H "Content-Type: application/json" \
  -d '{"ip": "10.31.103.6", "cluster": "my-cluster", "status": "ASSIGNED"}'
```

### Release an IP

```bash
curl -X POST http://localhost:8080/api/v1/networks/10.31.103/release \
  -H "Content-Type: application/json" \
  -d '{"ip": "10.31.103.6"}'
```

</details>

<details><summary>gRPC</summary>

```bash
# Get available IPs
grpcurl -plaintext localhost:50051 ipservice.IpService/GetIpAddressRange \
  -d '{"countIpAddresses": 2, "networkKey": "10.31.103"}'

# Assign IPs to a cluster
grpcurl -plaintext localhost:50051 ipservice.IpService/SetClusterInfo \
  -d '{"ipAddressRange": "10.31.103.6", "clusterName": "my-cluster", "status": "ASSIGNED"}'
```

</details>

<details><summary>CLI (machineshop)</summary>

### GET IPS

```bash
machineshop get \
--system=ips \
--destination=clusterbook.172.18.0.5.nip.io \
--path=10.31.103 \
--output=2
```

```bash
machineshop push \
--target=ips \
--destination=clusterbook.172.18.0.5.nip.io \
--artifacts="10.31.103.9;10.31.103.10" \
--assignee=app1
```

</details>

<details><summary>CREATE CR</summary>

```bash
kubectl apply -f - <<EOF
---
apiVersion: github.stuttgart-things.com/v1
kind: NetworkConfig
metadata:
  name: networks-labul
  namespace: clusterbook
spec:
  networks:
    10.31.101:
    - 6:ASSIGNED:rahul-andre-rke2
    - "7"
    - "9"
    - "10"
    - 5:ASSIGNED:rancher-mgmt
    10.31.102:
    - "5"
    - "6"
    - "7"
    - 8:ASSIGNED:unknown
    - "9"
    - "10"
    10.31.103:
    - 4:ASSIGNED:homerun-int2
    - 5:ASSIGNED:labul-automation
    - 6:ASSIGNED:labul-automation
    - "17"
    - "18"
    - 19:ASSIGNED:labul-automation
    - 8:ASSIGNED:fluxdev-3
    - 9:ASSIGNED:fluxdev-3
    - 16:ASSIGNED:homerun-dev
    10.31.104:
    - "5"
    - "6"
    - "7"
    - "8"
    - "9"
    - "10"
EOF
```

</details>

## CONFIGURATION

| Environment Variable | Description | Default |
|---------------------|-------------|---------|
| `LOAD_CONFIG_FROM` | Config source: `disk` or `cr` | - |
| `CONFIG_LOCATION` | File path or K8s namespace | - |
| `CONFIG_NAME` | File name or resource name | - |
| `SERVER_PORT` | gRPC server port | `50051` |
| `HTTP_PORT` | HTTP/HTMX server port | `8080` |
| `KUBECONFIG` | K8s config path (for CR backend) | - |

## DEV TASKS

```bash
task: Available tasks for this project:
* branch:         Create branch from main
* build:          Install
* build-ko:       Build image w/ KO
* commit:         Commit + push code into branch
* lint:           Lint Golang
* pr:             Create pull request into main
* proto:          Generate Go code from proto
* run:            Run
* test:           Test code
```

## AUTHOR

```bash
Patrick Hermann, stuttgart-things 09/2024
```

## EXAMPLE .env file

<details><summary>ENV FILE</summary>

.env file needed for Taskfile

```bash
cat <<EOF > .env
#LOAD_CONFIG_FROM=disk
#CONFIG_LOCATION=tests
#CONFIG_NAME=config.yaml
LOAD_CONFIG_FROM=cr
CONFIG_LOCATION=clusterbook #namespace
CONFIG_NAME=networks-labul #resource-name

SERVER_PORT=50051
HTTP_PORT=8080

#CLUSTERBOOK_SERVER=localhost:50051
#SECURE_CONNECTION=false
CLUSTERBOOK_SERVER=clusterbook.rke2.sthings-vsphere.labul.sva.de:443
SECURE_CONNECTION=true
EOF
```

</details>

## LICENSE

Licensed under the Apache License, Version 2.0 (the "License").

You may obtain a copy of the License at [apache.org/licenses/LICENSE-2.0](http://www.apache.org/licenses/LICENSE-2.0).

Unless required by applicable law or agreed to in writing, software distributed under the License is distributed on an _"AS IS"_ basis, without WARRANTIES or conditions of any kind, either express or implied.

See the License for the specific language governing permissions and limitations under the License.
