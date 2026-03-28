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
| CIDR-aware Allocation | Define pools as CIDR ranges (e.g. `10.31.103.0/24`) with auto-expansion |
| gRPC API | Programmatic access on port `:50051` |
| REST API | JSON endpoints on port `:8080` |
| HTMX Dashboard | Web UI for IP pool visualization and management |
| Dual Storage | Filesystem (YAML) or Kubernetes CRD backend |
| PowerDNS Integration | Optional DNS record management for IP assignments |
| DD-WRT Integration | Optional DNS via DD-WRT router (SSH + dnsmasq) |
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

## LOCAL DEVELOPMENT

<details><summary>RUN LOCALLY</summary>

### From disk config (easiest)

```bash
LOAD_CONFIG_FROM=disk CONFIG_LOCATION=tests CONFIG_NAME=config.yaml go run .
```

### From Kubernetes CR (requires cluster access)

```bash
LOAD_CONFIG_FROM=cr CONFIG_LOCATION=clusterbook CONFIG_NAME=networks-labul go run .
```

### Using Taskfile + .env

Create a `.env` file (see [example below](#example-env-file)), then:

```bash
task run
```

### Quick web UI start (no .env needed)

```bash
task run-web
```

This runs the web UI with disk config from `tests/config.yaml` on port `8080`. Override defaults with:

```bash
task run-web CONFIG_LOCATION=mydir CONFIG_NAME=myconfig.yaml HTTP_PORT=9090
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

### Create a network from CIDR

```bash
# Creates a /24 pool with 252 usable IPs (excludes .0, .1, .2, .255)
curl -X POST http://localhost:8080/api/v1/networks/cidr \
  -H "Content-Type: application/json" \
  -d '{"cidr": "10.31.105.0/24", "reserved": ["1", "2"]}'
```

The existing `POST /api/v1/networks` endpoint also accepts CIDR:

```bash
curl -X POST http://localhost:8080/api/v1/networks \
  -H "Content-Type: application/json" \
  -d '{"cidr": "10.31.105.0/28"}'
```

CIDR ranges spanning multiple /24 blocks (e.g. `/23`) automatically create multiple network entries.

### Assign an IP with DNS

```bash
curl -X POST http://localhost:8080/api/v1/networks/10.31.103/assign \
  -H "Content-Type: application/json" \
  -d '{"ip": "10.31.103.6", "cluster": "my-cluster", "status": "ASSIGNED", "create_dns": true}'
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

<details><summary>DAGGER MODULE</summary>

A [Dagger](https://dagger.io) module is available at [stuttgart-things/dagger/clusterbook](https://github.com/stuttgart-things/dagger) for pipeline integration.

```bash
# List all networks
dagger call list-networks --server="clusterbook.example.com:8080"

# Create network from CIDR
dagger call create-network-from-cidr --server="localhost:8080" --cidr="10.31.105.0/24" --reserved="1"

# Assign IP with DNS
dagger call assign-ip --server="localhost:8080" --network-key="10.31.103" \
  --ip="10.31.103.6" --cluster="my-cluster" --status="ASSIGNED" --create-dns

# Release IP
dagger call release-ip --server="localhost:8080" --network-key="10.31.103" --ip="10.31.103.6"

# List clusters
dagger call list-clusters --server="localhost:8080"
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
| `PDNS_ENABLED` | Enable PowerDNS integration | `false` |
| `PDNS_URL` | PowerDNS API URL | - |
| `PDNS_TOKEN` | PowerDNS API token | - |
| `PDNS_ZONE` | PowerDNS zone for records | - |
| `DDWRT_ENABLED` | Enable DD-WRT DNS integration | `false` |
| `DDWRT_HOST` | DD-WRT router IP/hostname | - |
| `DDWRT_USER` | SSH user for DD-WRT | - |
| `DDWRT_PASSWORD` | SSH password for DD-WRT | - |
| `DDWRT_ZONE` | DNS zone (e.g. `sthings.lab`) | - |

## DNS PROVIDERS

Both DNS providers are optional and can run simultaneously. Enable them via env vars. Records are created/deleted when `create_dns: true` is passed during assign/release.

<details><summary>POWERDNS</summary>

Creates wildcard A records (`*.cluster.zone`) via the PowerDNS REST API.

```bash
PDNS_ENABLED=true
PDNS_URL=http://pdns.sthings.lab:8081
PDNS_TOKEN=your-api-key
PDNS_ZONE=sthings.lab.
```

</details>

<details><summary>DD-WRT</summary>

Manages `dnsmasq` address entries on a DD-WRT router via SSH + `nvram`. Creates entries like `address=/cluster.zone/ip`.

```bash
DDWRT_ENABLED=true
DDWRT_HOST=192.168.1.1
DDWRT_USER=root
DDWRT_PASSWORD=your-router-password
DDWRT_ZONE=sthings.lab
```

**Credential handling in Kubernetes:**

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: clusterbook-ddwrt
type: Opaque
stringData:
  DDWRT_ENABLED: "true"
  DDWRT_HOST: "192.168.1.1"
  DDWRT_USER: "root"
  DDWRT_PASSWORD: "your-router-password"
  DDWRT_ZONE: "sthings.lab"
```

Reference in the deployment:

```yaml
envFrom:
  - secretRef:
      name: clusterbook-ddwrt
```

</details>

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

# DD-WRT DNS (optional)
#DDWRT_ENABLED=true
#DDWRT_HOST=192.168.1.1
#DDWRT_USER=root
#DDWRT_PASSWORD=secret
#DDWRT_ZONE=sthings.lab

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
