# GoRSS - TT-RSS Clone in Go

A self-hosted RSS reader written in Go, inspired by Tiny Tiny RSS.

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
├── cmd/srv/main.go      # Entry point
├── srv/
│   ├── server.go        # HTTP server & routes
│   ├── handlers.go      # Request handlers
│   ├── feed.go          # Feed fetching logic
│   ├── templates/       # HTML templates
│   └── static/          # CSS, JS
├── db/
│   ├── migrations/      # SQL migrations
│   ├── queries/         # sqlc queries
│   └── dbgen/           # Generated code
└── PLAN.md              # Implementation plan
```

## Running

### Local Development

```bash
go run ./cmd/srv
# or
make run
```

Server runs on :8000 by default.

### Docker

```bash
# Build and run with docker-compose
docker-compose up -d

# Or build manually
docker build -t gorss .
docker run -d -p 8000:8000 -v gorss-data:/data gorss
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
        - containerPort: 8000
        volumeMounts:
        - name: data
          mountPath: /data
        env:
        - name: GORSS_DB_PATH
          value: /data/gorss.db
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
| GORSS_LISTEN | :8000 | Listen address |
| GORSS_REFRESH_INTERVAL | 1h | Feed refresh interval (e.g., 30m, 1h, 2h) |
| TZ | UTC | Timezone |
