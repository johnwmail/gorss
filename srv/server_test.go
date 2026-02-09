package srv

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestServerSetupAndHandlers(t *testing.T) {
	tempDB := filepath.Join(t.TempDir(), "test_server.sqlite3")
	t.Cleanup(func() { os.Remove(tempDB) })

	server, err := New(tempDB, "test-hostname")
	if err != nil {
		t.Fatalf("failed to create server: %v", err)
	}

	// Test root endpoint without auth
	t.Run("root endpoint unauthenticated", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		w := httptest.NewRecorder()

		server.HandleRoot(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("expected status 200, got %d", w.Code)
		}

		body := w.Body.String()
		if !strings.Contains(body, "GoRSS") {
			t.Errorf("expected page to contain GoRSS, got body: %s", body)
		}
	})

	// Test root endpoint with auth headers
	t.Run("root endpoint authenticated", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("X-ExeDev-UserID", "user123")
		req.Header.Set("X-ExeDev-Email", "test@example.com")
		w := httptest.NewRecorder()

		server.HandleRoot(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("expected status 200, got %d", w.Code)
		}

		body := w.Body.String()
		if !strings.Contains(body, "GoRSS") {
			t.Errorf("expected page to contain GoRSS, got body: %s", body)
		}
	})

	// Test health endpoint
	t.Run("health endpoint", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/health", nil)
		w := httptest.NewRecorder()

		server.HandleHealth(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("expected status 200, got %d", w.Code)
		}

		body := w.Body.String()
		if !strings.Contains(body, `"status":"ok"`) {
			t.Errorf("expected health ok response, got: %s", body)
		}
	})
}

func TestAPIEndpoints(t *testing.T) {
	tempDB := filepath.Join(t.TempDir(), "test_api.sqlite3")
	t.Cleanup(func() { os.Remove(tempDB) })

	server, err := New(tempDB, "test-hostname")
	if err != nil {
		t.Fatalf("failed to create server: %v", err)
	}

	t.Run("get feeds empty", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/feeds", nil)
		req.Header.Set("X-ExeDev-UserID", "testuser")
		w := httptest.NewRecorder()

		server.HandleGetFeeds(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("expected status 200, got %d", w.Code)
		}

		body := w.Body.String()
		if body != "[]\n" {
			t.Errorf("expected empty array, got: %s", body)
		}
	})

	t.Run("get articles empty", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/articles", nil)
		req.Header.Set("X-ExeDev-UserID", "testuser")
		w := httptest.NewRecorder()

		server.HandleGetArticles(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("expected status 200, got %d", w.Code)
		}

		body := w.Body.String()
		if body != "[]\n" {
			t.Errorf("expected empty array, got: %s", body)
		}
	})

	t.Run("get counts", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/counts", nil)
		req.Header.Set("X-ExeDev-UserID", "testuser")
		w := httptest.NewRecorder()

		server.HandleGetCounts(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("expected status 200, got %d", w.Code)
		}

		body := w.Body.String()
		if !strings.Contains(body, `"total"`) || !strings.Contains(body, `"unread"`) {
			t.Errorf("expected counts response with total and unread, got: %s", body)
		}
	})
}
