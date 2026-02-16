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

## Theme (Day/Night Mode)

GoRSS includes automatic day/night theme switching:

- **Auto** (default): Uses device local time — 6 AM to 9 PM = light, otherwise dark. Re-checks every 10 minutes.
- **Light**: Always light theme
- **Dark**: Always dark theme

Click the theme toggle button (🌗/☀️/🌙) in the sidebar footer to cycle modes. Your preference is saved in the browser's localStorage.

The app also respects the OS-level `prefers-color-scheme` media query as a fallback to prevent flash of wrong theme on initial load.

## Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| GORSS_DB_PATH | ./db.sqlite3 | Path to SQLite database |
| GORSS_PORT | 8080 | Port number to listen on |
| GORSS_REFRESH_INTERVAL | 1h | Feed refresh interval (e.g., 30m, 1h, 2h) |
| GORSS_PURGE_DAYS | 30 | Auto-purge read articles older than X days (0 to disable) |
| GORSS_BACKUP_DIR | - | Directory for periodic backups (disabled if unset) |
| GORSS_BACKUP_INTERVAL | 24h | Backup interval (e.g., 12h, 24h) |
| GORSS_BACKUP_KEEP | 7 | Number of backup files to keep |
| GORSS_AUTH_MODE | none | Authentication mode: `none`, `password`, or `proxy` |
| GORSS_PASSWORD | - | Password for `password` auth mode |
| TZ | UTC | Timezone |

## Authentication Modes

- **none**: No authentication required (default)
- **password**: Single password protection, good for personal/family use
- **proxy**: Uses exe.dev proxy headers (X-ExeDev-UserID) for multi-user support

## Database Backup & Restore

GoRSS supports automatic periodic backups and manual backup/restore via CLI.

### Automatic Periodic Backup

Set `GORSS_BACKUP_DIR` to enable. Backups run every `GORSS_BACKUP_INTERVAL` (default 24h), keeping the most recent `GORSS_BACKUP_KEEP` copies (default 7).

```bash
# systemd / bare metal
export GORSS_BACKUP_DIR=/home/exedev/gorss/data/backups
./gorss

# Docker — backups go inside the data volume at /data/backups
# Already configured in docker-compose.yml
```

For Docker, to keep backups on the host (survives volume deletion):

```yaml
volumes:
  - gorss-data:/data
  - /host/path/backups:/backups
environment:
  - GORSS_BACKUP_DIR=/backups
```

### Manual One-Time Backup

```bash
./gorss -backup /path/to/backup/dir
# Output: Backup saved to: /path/to/backup/dir/gorss-2026-02-16-030000.db
```

### Restore from Backup

```bash
# Stop the server first!
sudo systemctl stop gorss

# Restore (interactive confirmation)
./gorss -restore /path/to/backup/gorss-2026-02-16-030000.db

# Restart
sudo systemctl start gorss
```

The restore command validates the backup is a real SQLite database and automatically cleans up stale WAL/SHM files.

### Clone / Migrate to Another Server

Since the entire app state is one SQLite file, migration is trivial:

```bash
scp /backups/gorss-latest.db new-server:/data/gorss.db
# On new server:
GORSS_DB_PATH=/data/gorss.db ./gorss
```

## Database

Uses SQLite with WAL mode. SQL queries are managed with sqlc.

## Code Layout

```
gorss/
├── cmd/srv/
│   └── main.go              # Entry point, CLI flags (--backup, --restore)
├── srv/
│   ├── server.go            # HTTP server, routes, middleware
│   ├── handlers.go          # API request handlers
│   ├── feed.go              # RSS/Atom feed fetching, parsing & background jobs
│   ├── auth.go              # Authentication (password/proxy modes)
│   ├── opml.go              # OPML import/export
│   ├── server_test.go       # Tests
│   ├── static/
│   │   ├── app.css          # Stylesheet
│   │   ├── app.js           # Frontend JavaScript
│   │   ├── favicon.svg      # Favicon (SVG)
│   │   └── favicon.ico      # Favicon (ICO fallback)
│   └── templates/
│       ├── app.html         # Main app template
│       └── welcome.html     # Login page template
├── db/
│   ├── db.go               # Database open & migration runner
│   ├── backup.go           # Backup, restore & prune functions
│   ├── backup_test.go      # Backup/restore tests
│   ├── migrations/
│   │   ├── 001-base.sql     # Initial schema
│   │   ├── 002-sort-order.sql
│   │   └── 003-feed-caching.sql  # ETag/Last-Modified/error_count
│   ├── queries/
│   │   └── visitors.sql     # sqlc query definitions
│   ├── dbgen/               # sqlc generated code
│   └── sqlc.yaml            # sqlc config
├── .github/workflows/
│   ├── test.yml             # CI test workflow
│   └── build-container.yml  # Container build workflow
├── .golangci.yml             # Linter configuration
├── Dockerfile               # Multi-stage Docker build
├── docker-compose.yml
├── gorss.service             # systemd unit file
├── Makefile
├── go.mod
└── go.sum
```
