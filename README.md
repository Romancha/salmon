# bear-sync

Syncs Bear notes with openclaw. Two components: **hub** (API server on VPS) and **bridge** (Mac agent that reads Bear SQLite).

## Architecture

- Bear is source-of-truth for user content
- Hub is a read replica with a write queue for openclaw
- Read flow: Bear → bridge → hub → openclaw API
- Write flow: openclaw → hub write_queue → bridge → Bear x-callback-url → ack

## Prerequisites

- Go 1.24+
- Bear.app (for bridge)
- [xcall](https://github.com/nicoulaj/xcall) CLI (for bridge write operations)

## Build

```
make build
```

Binaries are placed in `bin/bear-sync-hub` and `bin/bear-bridge`.

## Hub Setup

### Environment Variables

| Variable | Required | Default | Description |
|---|---|---|---|
| `HUB_PORT` | No | `8080` | Listen port |
| `HUB_DB_PATH` | Yes | — | Path to SQLite database file |
| `HUB_OPENCLAW_TOKEN` | Yes | — | Bearer token for openclaw API access |
| `HUB_BRIDGE_TOKEN` | Yes | — | Bearer token for bridge sync access |
| `HUB_ATTACHMENTS_DIR` | No | `attachments` | Directory for attachment file storage |

### Running

```
export HUB_DB_PATH=/opt/bear-sync/data/hub.db
export HUB_OPENCLAW_TOKEN=<token>
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

## Bridge Setup

### Environment Variables

| Variable | Required | Default | Description |
|---|---|---|---|
| `BRIDGE_HUB_URL` | Yes | — | Hub API URL (e.g. `https://bear-sync.example.com`) |
| `BRIDGE_HUB_TOKEN` | Yes | — | Bearer token matching `HUB_BRIDGE_TOKEN` |
| `BEAR_TOKEN` | Yes | — | Bear app API token (from Bear preferences) |
| `BRIDGE_STATE_PATH` | No | `~/.bear-bridge-state.json` | Path to bridge state file |
| `BEAR_DB_DIR` | No | Bear default | Path to Bear Application Data directory |

### Running

```
export BRIDGE_HUB_URL=https://bear-sync.example.com
export BRIDGE_HUB_TOKEN=<token>
export BEAR_TOKEN=<token>
./bin/bear-bridge
```

The bridge runs once per invocation (no daemon mode). Use launchd to schedule periodic runs.

### Launchd (production)

```
cp deploy/com.romancha.bear-bridge.plist ~/Library/LaunchAgents/
launchctl load ~/Library/LaunchAgents/com.romancha.bear-bridge.plist
```

Edit the plist to set your tokens and hub URL. Default interval: every 5 minutes.

## Reverse Proxy

A sample Caddyfile is provided in `deploy/Caddyfile`. It configures TLS (Let's Encrypt) and rate limiting.

## Development

```
make test          # run all tests
make test-race     # run tests with race detector
make lint          # run golangci-lint
make fmt           # format code
make tidy          # go mod tidy
```
