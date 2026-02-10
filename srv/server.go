package srv

import (
	"compress/gzip"
	"context"
	"database/sql"
	"fmt"
	"html/template"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/johnwmail/gorss/db"
	"github.com/johnwmail/gorss/db/dbgen"
)

type Server struct {
	DB           *sql.DB
	Hostname     string
	TemplatesDir string
	StaticDir    string
	Version      string // used as cache-buster for static assets
	PurgeDays    int    // articles older than this are filtered on fetch and purged
	fetcher      *FeedFetcher
}

func New(dbPath, hostname, version string) (*Server, error) {
	_, thisFile, _, _ := runtime.Caller(0)
	baseDir := filepath.Dir(thisFile)
	if version == "" {
		version = "dev"
	}
	srv := &Server{
		Hostname:     hostname,
		TemplatesDir: filepath.Join(baseDir, "templates"),
		StaticDir:    filepath.Join(baseDir, "static"),
		Version:      version,
		fetcher:      NewFeedFetcher(),
	}
	if err := srv.setUpDatabase(dbPath); err != nil {
		return nil, err
	}
	return srv, nil
}

// setUpDatabase initializes the database connection and runs migrations
func (s *Server) setUpDatabase(dbPath string) error {
	// Support env var override
	if envPath := os.Getenv("GORSS_DB_PATH"); envPath != "" {
		dbPath = envPath
	}

	wdb, err := db.Open(dbPath)
	if err != nil {
		return fmt.Errorf("failed to open db: %w", err)
	}
	s.DB = wdb
	if err := db.RunMigrations(wdb); err != nil {
		return fmt.Errorf("failed to run migrations: %w", err)
	}
	return nil
}

// Serve starts the HTTP server with the configured routes
func (s *Server) Serve(port string) error {
	// Support env var override
	if envPort := os.Getenv("GORSS_PORT"); envPort != "" {
		port = envPort
	}
	addr := ":" + port

	mux := http.NewServeMux()

	// Login/logout (before auth middleware)
	mux.HandleFunc("GET /login", s.HandleLogin)
	mux.HandleFunc("POST /login", s.HandleLogin)
	mux.HandleFunc("GET /logout", s.HandleLogout)

	// Main app
	mux.HandleFunc("GET /{$}", s.HandleRoot)
	// Legacy mobile routes redirect to main app (now responsive)
	mux.HandleFunc("GET /mobile/", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/", http.StatusMovedPermanently)
	})
	mux.HandleFunc("GET /mobile", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/", http.StatusMovedPermanently)
	})
	staticHandler := http.StripPrefix("/static/", http.FileServer(http.Dir(s.StaticDir)))
	mux.HandleFunc("/static/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		staticHandler.ServeHTTP(w, r)
	})

	// Health check
	mux.HandleFunc("GET /health", s.HandleHealth)

	// API routes
	mux.HandleFunc("GET /api/feeds", s.HandleGetFeeds)
	mux.HandleFunc("POST /api/feeds", s.HandleSubscribe)
	mux.HandleFunc("DELETE /api/feeds/{id}", s.HandleUnsubscribe)

	mux.HandleFunc("GET /api/articles", s.HandleGetArticles)
	mux.HandleFunc("GET /api/articles/{id}", s.HandleGetArticle)
	mux.HandleFunc("POST /api/articles/{id}/read", s.HandleMarkRead)
	mux.HandleFunc("POST /api/articles/{id}/unread", s.HandleMarkUnread)
	mux.HandleFunc("POST /api/articles/{id}/star", s.HandleStar)
	mux.HandleFunc("POST /api/articles/{id}/unstar", s.HandleUnstar)

	mux.HandleFunc("POST /api/feeds/{id}/mark-read", s.HandleMarkFeedRead)
	mux.HandleFunc("POST /api/refresh", s.HandleRefresh)
	mux.HandleFunc("POST /api/feeds/refresh", s.HandleRefresh) // Alias for JS client

	mux.HandleFunc("POST /api/articles/mark-read-batch", s.HandleMarkReadBatch)
	mux.HandleFunc("POST /api/articles/mark-all-read", s.HandleMarkAllRead)

	mux.HandleFunc("GET /api/categories", s.HandleGetCategories)
	mux.HandleFunc("POST /api/categories", s.HandleCreateCategory)
	mux.HandleFunc("PUT /api/categories/reorder", s.HandleReorderCategories)
	mux.HandleFunc("PUT /api/feeds/reorder", s.HandleReorderFeeds)

	// OPML import/export
	mux.HandleFunc("GET /api/opml/export", s.HandleExportOPML)
	mux.HandleFunc("POST /api/opml/import", s.HandleImportOPML)

	mux.HandleFunc("GET /api/counts", s.HandleGetCounts)

	// Start background feed refresh
	refreshInterval := 1 * time.Hour // default 1 hour
	if envInterval := os.Getenv("GORSS_REFRESH_INTERVAL"); envInterval != "" {
		if parsed, err := time.ParseDuration(envInterval); err == nil {
			refreshInterval = parsed
		} else {
			slog.Warn("invalid GORSS_REFRESH_INTERVAL, using default", "value", envInterval, "error", err)
		}
	}

	// Parse purge days setting (default 30 days, 0 to disable)
	s.PurgeDays = 30
	if envPurge := os.Getenv("GORSS_PURGE_DAYS"); envPurge != "" {
		if parsed, err := strconv.Atoi(envPurge); err == nil {
			s.PurgeDays = parsed
		}
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	slog.Info("starting background feed refresh", "interval", refreshInterval)
	s.StartBackgroundRefresh(ctx, refreshInterval)

	// Start auto-purge if enabled
	slog.Info("starting auto-purge for old read articles", "days", s.PurgeDays)
	s.StartAutoPurge(ctx)

	// Also do an initial refresh on startup
	go s.refreshAllFeeds(ctx)

	// Apply auth middleware
	authMode := GetAuthMode()
	slog.Info("starting server", "addr", addr, "auth_mode", authMode)

	handler := gzipMiddleware(s.AuthMiddleware(mux))
	return http.ListenAndServe(addr, handler)
}

// gzip middleware
var gzipPool = sync.Pool{
	New: func() any { w, _ := gzip.NewWriterLevel(nil, gzip.DefaultCompression); return w },
}

type gzipResponseWriter struct {
	http.ResponseWriter
	gz *gzip.Writer
}

func (w *gzipResponseWriter) Write(b []byte) (int, error) { return w.gz.Write(b) }

func gzipMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
			next.ServeHTTP(w, r)
			return
		}
		gz := gzipPool.Get().(*gzip.Writer)
		defer gzipPool.Put(gz)
		gz.Reset(w)
		defer func() { _ = gz.Close() }()

		w.Header().Set("Content-Encoding", "gzip")
		w.Header().Del("Content-Length")
		next.ServeHTTP(&gzipResponseWriter{ResponseWriter: w, gz: gz}, r)
	})
}

// HandleRoot serves the unified responsive application page
func (s *Server) HandleRoot(w http.ResponseWriter, r *http.Request) {
	userID := strings.TrimSpace(r.Header.Get("X-ExeDev-UserID"))
	userEmail := strings.TrimSpace(r.Header.Get("X-ExeDev-Email"))
	now := time.Now()

	if userID == "" {
		userID = "anonymous"
	}

	// Ensure user exists
	q := dbgen.New(s.DB)
	_ = q.UpsertUser(r.Context(), dbgen.UpsertUserParams{
		ID:        userID,
		Email:     &userEmail,
		CreatedAt: now,
		LastSeen:  now,
	})

	// Get counts
	unreadCount, _ := q.GetUnreadCount(r.Context(), userID)
	starredCount, _ := q.GetStarredCount(r.Context(), userID)

	// Get feeds with unread counts
	feeds, _ := q.GetFeeds(r.Context(), userID)

	// Get categories
	categories, _ := q.GetCategories(r.Context(), userID)

	data := map[string]any{
		"Version":      s.Version,
		"Hostname":     s.Hostname,
		"UserEmail":    userEmail,
		"UserID":       userID,
		"UnreadCount":  unreadCount,
		"StarredCount": starredCount,
		"Feeds":        feeds,
		"Categories":   categories,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.renderTemplate(w, "app.html", data); err != nil {
		slog.Warn("render template", "url", r.URL.Path, "error", err)
	}
}

func (s *Server) renderTemplate(w http.ResponseWriter, name string, data any) error {
	path := filepath.Join(s.TemplatesDir, name)
	tmpl, err := template.ParseFiles(path)
	if err != nil {
		return fmt.Errorf("parse template %q: %w", name, err)
	}
	if err := tmpl.Execute(w, data); err != nil {
		return fmt.Errorf("execute template %q: %w", name, err)
	}
	return nil
}
