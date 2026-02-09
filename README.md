# GoRSS - RSS Reader

A self-hosted RSS/Atom feed reader written in Go, inspired by Tiny Tiny RSS.

## Building and Running

### Local Development

```bash
# Build and run
make build
./gorss

# Or run directly
go run ./cmd/srv
```

Server listens on port 8080 by default. Override with `GORSS_PORT=3000 ./gorss`.

### Docker

```bash
# Build and run with docker compose
docker compose up -d

# Or build manually
docker build -t gorss .
docker run -d -p 8080:8080 -v gorss-data:/data gorss
```

### systemd Service

```bash
# Install the service file
sudo cp gorss.service /etc/systemd/system/gorss.service

# Reload systemd and enable the service
sudo systemctl daemon-reload
sudo systemctl enable gorss.service
sudo systemctl start gorss

# Check status / view logs
systemctl status gorss
journalctl -u gorss -f
```

To restart after code changes:

```bash
make build
sudo systemctl restart gorss
```

## Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| GORSS_DB_PATH | ./db.sqlite3 | Path to SQLite database |
| GORSS_PORT | 8080 | Port number to listen on |
| GORSS_REFRESH_INTERVAL | 1h | Feed refresh interval (e.g., 30m, 1h, 2h) |
| GORSS_PURGE_DAYS | 30 | Auto-purge read articles older than X days (0 to disable) |
| GORSS_AUTH_MODE | none | Authentication mode: `none`, `password`, or `proxy` |
| GORSS_PASSWORD | - | Password for `password` auth mode |
| TZ | UTC | Timezone |

## Authentication Modes

- **none**: No authentication required (default)
- **password**: Single password protection, good for personal/family use
- **proxy**: Uses exe.dev proxy headers (X-ExeDev-UserID) for multi-user support

## Database

Uses SQLite (`db.sqlite3`). SQL queries are managed with sqlc.

## Code Layout

- `cmd/srv/` — main package (binary entrypoint)
- `srv/` — HTTP server logic (handlers, feed fetcher, auth)
- `srv/templates/` — Go HTML templates
- `srv/static/` — CSS, JS
- `db/` — SQLite open + migrations
- `db/queries/` — sqlc query definitions
- `db/dbgen/` — generated query code
