package srv

import (
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
	"time"

	"srv.exe.dev/db"
	"srv.exe.dev/db/dbgen"
)

type Server struct {
	DB           *sql.DB
	Hostname     string
	TemplatesDir string
	StaticDir    string
	fetcher      *FeedFetcher
}

func New(dbPath, hostname string) (*Server, error) {
	_, thisFile, _, _ := runtime.Caller(0)
	baseDir := filepath.Dir(thisFile)
	srv := &Server{
		Hostname:     hostname,
		TemplatesDir: filepath.Join(baseDir, "templates"),
		StaticDir:    filepath.Join(baseDir, "static"),
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
	mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir(s.StaticDir))))

	// Health check
	mux.HandleFunc("GET /health", s.HandleHealth)

	// API routes
	mux.HandleFunc("GET /api/feeds", s.HandleGetFeeds)
	mux.HandleFunc("POST /api/feeds", s.HandleSubscribe)
	mux.HandleFunc("DELETE /api/feeds/{id}", s.HandleUnsubscribe)

	mux.HandleFunc("GET /api/articles", s.HandleGetArticles)
	mux.HandleFunc("POST /api/articles/{id}/read", s.HandleMarkRead)
	mux.HandleFunc("POST /api/articles/{id}/unread", s.HandleMarkUnread)
	mux.HandleFunc("POST /api/articles/{id}/star", s.HandleStar)
	mux.HandleFunc("POST /api/articles/{id}/unstar", s.HandleUnstar)

	mux.HandleFunc("POST /api/feeds/{id}/mark-read", s.HandleMarkFeedRead)
	mux.HandleFunc("POST /api/refresh", s.HandleRefresh)
	mux.HandleFunc("POST /api/feeds/refresh", s.HandleRefresh) // Alias for JS client

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
	purgeDays := 30
	if envPurge := os.Getenv("GORSS_PURGE_DAYS"); envPurge != "" {
		if parsed, err := strconv.Atoi(envPurge); err == nil {
			purgeDays = parsed
		}
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	slog.Info("starting background feed refresh", "interval", refreshInterval)
	s.StartBackgroundRefresh(ctx, refreshInterval)

	// Start auto-purge if enabled
	if purgeDays > 0 {
		slog.Info("starting auto-purge for old read articles", "days", purgeDays)
		s.StartAutoPurge(ctx, purgeDays)
	}

	// Also do an initial refresh on startup
	go s.refreshAllFeeds(ctx)

	// Apply auth middleware
	authMode := GetAuthMode()
	slog.Info("starting server", "addr", addr, "auth_mode", authMode)

	handler := s.AuthMiddleware(mux)
	return http.ListenAndServe(addr, handler)
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
	q.UpsertUser(r.Context(), dbgen.UpsertUserParams{
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
