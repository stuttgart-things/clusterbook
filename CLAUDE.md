# CLAUDE.md — clusterbook

> GitOps cluster configuration management service.
> Go gRPC + REST + HTMX. Module: `github.com/stuttgart-things/clusterbook`

---

## PROJECT STRUCTURE

```
clusterbook/
├── main.go               # Entry point: env wiring, gRPC server, StartWebServer
├── internal/             # Business logic (web handlers, DNS providers, IP mgmt)
│   ├── pdns.go           # PowerDNS provider (REST API)
│   ├── ddwrt.go          # DD-WRT provider (SSH + nvram)
│   ├── fakeddwrt.go      # In-process fake SSH server for tests
│   ├── web.go            # HTTP/HTMX handlers + assign endpoint
│   └── ...
├── ipservice/            # gRPC protobuf generated code
│   └── *.proto / *.go
├── kcl/                  # KCL manifests for Kubernetes deployment
├── provider/             # Crossplane provider (separate Go module)
├── tests/                # Local test configs + gRPC client
│   └── config.yaml       # Disk-based NetworkConfig for local dev
├── Taskfile.yaml         # All dev tasks (use `task <name>`)
├── .env                  # Local env vars (gitignored, see example below)
└── go.mod                # Module: github.com/stuttgart-things/clusterbook
```

---

## ENVIRONMENT VARIABLES

All config is via env vars. The `.env` file is loaded automatically by Taskfile.

### Core

| Variable          | Description                        | Default  |
|-------------------|------------------------------------|----------|
| `LOAD_CONFIG_FROM`| `disk` or `cr` (Kubernetes CRD)   | required |
| `CONFIG_LOCATION` | File dir or K8s namespace          | required |
| `CONFIG_NAME`     | File name or CRD resource name     | required |
| `SERVER_PORT`     | gRPC port                          | `50051`  |
| `HTTP_PORT`       | REST/HTMX port                     | `8080`   |

### PowerDNS provider

| Variable       | Description              |
|----------------|--------------------------|
| `PDNS_ENABLED` | `true` to enable         |
| `PDNS_URL`     | PowerDNS API URL         |
| `PDNS_TOKEN`   | PowerDNS API token       |
| `PDNS_ZONE`    | Zone (e.g. `sthings.lab.`) |

### DD-WRT provider

| Variable         | Description                        |
|------------------|------------------------------------|
| `DDWRT_ENABLED`  | `true` to enable                   |
| `DDWRT_HOST`     | Router IP/hostname (`:22` optional)|
| `DDWRT_USER`     | SSH user (`root`)                  |
| `DDWRT_PASSWORD` | SSH password                       |
| `DDWRT_ZONE`     | DNS zone (e.g. `sthings.lab`)      |

### Example `.env` for local dev with DD-WRT

```bash
LOAD_CONFIG_FROM=disk
CONFIG_LOCATION=tests
CONFIG_NAME=config.yaml
SERVER_PORT=50051
HTTP_PORT=8080

# DD-WRT (optional)
DDWRT_ENABLED=true
DDWRT_HOST=192.168.1.1
DDWRT_USER=root
DDWRT_PASSWORD=secret
DDWRT_ZONE=sthings.lab
```

---

## TASK COMMANDS

Always use `task` — never run go commands manually unless debugging.

```bash
task run          # build + run (reads .env)
task run-web      # run web UI only, disk config, port 8080
task build        # lint + test + go install
task test         # go mod tidy + go test ./... -v
task lint         # golangci-lint run
task proto        # regenerate gRPC Go code from .proto files
task check        # pre-commit run -a
task commit       # interactive commit + push (uses gum)
task branch       # create branch from main
task pr           # lint + create PR + watch CI + auto-merge
task build-ko     # build container image with KO + trivy scan
task release      # full release pipeline
```

Helpful overrides:

```bash
task run-web CONFIG_LOCATION=mydir CONFIG_NAME=myconfig.yaml HTTP_PORT=9090
task test                          # runs all tests incl. ddwrt fake SSH tests
```

---

## DNS PROVIDER PATTERN

Both providers follow the same interface contract wired in `main.go`:

```go
// main.go
pdns  := internal.NewPDNSClient(pdnsEnabled, pdnsURL, pdnsToken, pdnsZone)
ddwrt := internal.NewDDWRTClient(ddwrtEnabled, ddwrtHost, ddwrtUser, ddwrtPassword, ddwrtZone)
go internal.StartWebServer(httpPort, loadConfigFrom, configLocation, configName, pdns, ddwrt)
```

Both return `nil` when disabled — handlers must nil-check before calling.

```go
// internal/web.go — assign handler pattern
if pdns != nil {
    pdns.CreateRecord(hostname, ip)
}
if ddwrt != nil {
    ddwrt.CreateRecord(hostname, ip)
}
```

Triggered by REST assign with `"create_dns": true`:

```bash
curl -X POST http://localhost:8080/api/v1/networks/10.31.103/assign \
  -H "Content-Type: application/json" \
  -d '{"ip":"10.31.103.6","cluster":"myapp","status":"ASSIGNED","create_dns":true}'
```

---

## DD-WRT PROVIDER SPECIFICS

### How it works

SSHes into DD-WRT router and manages `dnsmasq_options` via `nvram`:

```bash
# Read current entries
nvram get dnsmasq_options

# Write new entry (preserves others, deduplicates by FQDN)
nvram set dnsmasq_options='address=/myapp.sthings.lab/10.31.103.6' && nvram commit && restart_dnsmasq
```

### Key files

| File                      | Purpose                                      |
|---------------------------|----------------------------------------------|
| `internal/ddwrt.go`       | Provider + `SSHExecutor` interface + helpers |
| `internal/fakeddwrt.go`   | In-process fake SSH server for tests         |
| `internal/ddwrt_test.go`  | Unit + mock + fake-SSH integration tests     |

### SSHExecutor interface (enables test injection)

```go
type SSHExecutor interface {
    Run(cmd string) (string, error)
    Close() error
}
```

Production creates `realSSHExecutor` per call.
Tests inject `fakeExecutor` (in-memory) or connect to `FakeDDWRTServer` (real SSH).

### Adding a new provider (follow this pattern)

1. Create `internal/<provider>.go` with `New<Provider>Client(enabled, ...args) *<Provider>Client`
2. Implement `CreateRecord(hostname, ip string) error` and `DeleteRecord(hostname string) error`
3. Add env vars to `main.go` var block
4. Init client in `main()` and pass to `StartWebServer`
5. Nil-check and call in assign/release handlers in `web.go`
6. Add tests in `internal/<provider>_test.go`

---

## TESTING

### Run all tests

```bash
task test
# or directly:
go test ./... -v
```

### DD-WRT test layers

```bash
# Unit tests (no network, pure functions)
go test ./internal/... -v -run TestMergeDNSEntry
go test ./internal/... -v -run TestRemoveDNSEntry

# Mock executor tests (in-memory, no SSH)
go test ./internal/... -v -run TestDDWRTClient.*Mock

# Fake SSH server integration tests (real SSH stack, in-process)
go test ./internal/... -v -run TestDDWRTClient.*FakeSSH
go test ./internal/... -v -run TestFakeDDWRTServer
```

### FakeDDWRTServer — usage in tests

```go
srv, _ := internal.NewFakeDDWRTServer("root", "testpass")
defer srv.Close()

// Pre-seed nvram state
srv.NvramSet("dnsmasq_options", "address=/existing.sthings.lab/10.31.103.5")

// Assert nvram state after operations
opts := srv.NvramGet("dnsmasq_options")
```

### Crossplane provider tests

```bash
task provider-test
# or:
cd provider && go test ./... -v
```

---

## LOCAL DEV WORKFLOW

```bash
# 1. Run with disk config (fastest)
task run-web

# 2. Assign an IP (without DNS)
curl -X POST http://localhost:8080/api/v1/networks/10.31.103/assign \
  -H "Content-Type: application/json" \
  -d '{"ip":"10.31.103.6","cluster":"myapp","status":"ASSIGNED"}'

# 3. Assign an IP with DD-WRT DNS (needs DDWRT_* env vars set)
curl -X POST http://localhost:8080/api/v1/networks/10.31.103/assign \
  -H "Content-Type: application/json" \
  -d '{"ip":"10.31.103.6","cluster":"myapp","status":"ASSIGNED","create_dns":true}'

# 4. List networks
curl http://localhost:8080/api/v1/networks

# 5. gRPC (requires grpcurl)
grpcurl -plaintext localhost:50051 ipservice.IpService/GetIpAddressRange \
  -d '{"countIpAddresses":2,"networkKey":"10.31.103"}'
```

---

## CODE CONVENTIONS

- Logger: always use `pterm.DefaultLogger.WithLevel(pterm.LogLevelTrace)` — never `fmt.Println` or `log.Printf` in providers
- Errors: wrap with context using `fmt.Errorf("ddwrt <operation>: %w", err)`
- Env vars: `SCREAMING_SNAKE_CASE`, read only in `main.go` var block, passed as constructor args
- Constructor returns `nil` when disabled — callers always nil-check
- Pure helper functions (no I/O) in same file as provider, named without receiver — keeps them unit-testable
- Test file naming: `<provider>_test.go` in same package (`package internal`)
- No `init()` functions
- `go mod tidy` before every commit (`task build` does this)

---

## MODULE & DEPENDENCIES

```
module github.com/stuttgart-things/clusterbook

key dependencies:
  github.com/pterm/pterm          # logging + banners
  google.golang.org/grpc          # gRPC server
  golang.org/x/crypto/ssh         # SSH client (DD-WRT provider)
  github.com/pterm/pterm          # structured logging
```

Add new dependency:

```bash
go get <module>@latest
go mod tidy
```

---

## GIT WORKFLOW

```bash
task branch          # create feature branch
# ... make changes, task test ...
task commit          # interactive: picks message via gum, pushes
task pr              # creates PR, watches CI, auto-merges on green
```

Branch naming convention: `feat/ddwrt-provider`, `fix/nvram-dedup`

Commit message convention (semantic-release):
- `feat: add DD-WRT DNS provider`
- `fix: deduplicate nvram entries on update`
- `test: add fake SSH server for DD-WRT integration tests`

---

## CONTAINER BUILD

```bash
# Build + push image with KO (requires GITHUB_TOKEN)
task build-ko

# KO repo target
ghcr.io/stuttgart-things/clusterbook
```

Image is also scanned with Trivy after build.

---

## KNOWN PATTERNS TO FOLLOW

- **Nil-safe providers**: `if pdns != nil { ... }` — never assume enabled
- **Compound SSH commands**: `cmd1 && cmd2 && cmd3` — DD-WRT needs all three: `set`, `commit`, `restart_dnsmasq`
- **FQDN deduplication**: always call `mergeDNSEntry` before writing — idempotent by FQDN
- **Test hierarchy**: pure helpers -> mock executor -> fake SSH server — add tests at all three levels for new SSH behaviour
- **Env naming**: mirror pdns pattern exactly: `DDWRT_ENABLED`, `DDWRT_HOST`, `DDWRT_PASSWORD`, `DDWRT_ZONE`
