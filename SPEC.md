# Spec: `cloud-sql-proxy-runner` Go CLI

## Context

We manually run `cloud-sql-proxy` instances for prod/staging databases, each on different ports, and look up passwords via `gcloud secrets`. This CLI automates that: start/stop/list proxy instances from a single YAML config, with idempotent starts and clear error messages when dependencies are missing.

**Key design choice**: Uses Go SDKs directly (`cloudsqlconn` for proxying, Secret Manager SDK for passwords) instead of shelling out to `cloud-sql-proxy` or `gcloud` CLIs. The proxy runs **in-process** as a daemon — one Go process with goroutines for each TCP listener.

---

## Commands

```
cloud-sql-proxy-runner start              # Start daemon with all proxies (idempotent)
cloud-sql-proxy-runner stop               # Stop the daemon
cloud-sql-proxy-runner list               # List proxies with status, ports
cloud-sql-proxy-runner list --show-passwords  # Include passwords in connection strings
```

Global flag: `--config <path>` (default: `~/.config/cloud-sql-proxy-runner/config.yaml`)

---

## YAML Config

```yaml
proxies:
  - instance: "org-123456:us-central1:org-clone"
    port: 5432
    secret: "app-db-user-password"

  - instance: "org-staging:us-central1:org"
    port: 5433
    secret: "app-db-user-password"
```

- **`instance`**: Cloud SQL connection string (`project:region:name`). Project is parsed from this.
- **`port`**: Local port to listen on.
- **`secret`**: Secret Manager secret name for the DB password.

### JSON Schema Validation

The YAML config is validated against an embedded JSON Schema before use. Errors report the exact path and constraint violated.

**Schema** (embedded in binary as `config_schema.json`):
```json
{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "type": "object",
  "required": ["proxies"],
  "additionalProperties": false,
  "properties": {
    "proxies": {
      "type": "array",
      "minItems": 1,
      "items": {
        "type": "object",
        "required": ["instance", "port", "secret"],
        "additionalProperties": false,
        "properties": {
          "instance": {
            "type": "string",
            "pattern": "^[a-z][a-z0-9-]+:[a-z][a-z0-9-]+:.+$",
            "description": "Cloud SQL connection string (project:region:name)"
          },
          "port": {
            "type": "integer",
            "minimum": 1024,
            "maximum": 65535
          },
          "secret": {
            "type": "string",
            "minLength": 1
          }
        }
      }
    }
  }
}
```

**Additional Go-level validation** (not expressible in JSON Schema):
- Instances are unique across entries
- Ports are unique across entries

**Error output example**:
```
Invalid config: proxies.1.port: Must be >= 1024
```

**Go package**: `github.com/santhosh-tekuri/jsonschema/v6` — lightweight, draft 2020-12 compliant.

---

## Architecture: In-Process Proxy

Instead of spawning `cloud-sql-proxy` child processes, the daemon **embeds the proxy** using `cloud.google.com/go/cloudsqlconn`:

```
For each configured proxy:
  1. net.Listen("tcp", "localhost:<port>")       // bind local port
  2. Accept loop (goroutine per connection):
     a. dialer.Dial(ctx, "<instance>")           // connect to Cloud SQL via SDK
     b. Bidirectional io.Copy between local conn and Cloud SQL conn
```

The `cloudsqlconn.Dialer` handles TLS, certificate rotation, and IAM auth automatically. No external binary needed.

---

## Behavior

### `start`
1. Run preflight: check ADC credentials exist via `google.FindDefaultCredentials()`
2. Load + validate config
3. Check for existing daemon: read PID file `~/.cloud-sql-proxy-runner/daemon.pid`
   - If PID exists and process is alive → print `Daemon already running (pid <N>)`, exit 0 (idempotent)
   - If stale PID file → clean it up, continue
4. Daemonize: re-exec itself with `--daemon` internal flag, detached from terminal
5. The daemon process:
   - Writes PID to `~/.cloud-sql-proxy-runner/daemon.pid`
   - Creates `cloudsqlconn.NewDialer()` (one shared dialer)
   - For each proxy in config: start a TCP listener goroutine on the configured port
   - Writes state file `~/.cloud-sql-proxy-runner/state.json` with all proxy details
   - Handles SIGTERM/SIGINT: close all listeners, close dialer, remove PID + state files, exit
   - Logs to `~/.cloud-sql-proxy-runner/daemon.log`
6. Parent process waits briefly, confirms ports are accepting connections
7. Prints:
   ```
   prod:    started on port 5432
   staging: started on port 5433
   ```

### `stop`
1. Read `~/.cloud-sql-proxy-runner/daemon.pid`
2. If no PID file or process dead → print `No daemon is running.`, exit 0
3. Send SIGTERM to PID, wait up to 5s for exit
4. If still alive → SIGKILL
5. Clean up PID/state files if daemon didn't
6. Print `Daemon stopped.`

### `list`
1. Load config
2. Read `~/.cloud-sql-proxy-runner/state.json` and check if daemon PID is alive
3. If `--show-passwords`, fetch secrets via Secret Manager SDK (in parallel via `errgroup`)
4. Print table:

```
INSTANCE                                          PORT   PROJECT             STATUS
org-123456:us-central1:org-clone      5432   org-123456    running
org-staging:us-central1:org            5433   org-staging   running
```

With `--show-passwords`, an extra column shows the password:
```
INSTANCE                                          PORT   PROJECT             STATUS    PASSWORD
org-123456:us-central1:org-clone      5432   org-123456    running   s3cretPa$$
org-staging:us-central1:org            5433   org-staging   running   0therPa$$
```

---

## Preflight Checks

Run before `start` and `list --show-passwords`. Uses SDK — no CLI binaries needed:

| Check | How | Error message |
|-------|-----|--------------|
| Application Default Credentials | `google.FindDefaultCredentials(ctx, scopes...)` | `No Google Cloud credentials found.\n\nRun: gcloud auth application-default login` |
| Secret access (on `list --show-passwords`) | SDK error from `AccessSecretVersion` | `Failed to access secret "<name>" in project "<project>".\n\nEnsure you have the Secret Manager Secret Accessor role.` |

Note: `gcloud` CLI is only needed by the user once to set up ADC. After that, the tool is self-contained.

---

## State Directory: `~/.cloud-sql-proxy-runner/`

```
~/.cloud-sql-proxy-runner/
├── daemon.pid          # Single PID for the daemon process
├── daemon.log          # Daemon stdout/stderr
└── state.json          # All proxy details for `list` to read
```

`state.json` format:
```json
{
  "pid": 12345,
  "started_at": "2026-02-25T10:00:00Z",
  "proxies": [
    {"instance": "org-123456:us-central1:org-clone", "port": 5432, "secret": "app-db-user-password"},
    {"instance": "org-staging:us-central1:org", "port": 5433, "secret": "app-db-user-password"}
  ]
}
```

---

## Project Structure

```
valencia/
├── main.go
├── go.mod
├── cloud-sql-proxy-runner.example.yaml
├── cmd/
│   ├── root.go         # Root cobra cmd, --config flag, config loading
│   ├── start.go        # Start command + daemonization
│   ├── stop.go         # Stop command
│   └── list.go         # List command with table output
└── internal/
    ├── config/
    │   ├── config.go        # Config struct, YAML parsing, schema + uniqueness validation
    │   └── schema.json      # Embedded JSON Schema (via go:embed)
    ├── proxy/
    │   ├── proxy.go    # TCP listener + cloudsqlconn dialer, per-proxy goroutine
    │   └── daemon.go   # Daemon lifecycle: start listeners, signal handling, state/pid files
    ├── secrets/
    │   └── secrets.go  # Secret Manager SDK: AccessSecretVersion
    └── preflight/
        └── checks.go   # ADC credential check via google.FindDefaultCredentials
```

**Dependencies**:
- `github.com/spf13/cobra` — CLI framework
- `gopkg.in/yaml.v3` — config parsing
- `cloud.google.com/go/cloudsqlconn` — Cloud SQL proxy dialer
- `cloud.google.com/go/secretmanager/apiv1` — Secret Manager access
- `golang.org/x/oauth2/google` — ADC credential detection
- `golang.org/x/sync/errgroup` — parallel secret fetching
- `github.com/santhosh-tekuri/jsonschema/v6` — JSON Schema validation

---

## Implementation Order

1. `go.mod`, `main.go`, `cmd/root.go` — skeleton + cobra setup
2. `internal/config/config.go` — config parsing/validation
3. `internal/preflight/checks.go` — ADC credential check
4. `internal/proxy/proxy.go` — TCP listener + cloudsqlconn dialer (core proxy logic)
5. `internal/proxy/daemon.go` — daemon lifecycle: PID/state files, signal handling, daemonization
6. `cmd/start.go` — wire up start + daemonize
7. `cmd/stop.go` — wire up stop
8. `internal/secrets/secrets.go` — Secret Manager SDK
9. `cmd/list.go` — wire up list + table formatting
10. `cloud-sql-proxy-runner.example.yaml`

---

## Testing

### Test file layout

```
internal/
├── config/
│   └── config_test.go       # Unit: schema validation, parsing, uniqueness checks
├── proxy/
│   ├── proxy_test.go        # Unit: TCP listener accept/proxy logic with mock dialer
│   └── daemon_test.go       # Unit: PID/state file lifecycle, signal handling
├── secrets/
│   └── secrets_test.go      # Unit: secret fetching with mock Secret Manager client
└── preflight/
    └── checks_test.go       # Unit: ADC detection with mock credentials
cmd/
├── start_test.go            # Integration: start daemon, verify ports, idempotency
├── stop_test.go             # Integration: start then stop, verify cleanup
└── list_test.go             # Integration: list output formatting, --show-passwords
```

### Unit tests

**`internal/config/config_test.go`**
- Valid config parses correctly
- Missing `proxies` key → schema error
- Empty proxies array → schema error (`minItems: 1`)
- Missing required field (instance, port, secret) → schema error naming the field
- Invalid instance format (e.g. `"bad"`, `"a:b"`) → schema pattern error
- Port out of range (0, 1023, 65536) → schema range error
- Empty secret string → schema error
- Duplicate ports → Go-level uniqueness error
- Duplicate instances → Go-level uniqueness error
- Extra unknown field in YAML → schema `additionalProperties` error
- Project is correctly parsed from instance string

**`internal/proxy/proxy_test.go`**
- Inject a mock dialer (interface) instead of real `cloudsqlconn.Dialer`
- Listener binds to a random free port, accepts a connection, proxies bytes bidirectionally through mock dialer
- Listener closes cleanly on context cancellation
- Connection errors from dialer are handled (logged, client conn closed)

**`internal/proxy/daemon_test.go`**
- Write + read PID file roundtrip
- Write + read state.json roundtrip
- `IsRunning()` returns true for own PID, false for non-existent PID
- Stale PID file cleanup (PID file exists but process dead)
- State directory creation when `~/.cloud-sql-proxy-runner/` doesn't exist

**`internal/secrets/secrets_test.go`**
- Inject a mock Secret Manager client (interface)
- Successful secret fetch returns trimmed value
- Missing secret → error with project + secret name in message
- Permission denied → error with role suggestion in message

**`internal/preflight/checks_test.go`**
- Inject a credential-finder function (not real ADC)
- Credentials found → no error
- Credentials missing → error message includes `gcloud auth application-default login`

### Integration tests

Run with `go test ./cmd/ -tags=integration` (build tag to skip in CI without credentials).

**`cmd/start_test.go`**
- Uses a temp config file and temp state directory (override via env or flag)
- Starts daemon with mock dialer (no real Cloud SQL needed)
- Verifies PID file written, state.json written, ports accepting TCP connections
- Second `start` is idempotent: prints "already running", no new process
- Start with port already in use by another process → clear error

**`cmd/stop_test.go`**
- Start daemon, then stop: PID file removed, state.json removed, ports freed
- Stop with no daemon running → "No daemon is running.", exit 0
- Stop with stale PID file → cleans up, exit 0

**`cmd/list_test.go`**
- List with daemon running → shows "running" status for each proxy
- List with daemon stopped → shows "stopped" status
- `--show-passwords` with mock secret client → passwords appear in output
- Output is valid tabwriter-formatted table

### Test interfaces for mocking

The proxy and secrets packages accept interfaces so tests avoid real GCP calls:

```go
// internal/proxy/proxy.go
type Dialer interface {
    Dial(ctx context.Context, instance string) (net.Conn, error)
    Close() error
}

// internal/secrets/secrets.go
type SecretClient interface {
    AccessSecretVersion(ctx context.Context, req *smpb.AccessSecretVersionRequest, opts ...gax.CallOption) (*smpb.AccessSecretVersionResponse, error)
}

// internal/preflight/checks.go
type CredentialFinder func(ctx context.Context, scopes ...string) (*google.Credentials, error)
```

Production code passes the real SDK implementations; tests pass mocks.

---

## Verification

1. `go build -o cloud-sql-proxy-runner .` — compiles
2. Create config at `~/.config/cloud-sql-proxy-runner/config.yaml` with prod+staging entries
3. `./cloud-sql-proxy-runner start` — daemon starts, ports accessible
4. `./cloud-sql-proxy-runner start` — idempotent, prints "already running"
5. `./cloud-sql-proxy-runner list` — shows both as running with project + database columns
6. `./cloud-sql-proxy-runner list --show-passwords` — shows passwords in connection strings
7. `./cloud-sql-proxy-runner stop` — daemon stops, ports freed
8. `./cloud-sql-proxy-runner list` — shows both as stopped
9. Revoke ADC (`rm ~/.config/gcloud/application_default_credentials.json`), run `./cloud-sql-proxy-runner start` — clear error with instructions
