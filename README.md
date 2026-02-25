# cloud-proxy

Manage Cloud SQL proxy connections from a single YAML config. Start, stop, and list proxies with one command — no need to run multiple `cloud-sql-proxy` instances manually.

Uses Go SDKs directly (no `cloud-sql-proxy` or `gcloud` CLI binaries required at runtime).

## Install

```sh
# Nix (recommended)
nix profile install github:steel-gareth/cloud-proxy

# Or run without installing
nix run github:steel-gareth/cloud-proxy -- --help

# Or build from source
go install .
```

## Setup

1. Authenticate with Google Cloud (one-time):
   ```sh
   gcloud auth application-default login
   ```

2. Create a config file at `~/.config/cloud-proxy/config.yaml`:
   ```yaml
   proxies:
     - instance: "my-project:us-central1:my-database"
       port: 5432
       secret: "db-password"

     - instance: "my-project:us-central1:other-database"
       port: 5433
       secret: "other-db-password"
   ```

   - **instance**: Cloud SQL connection string (`project:region:name`)
   - **port**: Local port to listen on (1024–65535)
   - **secret**: Secret Manager secret name for the DB password

## Usage

```sh
cloud-proxy start                  # Start daemon with all proxies (idempotent)
cloud-proxy stop                   # Stop the daemon
cloud-proxy list                   # List proxies with status and ports
cloud-proxy list --show-passwords  # Include passwords from Secret Manager
```

Use `--config <path>` to specify a different config file.

### `start`

Runs preflight checks (ADC credentials), validates config, and starts a background daemon. Each proxy gets a TCP listener on localhost. Running `start` again when the daemon is already running is a no-op.

### `stop`

Sends SIGTERM to the daemon, waits up to 5s, then SIGKILL if needed. Cleans up PID and state files.

### `list`

Shows a table of configured proxies with their status:

```
INSTANCE                                       PORT   PROJECT            STATUS
my-project:us-central1:my-database             5432   my-project         running
my-project:us-central1:other-database          5433   my-project         running
```

With `--show-passwords`, fetches secrets from Secret Manager in parallel and adds a PASSWORD column.

## State directory

Runtime files are stored in `~/.cloud-proxy/`:

```
~/.cloud-proxy/
├── daemon.pid    # Daemon process ID
├── daemon.log    # Daemon stdout/stderr
└── state.json    # Proxy details for `list`
```
