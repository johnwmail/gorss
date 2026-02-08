package srv

import (
	"database/sql"
	"fmt"
	"html/template"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
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
func (s *Server) Serve(addr string) error {
	// Support env var override
	if envAddr := os.Getenv("GORSS_LISTEN"); envAddr != "" {
		addr = envAddr
	}

	mux := http.NewServeMux()

	// Main app
	mux.HandleFunc("GET /{$}", s.HandleRoot)
	mux.HandleFunc("GET /mobile/", s.HandleMobile)
	mux.HandleFunc("GET /mobile", s.HandleMobile)
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

	mux.HandleFunc("GET /api/categories", s.HandleGetCategories)
	mux.HandleFunc("POST /api/categories", s.HandleCreateCategory)

	// OPML import/export
	mux.HandleFunc("GET /api/opml/export", s.HandleExportOPML)
	mux.HandleFunc("POST /api/opml/import", s.HandleImportOPML)

	mux.HandleFunc("GET /api/counts", s.HandleGetCounts)

	slog.Info("starting server", "addr", addr)
	return http.ListenAndServe(addr, mux)
}

// isMobile checks if the request is from a mobile device
func isMobile(r *http.Request) bool {
	ua := strings.ToLower(r.UserAgent())
	mobileKeywords := []string{"mobile", "android", "iphone", "ipad", "ipod", "blackberry", "windows phone"}
	for _, keyword := range mobileKeywords {
		if strings.Contains(ua, keyword) {
			return true
		}
	}
	return false
}

// HandleRoot serves the main application page (auto-detects mobile)
func (s *Server) HandleRoot(w http.ResponseWriter, r *http.Request) {
	// Auto-redirect mobile devices to mobile page
	if isMobile(r) {
		http.Redirect(w, r, "/mobile/", http.StatusFound)
		return
	}
	s.serveApp(w, r, "index.html")
}

// HandleMobile serves the mobile application page
func (s *Server) HandleMobile(w http.ResponseWriter, r *http.Request) {
	s.serveApp(w, r, "mobile.html")
}

// serveApp serves either desktop or mobile template
func (s *Server) serveApp(w http.ResponseWriter, r *http.Request, templateName string) {
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
	if err := s.renderTemplate(w, templateName, data); err != nil {
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
