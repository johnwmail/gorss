package srv

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"log/slog"
	"net/http"
	"os"
	"sync"
	"time"
)

// AuthMode defines the authentication mode
type AuthMode string

const (
	AuthModeNone     AuthMode = "none"     // No authentication
	AuthModePassword AuthMode = "password" // Single password
	AuthModeProxy    AuthMode = "proxy"    // Use exe.dev proxy headers
)

// Session store for password auth
var (
	sessions     = make(map[string]time.Time)
	sessionsLock sync.RWMutex
	sessionTTL   = 30 * 24 * time.Hour // 30 days
)

// GetAuthMode returns the current auth mode from environment
func GetAuthMode() AuthMode {
	mode := os.Getenv("GORSS_AUTH_MODE")
	switch mode {
	case "password":
		return AuthModePassword
	case "proxy":
		return AuthModeProxy
	default:
		return AuthModeNone
	}
}

// GetPassword returns the configured password
func GetPassword() string {
	return os.Getenv("GORSS_PASSWORD")
}

// generateSessionID creates a random session ID
func generateSessionID() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return base64.URLEncoding.EncodeToString(b)
}

// validateSession checks if a session is valid
func validateSession(sessionID string) bool {
	sessionsLock.RLock()
	defer sessionsLock.RUnlock()

	expiry, ok := sessions[sessionID]
	if !ok {
		return false
	}
	return time.Now().Before(expiry)
}

// createSession creates a new session
func createSession() string {
	sessionID := generateSessionID()
	sessionsLock.Lock()
	defer sessionsLock.Unlock()

	sessions[sessionID] = time.Now().Add(sessionTTL)
	return sessionID
}

// deleteSession removes a session
func deleteSession(sessionID string) {
	sessionsLock.Lock()
	defer sessionsLock.Unlock()
	delete(sessions, sessionID)
}

// cleanupSessions removes expired sessions
func cleanupSessions() {
	sessionsLock.Lock()
	defer sessionsLock.Unlock()

	now := time.Now()
	for id, expiry := range sessions {
		if now.After(expiry) {
			delete(sessions, id)
		}
	}
}

// AuthMiddleware wraps handlers with authentication
func (s *Server) AuthMiddleware(next http.Handler) http.Handler {
	mode := GetAuthMode()
	password := GetPassword()

	// Start session cleanup goroutine
	go func() {
		for {
			time.Sleep(time.Hour)
			cleanupSessions()
		}
	}()

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Skip auth for static files and health check
		if r.URL.Path == "/health" || 
		   r.URL.Path == "/static/style.css" || 
		   r.URL.Path == "/static/app.js" ||
		   r.URL.Path == "/login" {
			next.ServeHTTP(w, r)
			return
		}

		switch mode {
		case AuthModeNone:
			// No auth required
			next.ServeHTTP(w, r)

		case AuthModePassword:
			if password == "" {
				slog.Warn("password auth enabled but GORSS_PASSWORD not set")
				next.ServeHTTP(w, r)
				return
			}

			// Check session cookie
			cookie, err := r.Cookie("gorss_session")
			if err == nil && validateSession(cookie.Value) {
				next.ServeHTTP(w, r)
				return
			}

			// Redirect to login
			http.Redirect(w, r, "/login", http.StatusFound)

		case AuthModeProxy:
			// Check for proxy headers
			userID := r.Header.Get("X-ExeDev-UserID")
			if userID == "" {
				http.Error(w, "Unauthorized - proxy auth required", http.StatusUnauthorized)
				return
			}
			next.ServeHTTP(w, r)
		}
	})
}

// HandleLogin handles the login page and form submission
func (s *Server) HandleLogin(w http.ResponseWriter, r *http.Request) {
	password := GetPassword()

	if r.Method == "POST" {
		submitted := r.FormValue("password")
		if subtle.ConstantTimeCompare([]byte(submitted), []byte(password)) == 1 {
			// Password correct, create session
			sessionID := createSession()
			http.SetCookie(w, &http.Cookie{
				Name:     "gorss_session",
				Value:    sessionID,
				Path:     "/",
				HttpOnly: true,
				Secure:   r.TLS != nil,
				SameSite: http.SameSiteLaxMode,
				MaxAge:   int(sessionTTL.Seconds()),
			})
			http.Redirect(w, r, "/", http.StatusFound)
			return
		}
		// Wrong password
		s.renderLoginPage(w, "Invalid password")
		return
	}

	s.renderLoginPage(w, "")
}

// HandleLogout handles logout
func (s *Server) HandleLogout(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie("gorss_session")
	if err == nil {
		deleteSession(cookie.Value)
	}

	// Clear cookie
	http.SetCookie(w, &http.Cookie{
		Name:     "gorss_session",
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		MaxAge:   -1,
	})

	http.Redirect(w, r, "/login", http.StatusFound)
}

func (s *Server) renderLoginPage(w http.ResponseWriter, errorMsg string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	html := `<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>GoRSS - Login</title>
  <link rel="icon" type="image/svg+xml" href="/static/favicon.svg">
  <link rel="icon" type="image/x-icon" href="/static/favicon.ico" sizes="16x16 32x32">
  <style>
    * { box-sizing: border-box; margin: 0; padding: 0; }
    :root {
      --bg: #f5f5f5;
      --card-bg: white;
      --text: #333;
      --text-muted: #666;
      --primary: #1a73e8;
      --primary-hover: #1557b0;
      --border: #e0e0e0;
      --shadow: rgba(0,0,0,0.1);
      --error-bg: #ffebee;
      --error-text: #c62828;
    }
    @media (prefers-color-scheme: dark) {
      :root:not([data-theme="light"]) {
        --bg: #2b2d31;
        --card-bg: #36393f;
        --text: #dcddde;
        --text-muted: #999;
        --primary: #7bafe8;
        --primary-hover: #6a9fd8;
        --border: #4a4d52;
        --shadow: rgba(0,0,0,0.3);
        --error-bg: #442222;
        --error-text: #ff6b6b;
      }
    }
    [data-theme="dark"] {
      --bg: #2b2d31;
      --card-bg: #36393f;
      --text: #dcddde;
      --text-muted: #999;
      --primary: #7bafe8;
      --primary-hover: #6a9fd8;
      --border: #4a4d52;
      --shadow: rgba(0,0,0,0.3);
      --error-bg: #442222;
      --error-text: #ff6b6b;
    }
    body {
      font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
      background: var(--bg);
      color: var(--text);
      min-height: 100vh;
      display: flex;
      align-items: center;
      justify-content: center;
      transition: background 0.3s, color 0.3s;
    }
    .login-box {
      background: var(--card-bg);
      padding: 40px;
      border-radius: 12px;
      box-shadow: 0 4px 20px var(--shadow);
      width: 100%;
      max-width: 400px;
      transition: background 0.3s;
    }
    h1 {
      color: var(--primary);
      margin-bottom: 8px;
      font-size: 28px;
    }
    .subtitle {
      color: var(--text-muted);
      margin-bottom: 24px;
    }
    .error {
      background: var(--error-bg);
      color: var(--error-text);
      padding: 12px;
      border-radius: 8px;
      margin-bottom: 16px;
    }
    input[type="password"] {
      width: 100%;
      padding: 14px;
      border: 2px solid var(--border);
      border-radius: 8px;
      font-size: 16px;
      margin-bottom: 16px;
      transition: border-color 0.2s;
      background: var(--bg);
      color: var(--text);
    }
    input[type="password"]:focus {
      outline: none;
      border-color: var(--primary);
    }
    button {
      width: 100%;
      padding: 14px;
      background: var(--primary);
      color: white;
      border: none;
      border-radius: 8px;
      font-size: 16px;
      font-weight: 500;
      cursor: pointer;
      transition: background 0.2s;
    }
    button:hover {
      background: var(--primary-hover);
    }
    .version-link {
      display: flex;
      align-items: center;
      justify-content: center;
      gap: 4px;
      margin-top: 20px;
      font-size: 13px;
      color: var(--text-muted);
      text-decoration: none;
      transition: color 0.2s;
    }
    .version-link:hover {
      color: var(--primary);
    }
    .version-link svg {
      width: 14px;
      height: 14px;
      fill: currentColor;
    }
    h1 a {
      color: inherit;
      text-decoration: none;
    }
    h1 a:hover {
      text-decoration: underline;
    }
  </style>
</head>
<body>
  <div class="login-box">
    <h1>📰 <a href="https://github.com/johnwmail/gorss" target="_blank" rel="noopener">GoRSS</a></h1>
    <p class="subtitle">Enter password to continue</p>
    ` + errorHTML(errorMsg) + `
    <form method="POST">
      <input type="password" name="password" placeholder="Password" autofocus required>
      <button type="submit">Login</button>
    </form>
    <a href="https://github.com/johnwmail/gorss/pkgs/container/gorss" target="_blank" rel="noopener" class="version-link"><svg viewBox="0 0 16 16"><path d="M8 0C3.58 0 0 3.58 0 8c0 3.54 2.29 6.53 5.47 7.59.4.07.55-.17.55-.38 0-.19-.01-.82-.01-1.49-2.01.37-2.53-.49-2.69-.94-.09-.23-.48-.94-.82-1.13-.28-.15-.68-.52-.01-.53.63-.01 1.08.58 1.23.82.72 1.21 1.87.87 2.33.66.07-.52.28-.87.51-1.07-1.78-.2-3.64-.89-3.64-3.95 0-.87.31-1.59.82-2.15-.08-.2-.36-1.02.08-2.12 0 0 .67-.21 2.2.82.64-.18 1.32-.27 2-.27.68 0 1.36.09 2 .27 1.53-1.04 2.2-.82 2.2-.82.44 1.1.16 1.92.08 2.12.51.56.82 1.27.82 2.15 0 3.07-1.87 3.75-3.65 3.95.29.25.54.73.54 1.48 0 1.07-.01 1.93-.01 2.2 0 .21.15.46.55.38A8.01 8.01 0 0016 8c0-4.42-3.58-8-8-8z"/></svg>` + s.Version + `</a>
  </div>
  <script>
    // Respect the same theme preference as the main app
    (function() {
      var mode = localStorage.getItem('gorss-theme-mode') || 'auto';
      if (mode === 'dark') { document.documentElement.dataset.theme = 'dark'; }
      else if (mode === 'light') { document.documentElement.dataset.theme = 'light'; }
      else {
        var hour = new Date().getHours();
        if (hour < 6 || hour >= 21) { document.documentElement.dataset.theme = 'dark'; }
      }
    })();
  </script>
</body>
</html>`
	_, _ = w.Write([]byte(html))
}

func errorHTML(msg string) string {
	if msg == "" {
		return ""
	}
	return `<div class="error">` + msg + `</div>`
}
