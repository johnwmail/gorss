# GoRSS - TT-RSS Clone in Go

A self-hosted RSS reader written in Go, inspired by Tiny Tiny RSS.

## Pre-Commit Requirements

**All code changes MUST pass `golangci-lint` before committing.** Run:

```bash
make lint    # or: golangci-lint run ./...
make test    # also run tests
```

Do NOT commit if either command fails. Fix all lint issues (including cyclomatic complexity, gocyclo ≤ 15) and test failures first.

## Project Overview

GoRSS is a web-based RSS/Atom feed reader that replicates the core functionality of TT-RSS without PHP dependencies.

## Tech Stack

- **Backend**: Go (net/http, html/template)
- **Database**: SQLite (via modernc.org/sqlite)
- **Feed Parsing**: github.com/mmcdole/gofeed
- **Frontend**: Vanilla JS, CSS (no framework)
- **Auth**: exe.dev proxy headers (X-ExeDev-UserID, X-ExeDev-Email)

## Key Design Decisions

1. **SQLite over PostgreSQL/MySQL**: Simpler deployment, no external DB server
2. **Server-side rendering**: HTML templates with minimal JS for interactivity
3. **Vi-style keyboard navigation**: j/k/n/p keys for article navigation
4. **Background feed refresh**: Goroutine-based feed updater

## Directory Structure

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
│   ├── migrations/          # SQL schema migrations
│   │   ├── 001-base.sql
│   │   ├── 002-sort-order.sql
│   │   └── 003-feed-caching.sql  # ETag/Last-Modified/error_count
│   ├── queries/             # sqlc query definitions
│   ├── dbgen/               # sqlc generated code
│   └── sqlc.yaml            # sqlc config
├── Dockerfile               # Multi-stage Docker build
├── docker-compose.yml
├── gorss.service             # systemd unit file
└── Makefile
```

## Running

### Local Development

```bash
go run ./cmd/srv
# or
make build && ./gorss
```

Server runs on port 8080 by default. Override with `GORSS_PORT=3000`.

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
sudo cp gorss.service /etc/systemd/system/gorss.service
sudo systemctl daemon-reload
sudo systemctl enable gorss.service
sudo systemctl start gorss
```

### Kubernetes

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: gorss
spec:
  replicas: 1  # SQLite requires single replica
  selector:
    matchLabels:
      app: gorss
  template:
    metadata:
      labels:
        app: gorss
    spec:
      containers:
      - name: gorss
        image: gorss:latest
        ports:
        - containerPort: 8080
        volumeMounts:
        - name: data
          mountPath: /data
        env:
        - name: GORSS_DB_PATH
          value: /data/gorss.db
        - name: GORSS_PORT
          value: "8080"
      volumes:
      - name: data
        persistentVolumeClaim:
          claimName: gorss-pvc
```

**Note**: SQLite requires a single replica. For multi-replica deployments, migrate to PostgreSQL.

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

## Theme (Day/Night Mode)

- Auto mode uses device local time: 6 AM–9 PM = light, else dark
- Manual override: auto → light → dark (cycle via sidebar footer button)
- CSS custom properties (`--bg`, `--card-bg`, `--text`, etc.) drive all colors
- `[data-theme="dark"]` selector overrides `:root` variables
- `prefers-color-scheme: dark` media query prevents flash on load
- Preference stored in `localStorage` key `gorss-theme-mode`
- 10-minute interval re-checks for auto mode transitions
- Smooth 0.3s CSS transition between themes

## Feed Fetching

- **HTTP conditional GET**: Sends `If-None-Match` (ETag) and `If-Modified-Since` headers; skips parsing/upserting on `304 Not Modified`
- **Article age filtering**: Articles older than `GORSS_PURGE_DAYS` are skipped at ingestion (subscribe, import, refresh)
- **Error backoff**: Feeds with consecutive errors get exponential backoff (2h → 4h → 8h → 24h cap); resets on success
- **Background refresh**: Goroutine refreshes all feeds every `GORSS_REFRESH_INTERVAL`

## Database Backup & Restore

- **Periodic backup**: Set `GORSS_BACKUP_DIR` to enable; backs up every `GORSS_BACKUP_INTERVAL` (default 24h)
- **Prune old backups**: Keeps `GORSS_BACKUP_KEEP` (default 7) most recent copies
- **CLI backup**: `gorss --backup /path/to/dir` for one-time backup
- **CLI restore**: `gorss --restore /path/to/backup.db` with validation and WAL/SHM cleanup
- Uses SQLite `VACUUM INTO` for safe online backup (no locking, no downtime)
- Backup files are portable — copy to clone/migrate the entire app

## Performance

- **Gzip compression** on all responses
- **Lazy-load article content** on expand (list endpoint strips content/summary)
- **Cache-Control** headers for static assets
- **Batch mark-read API** to avoid SQLite write contention
- **SQLite WAL mode** + 5s busy timeout for concurrent read/write
- **Infinite scroll** — articles load in pages of 100, next page fetched automatically
- **New articles badge** — polls counts every 30s, shows · +N new inline in header

## Authentication Modes

- **none**: No authentication required (default)
- **password**: Single password protection, good for personal/family use
- **proxy**: Uses exe.dev proxy headers (X-ExeDev-UserID) for multi-user support
