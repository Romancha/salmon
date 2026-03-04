# bear-sync

Syncs Bear notes with external consumers. Two components: **hub** (API server on VPS) and **bridge** (Mac agent that reads Bear SQLite).

## Architecture

### Components

**Bear** — source of truth for all note content. Stores notes in a local SQLite database (Core Data schema). The bridge reads this database directly and applies writes via Bear's x-callback-url scheme.

**Bridge** (`bin/bear-bridge`) — Mac agent that runs on the same machine as Bear. Runs once per invocation (scheduled via launchd). Reads Bear's SQLite, detects changes since the last run, pushes them to the hub, and pulls pending write operations from the hub to apply back to Bear via bear-xcall.

**Hub** (`bin/bear-sync-hub`) — API server that runs on a VPS. Acts as a read replica of Bear's notes and exposes a REST API for external consumers. Holds a write queue for consumer-initiated changes that need to propagate back to Bear.

**Consumers** — external applications that read and write notes via the hub API. Each consumer is identified by name and authenticated with its own token. Multiple consumers can be configured simultaneously. Consumers communicate only with the hub; never touch Bear or the bridge directly.

### System Overview

```mermaid
graph TB
    subgraph mac["Mac (user's machine)"]
        Bear["Bear.app\n(SQLite source of truth)"]
        Bridge["bear-bridge\n(launchd, every 5 min)"]
        bearxcall["bear-xcall CLI\n(x-callback-url executor)"]
    end

    subgraph vps["VPS"]
        Caddy["Caddy\n(TLS reverse proxy)"]
        Hub["bear-sync-hub\n(REST API + SQLite)"]
    end

    Consumer["Consumer\n(API client)"]

    Bridge -- "reads Bear SQLite\n(read-only)" --> Bear
    Bridge -- "applies writes via\nbear:// URL scheme" --> bearxcall
    bearxcall -- "x-callback-url" --> Bear
    Bridge -- "POST /api/sync/push\n(bridge token)" --> Caddy
    Bridge -- "GET /api/sync/queue\nPOST /api/sync/ack" --> Caddy
    Caddy --> Hub
    Consumer -- "GET/POST/PUT/DELETE /api/notes/, /api/tags/\n(consumer token)" --> Caddy
```

### Note `sync_status` State Machine

The `sync_status` field on each hub note guards against write conflicts between consumers and Bear.

```mermaid
stateDiagram-v2
    synced: synced\n(normal state)
    pending: pending_to_bear\n(consumer write queued)
    conflict: conflict\n(Bear changed while write pending)

    [*] --> synced: Bear push (initial/delta)

    synced --> pending: consumer enqueues write\n(POST/PUT/DELETE)
    pending --> synced: bridge ACKs applied
    pending --> conflict: Bear push arrives\nwith newer modified_at

    conflict --> synced: bridge creates [Conflict] note\nand ACKs conflict_resolved=true
```

While a note is `pending_to_bear`, Bear delta pushes do not overwrite `title`/`body` on the hub. If Bear modifies the note before the bridge ACKs, the hub detects a conflict and the bridge creates a `[Conflict] Title` note in Bear instead of overwriting.

### Write Actions

Consumers can enqueue write operations via the hub API. The bridge picks them up and applies them to Bear via x-callback-url.

| Action | Consumer API | Description |
|---|---|---|
| `create` | `POST /api/notes` | Create a new note |
| `update` | `PUT /api/notes/{id}` | Update note title/body |
| `add_tag` | `POST /api/notes/{id}/tags` | Add a tag to a note |
| `trash` | `DELETE /api/notes/{id}` | Move note to trash |
| `add_file` | `POST /api/notes/{id}/attachments` | Attach a file to a note (multipart, 5 MB limit) |
| `archive` | `POST /api/notes/{id}/archive` | Archive a note |
| `rename_tag` | `PUT /api/tags/{id}` | Rename a tag |
| `delete_tag` | `DELETE /api/tags/{id}` | Delete a tag |

All mutating consumer endpoints require an `Idempotency-Key` header. Encrypted notes are read-only (403).

## Prerequisites

- Go 1.24+
- Xcode Command Line Tools (for building bear-xcall on macOS; provides `swiftc`)
- Bear.app (for bridge)
- bear-xcall CLI (built via `make build-xcall`, for bridge write operations; source in `tools/bear-xcall/`)

## Build

```
make build
```

Binaries are placed in `bin/bear-sync-hub`, `bin/bear-bridge`, and `bin/bear-xcall.app` (macOS only).

## Hub Setup

### Environment Variables

| Variable | Required | Default | Description |
|---|---|---|---|
| `HUB_HOST` | No | `127.0.0.1` | Listen host (`0.0.0.0` for Docker) |
| `HUB_PORT` | No | `7433` | Listen port |
| `HUB_DB_PATH` | Yes | — | Path to SQLite database file |
| `HUB_CONSUMER_TOKENS` | Yes | — | Consumer tokens in `name:token` format, comma-separated (e.g. `openclaw:secret1,myapp:secret2`) |
| `HUB_BRIDGE_TOKEN` | Yes | — | Bearer token for bridge sync access |
| `HUB_ATTACHMENTS_DIR` | No | `attachments` | Directory for attachment file storage |

### Running

```
export HUB_DB_PATH=/opt/bear-sync/data/hub.db
export HUB_CONSUMER_TOKENS="openclaw:secret1,myapp:secret2"
export HUB_BRIDGE_TOKEN=<token>
./bin/bear-sync-hub
```

The hub listens on `127.0.0.1:PORT` (localhost only). Use a reverse proxy (e.g. Caddy) for TLS termination.

### Systemd (production)

```
sudo cp deploy/bear-sync-hub.service /etc/systemd/system/
sudo systemctl enable bear-sync-hub
sudo systemctl start bear-sync-hub
```

Create `/opt/bear-sync/.env` with the environment variables above.

### Docker Compose (production)

1. Copy `.env.example` to `.env` and fill in secrets:

```
cp .env.example .env
```

2. Set your domain in `.env`:

```
HUB_CONSUMER_TOKENS="openclaw:secret1,myapp:secret2"
HUB_BRIDGE_TOKEN=<token>
DOMAIN=bear-sync.example.com
```

3. Start the stack:

```
docker compose up -d
```

This starts the hub server and Caddy reverse proxy with automatic TLS. The hub is accessible only through Caddy (ports 80/443).

To check status:

```
docker compose ps
curl https://your-domain.com/healthz
```

To update to a new version:

```
docker compose pull
docker compose up -d
```

Data is persisted in Docker named volumes (`hub-data` for SQLite + attachments).

#### Volume Permissions (Synology / bind mounts)

The hub container runs as non-root user `hub` (UID 1000). When using bind mounts, ensure the host directory is owned by UID 1000, otherwise SQLite will fail with `unable to open database file: out of memory (14)`:

```
mkdir -p /volume1/docker/bear_hub/attachments
chown -R 1000:1000 /volume1/docker/bear_hub
```

This is not needed for Docker named volumes — they inherit permissions from the image automatically.

## Bridge Setup

### Environment Variables

| Variable | Required | Default | Description |
|---|---|---|---|
| `BRIDGE_HUB_URL` | Yes | — | Hub API URL (e.g. `https://bear-sync.example.com`) |
| `BRIDGE_HUB_TOKEN` | Yes | — | Bearer token matching `HUB_BRIDGE_TOKEN` |
| `BEAR_TOKEN` | Yes | — | Token for Bear x-callback-url API (any string, e.g. `openssl rand -base64 32`; Bear will prompt to allow access on first use) |
| `BRIDGE_STATE_PATH` | No | `~/.bear-bridge-state.json` | Path to bridge state file |
| `BEAR_DB_DIR` | No | `~/Library/Group Containers/9K33E3U3T4.net.shinyfrog.bear/Application Data` | Path to Bear Application Data directory |

### Running

```
export BRIDGE_HUB_URL=https://bear-sync.example.com
export BRIDGE_HUB_TOKEN=<token>
export BEAR_TOKEN=<token>
./bin/bear-bridge
```

The bridge runs once per invocation (no daemon mode). Use launchd to schedule periodic runs.

### Launchd (production)

Install the bridge, bear-xcall, and the launchd agent:

```
make install-bridge
```

This builds both binaries, copies them to `~/bin/`, installs the wrapper script and launchd plist,
and creates a config template at `~/.config/bear-bridge/.env.bridge`.

Edit the config to set your hub URL and tokens:

```
nano ~/.config/bear-bridge/.env.bridge
```

Reload the agent to pick up the new config:

```
launchctl bootout gui/$(id -u)/com.romancha.bear-bridge
launchctl bootstrap gui/$(id -u) ~/Library/LaunchAgents/com.romancha.bear-bridge.plist
```

The bridge runs every 5 minutes and starts automatically on login.

Logs are written to `~/Library/Logs/bear-bridge/` (`stdout.log`, `stderr.log`). The wrapper script automatically rotates logs when they exceed 5 MB, keeping one backup (`.log.1`).

To update to a new version (from source):

```
git pull
make install-bridge
```

Or download the latest release from the [Releases](../../releases) page and run `make install-bridge` from the extracted archive.

To uninstall:

```
make uninstall-bridge
```

## Reverse Proxy

A sample Caddyfile is provided in `deploy/Caddyfile` for systemd setup. For Docker Compose, `deploy/Caddyfile.docker` is used automatically.

The sample Caddyfile uses rate limiting, which requires the [caddy-ratelimit](https://github.com/mholt/caddy-ratelimit) plugin. Build Caddy with this plugin using `xcaddy`:

```
xcaddy build --with github.com/mholt/caddy-ratelimit
```

## API Documentation

The hub serves interactive API documentation via Swagger UI at `/api/docs/` (requires consumer auth).

To regenerate the OpenAPI spec after changing handler annotations:

```
make swagger
```

For a quick start guide with curl examples and integration details, see [docs/consumer-api.md](docs/consumer-api.md).

## Development

```
make test          # run all tests
make test-race     # run tests with race detector
make test-xcall    # run bear-xcall manual tests (macOS + Bear)
make build-xcall   # build bear-xcall .app bundle (macOS only)
make lint          # run golangci-lint
make fmt           # format code
make tidy          # go mod tidy
make swagger       # generate Swagger docs (swag init)
```

## Install from GitHub Release

Pre-built, signed, and notarized binaries are available on the [Releases](../../releases) page.

1. Download the archive for your architecture:
   - `bear-bridge-vX.Y.Z-darwin-arm64.tar.gz` (Apple Silicon)
   - `bear-bridge-vX.Y.Z-darwin-amd64.tar.gz` (Intel)

2. Verify the checksum (optional):

```
shasum -a 256 -c bear-bridge-vX.Y.Z-darwin-arm64.tar.gz.sha256
```

3. Extract and install:

```
tar xzf bear-bridge-vX.Y.Z-darwin-arm64.tar.gz
cd bear-bridge-vX.Y.Z-darwin-arm64
make install-bridge
```

4. Edit the config and reload the agent (see [Bridge Setup](#bridge-setup) above for details).

The release binaries are signed with a Developer ID certificate and notarized by Apple, so macOS Gatekeeper will allow them without warnings.

To verify signatures after install:

```
make verify-bridge
```

## CI/CD

GitHub Actions runs automatically:

- **CI** (push/PR to main): lint, test, test with race detector
- **Docker Publish** (push tag `v*`): builds multi-platform hub image (`linux/amd64`, `linux/arm64`) and pushes to `ghcr.io/romancha/bear-sync-hub`
- **Release Bridge** (push tag `v*`): builds, signs, notarizes, and publishes bridge + bear-xcall for macOS (`arm64`, `amd64`) as GitHub Release assets

### Publishing a release

Tag and push to trigger both Docker and bridge release workflows:

```
git tag v0.1.0
git push origin v0.1.0
```

Pre-release tags (e.g., `v0.1.0-rc.1`) are automatically marked as pre-releases on GitHub.

### Required GitHub secrets for bridge release

The bridge release workflow requires Apple code signing credentials. Set these in the repository settings under Settings > Secrets and variables > Actions:

| Secret | Description |
|---|---|
| `APPLE_CERTIFICATE` | Base64-encoded Developer ID Application .p12 certificate (`base64 -i cert.p12 \| pbcopy`) |
| `APPLE_CERTIFICATE_PASSWORD` | Password for the .p12 certificate |
| `APPLE_TEAM_ID` | Apple Developer Team ID |
| `APPLE_ID` | Apple ID email for notarytool authentication |
| `APPLE_ID_PASSWORD` | App-specific password for notarytool (generate at [appleid.apple.com](https://appleid.apple.com/account/manage)) |

Setup steps:
1. Export your Developer ID Application certificate as .p12 from Keychain Access
2. Base64-encode it: `base64 -i cert.p12 | pbcopy`
3. Create an app-specific password at https://appleid.apple.com/account/manage
4. Add all five secrets in the repository settings
