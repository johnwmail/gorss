package srv

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"srv.exe.dev/db/dbgen"
)

// newTestServer creates a Server with a temp SQLite DB.
func newTestServer(t *testing.T) *Server {
	t.Helper()
	db := filepath.Join(t.TempDir(), "test.sqlite3")
	s, err := New(db, "test-host")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return s
}

// authReq creates a request with the test user header set.
func authReq(method, url string, body string) *http.Request {
	var r *http.Request
	if body != "" {
		r = httptest.NewRequest(method, url, strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
	} else {
		r = httptest.NewRequest(method, url, nil)
	}
	r.Header.Set("X-ExeDev-UserID", "testuser")
	return r
}

// seedFeed inserts a feed and n articles for testuser, returning the feed.
func seedFeed(t *testing.T, s *Server, title string, catID *int64, n int) dbgen.Feed {
	t.Helper()
	q := dbgen.New(s.DB)
	ctx := context.Background()

	// Ensure user exists (FK constraint)
	email := "test@example.com"
	now := time.Now()
	_ = q.UpsertUser(ctx, dbgen.UpsertUserParams{
		ID: "testuser", Email: &email, CreatedAt: now, LastSeen: now,
	})

	feed, err := q.CreateFeed(ctx, dbgen.CreateFeedParams{
		UserID: "testuser", Url: "http://example.com/" + title,
		Title: title, CategoryID: catID,
	})
	if err != nil {
		t.Fatalf("CreateFeed: %v", err)
	}
	for i := range n {
		guid := title + "-" + strings.Repeat("a", i+1)
		_, _ = q.UpsertArticle(ctx, dbgen.UpsertArticleParams{
			FeedID: feed.ID, Guid: guid,
			Url: "http://example.com/" + guid, Title: "Article " + guid,
			Content: "<p>content</p>", PublishedAt: &now,
		})
	}
	return feed
}

// decodeJSON unmarshals the recorder body into v.
func decodeJSON(t *testing.T, w *httptest.ResponseRecorder, v any) {
	t.Helper()
	if err := json.NewDecoder(w.Body).Decode(v); err != nil {
		t.Fatalf("decode json: %v (body=%s)", err, w.Body.String())
	}
}

// assertStatus checks the response code.
func assertStatus(t *testing.T, w *httptest.ResponseRecorder, want int) {
	t.Helper()
	if w.Code != want {
		t.Errorf("status = %d, want %d (body=%s)", w.Code, want, w.Body.String())
	}
}

// --------------- Server & Health ---------------

func TestHealth(t *testing.T) {
	s := newTestServer(t)
	w := httptest.NewRecorder()
	s.HandleHealth(w, httptest.NewRequest("GET", "/health", nil))
	assertStatus(t, w, 200)
	var m map[string]string
	decodeJSON(t, w, &m)
	if m["status"] != "ok" {
		t.Errorf("health status = %q", m["status"])
	}
}

func TestRootPage(t *testing.T) {
	s := newTestServer(t)

	t.Run("unauthenticated", func(t *testing.T) {
		w := httptest.NewRecorder()
		s.HandleRoot(w, httptest.NewRequest("GET", "/", nil))
		assertStatus(t, w, 200)
		if !strings.Contains(w.Body.String(), "GoRSS") {
			t.Error("page should contain GoRSS")
		}
	})

	t.Run("authenticated", func(t *testing.T) {
		w := httptest.NewRecorder()
		r := authReq("GET", "/", "")
		s.HandleRoot(w, r)
		assertStatus(t, w, 200)
	})
}

// --------------- Categories ---------------

func TestCategories(t *testing.T) {
	s := newTestServer(t)

	t.Run("empty list", func(t *testing.T) {
		w := httptest.NewRecorder()
		s.HandleGetCategories(w, authReq("GET", "/api/categories", ""))
		assertStatus(t, w, 200)
		var cats []dbgen.Category
		decodeJSON(t, w, &cats)
		if len(cats) != 0 {
			t.Errorf("expected 0 categories, got %d", len(cats))
		}
	})

	t.Run("create", func(t *testing.T) {
		w := httptest.NewRecorder()
		s.HandleCreateCategory(w, authReq("POST", "/api/categories", `{"title":"Tech"}`))
		assertStatus(t, w, 200)
		var cat dbgen.Category
		decodeJSON(t, w, &cat)
		if cat.Title != "Tech" {
			t.Errorf("title = %q, want Tech", cat.Title)
		}
		if cat.ID == 0 {
			t.Error("expected non-zero ID")
		}
	})

	t.Run("create missing title", func(t *testing.T) {
		w := httptest.NewRecorder()
		s.HandleCreateCategory(w, authReq("POST", "/api/categories", `{"title":""}`))
		assertStatus(t, w, 400)
	})

	t.Run("create bad json", func(t *testing.T) {
		w := httptest.NewRecorder()
		s.HandleCreateCategory(w, authReq("POST", "/api/categories", `not json`))
		assertStatus(t, w, 400)
	})

	t.Run("list after create", func(t *testing.T) {
		w := httptest.NewRecorder()
		s.HandleGetCategories(w, authReq("GET", "/api/categories", ""))
		assertStatus(t, w, 200)
		var cats []dbgen.Category
		decodeJSON(t, w, &cats)
		if len(cats) != 1 {
			t.Errorf("expected 1 category, got %d", len(cats))
		}
	})
}

// --------------- Feeds ---------------

func TestFeeds(t *testing.T) {
	s := newTestServer(t)

	t.Run("empty list", func(t *testing.T) {
		w := httptest.NewRecorder()
		s.HandleGetFeeds(w, authReq("GET", "/api/feeds", ""))
		assertStatus(t, w, 200)
		if w.Body.String() != "[]\n" {
			t.Errorf("expected [], got %s", w.Body.String())
		}
	})

	t.Run("subscribe missing url", func(t *testing.T) {
		w := httptest.NewRecorder()
		s.HandleSubscribe(w, authReq("POST", "/api/feeds", `{"url":""}`))
		assertStatus(t, w, 400)
	})

	t.Run("subscribe bad json", func(t *testing.T) {
		w := httptest.NewRecorder()
		s.HandleSubscribe(w, authReq("POST", "/api/feeds", `bad`))
		assertStatus(t, w, 400)
	})

	t.Run("unsubscribe invalid id", func(t *testing.T) {
		w := httptest.NewRecorder()
		r := authReq("DELETE", "/api/feeds/abc", "")
		r.SetPathValue("id", "abc")
		s.HandleUnsubscribe(w, r)
		assertStatus(t, w, 400)
	})

	t.Run("list seeded feeds", func(t *testing.T) {
		seedFeed(t, s, "feed1", nil, 3)
		w := httptest.NewRecorder()
		s.HandleGetFeeds(w, authReq("GET", "/api/feeds", ""))
		assertStatus(t, w, 200)
		var feeds []dbgen.Feed
		decodeJSON(t, w, &feeds)
		if len(feeds) < 1 {
			t.Error("expected at least 1 feed")
		}
	})

	t.Run("unsubscribe valid feed", func(t *testing.T) {
		f := seedFeed(t, s, "to-delete", nil, 0)
		w := httptest.NewRecorder()
		r := authReq("DELETE", "/api/feeds/"+fmt.Sprint(f.ID), "")
		r.SetPathValue("id", fmt.Sprint(f.ID))
		s.HandleUnsubscribe(w, r)
		assertStatus(t, w, 200)
	})
}



// --------------- Articles & State ---------------

func TestArticles(t *testing.T) {
	s := newTestServer(t)
	feed := seedFeed(t, s, "testfeed", nil, 5)

	// Fetch all articles to get IDs
	q := dbgen.New(s.DB)
	ctx := context.Background()
	arts, _ := q.GetArticlesByFeed(ctx, dbgen.GetArticlesByFeedParams{
		UserID: "testuser", ID: feed.ID, UserID_2: "testuser", Limit: 100,
	})
	if len(arts) != 5 {
		t.Fatalf("expected 5 articles, got %d", len(arts))
	}
	id1 := fmt.Sprint(arts[0].ID)

	t.Run("get all articles", func(t *testing.T) {
		w := httptest.NewRecorder()
		s.HandleGetArticles(w, authReq("GET", "/api/articles", ""))
		assertStatus(t, w, 200)
		var list []json.RawMessage
		decodeJSON(t, w, &list)
		if len(list) != 5 {
			t.Errorf("got %d articles, want 5", len(list))
		}
	})

	t.Run("get unread articles", func(t *testing.T) {
		w := httptest.NewRecorder()
		s.HandleGetArticles(w, authReq("GET", "/api/articles?view=unread", ""))
		assertStatus(t, w, 200)
		var list []json.RawMessage
		decodeJSON(t, w, &list)
		if len(list) != 5 {
			t.Errorf("got %d unread, want 5", len(list))
		}
	})

	t.Run("get by feed_id", func(t *testing.T) {
		w := httptest.NewRecorder()
		s.HandleGetArticles(w, authReq("GET", "/api/articles?feed_id="+fmt.Sprint(feed.ID), ""))
		assertStatus(t, w, 200)
		var list []json.RawMessage
		decodeJSON(t, w, &list)
		if len(list) != 5 {
			t.Errorf("got %d, want 5", len(list))
		}
	})

	t.Run("get with limit and offset", func(t *testing.T) {
		w := httptest.NewRecorder()
		s.HandleGetArticles(w, authReq("GET", "/api/articles?limit=2&offset=0", ""))
		assertStatus(t, w, 200)
		var list []json.RawMessage
		decodeJSON(t, w, &list)
		if len(list) != 2 {
			t.Errorf("got %d, want 2", len(list))
		}
	})

	t.Run("get starred empty", func(t *testing.T) {
		w := httptest.NewRecorder()
		s.HandleGetArticles(w, authReq("GET", "/api/articles?view=starred", ""))
		assertStatus(t, w, 200)
		var list []json.RawMessage
		decodeJSON(t, w, &list)
		if len(list) != 0 {
			t.Errorf("got %d starred, want 0", len(list))
		}
	})

	t.Run("mark read", func(t *testing.T) {
		w := httptest.NewRecorder()
		r := authReq("POST", "/api/articles/"+id1+"/read", "")
		r.SetPathValue("id", id1)
		s.HandleMarkRead(w, r)
		assertStatus(t, w, 200)

		// Verify unread count dropped
		w2 := httptest.NewRecorder()
		s.HandleGetArticles(w2, authReq("GET", "/api/articles?view=unread", ""))
		var list []json.RawMessage
		decodeJSON(t, w2, &list)
		if len(list) != 4 {
			t.Errorf("got %d unread after mark read, want 4", len(list))
		}
	})

	t.Run("mark read invalid id", func(t *testing.T) {
		w := httptest.NewRecorder()
		r := authReq("POST", "/api/articles/bad/read", "")
		r.SetPathValue("id", "bad")
		s.HandleMarkRead(w, r)
		assertStatus(t, w, 400)
	})

	t.Run("mark unread", func(t *testing.T) {
		w := httptest.NewRecorder()
		r := authReq("POST", "/api/articles/"+id1+"/unread", "")
		r.SetPathValue("id", id1)
		s.HandleMarkUnread(w, r)
		assertStatus(t, w, 200)

		w2 := httptest.NewRecorder()
		s.HandleGetArticles(w2, authReq("GET", "/api/articles?view=unread", ""))
		var list []json.RawMessage
		decodeJSON(t, w2, &list)
		if len(list) != 5 {
			t.Errorf("got %d unread after mark unread, want 5", len(list))
		}
	})

	t.Run("mark unread invalid id", func(t *testing.T) {
		w := httptest.NewRecorder()
		r := authReq("POST", "/api/articles/xyz/unread", "")
		r.SetPathValue("id", "xyz")
		s.HandleMarkUnread(w, r)
		assertStatus(t, w, 400)
	})

	t.Run("star", func(t *testing.T) {
		w := httptest.NewRecorder()
		r := authReq("POST", "/api/articles/"+id1+"/star", "")
		r.SetPathValue("id", id1)
		s.HandleStar(w, r)
		assertStatus(t, w, 200)

		w2 := httptest.NewRecorder()
		s.HandleGetArticles(w2, authReq("GET", "/api/articles?view=starred", ""))
		var list []json.RawMessage
		decodeJSON(t, w2, &list)
		if len(list) != 1 {
			t.Errorf("got %d starred, want 1", len(list))
		}
	})

	t.Run("star invalid id", func(t *testing.T) {
		w := httptest.NewRecorder()
		r := authReq("POST", "/api/articles/nope/star", "")
		r.SetPathValue("id", "nope")
		s.HandleStar(w, r)
		assertStatus(t, w, 400)
	})

	t.Run("unstar", func(t *testing.T) {
		w := httptest.NewRecorder()
		r := authReq("POST", "/api/articles/"+id1+"/unstar", "")
		r.SetPathValue("id", id1)
		s.HandleUnstar(w, r)
		assertStatus(t, w, 200)

		w2 := httptest.NewRecorder()
		s.HandleGetArticles(w2, authReq("GET", "/api/articles?view=starred", ""))
		var list []json.RawMessage
		decodeJSON(t, w2, &list)
		if len(list) != 0 {
			t.Errorf("got %d starred after unstar, want 0", len(list))
		}
	})

	t.Run("unstar invalid id", func(t *testing.T) {
		w := httptest.NewRecorder()
		r := authReq("POST", "/api/articles/!/unstar", "")
		r.SetPathValue("id", "!")
		s.HandleUnstar(w, r)
		assertStatus(t, w, 400)
	})
}

// --------------- Mark All / Feed Read ---------------

func TestMarkAllRead(t *testing.T) {
	s := newTestServer(t)
	seedFeed(t, s, "f1", nil, 3)
	seedFeed(t, s, "f2", nil, 2)

	// Before: 5 unread
	w := httptest.NewRecorder()
	s.HandleGetArticles(w, authReq("GET", "/api/articles?view=unread", ""))
	var before []json.RawMessage
	decodeJSON(t, w, &before)
	if len(before) != 5 {
		t.Fatalf("expected 5 unread, got %d", len(before))
	}

	// Mark all read
	w = httptest.NewRecorder()
	s.HandleMarkAllRead(w, authReq("POST", "/api/mark-all-read", ""))
	assertStatus(t, w, 200)

	// After: 0 unread
	w = httptest.NewRecorder()
	s.HandleGetArticles(w, authReq("GET", "/api/articles?view=unread", ""))
	var after []json.RawMessage
	decodeJSON(t, w, &after)
	if len(after) != 0 {
		t.Errorf("expected 0 unread after mark-all-read, got %d", len(after))
	}
}

func TestMarkFeedRead(t *testing.T) {
	s := newTestServer(t)
	f1 := seedFeed(t, s, "f1", nil, 3)
	seedFeed(t, s, "f2", nil, 2)

	w := httptest.NewRecorder()
	r := authReq("POST", "/api/feeds/"+fmt.Sprint(f1.ID)+"/mark-read", "")
	r.SetPathValue("id", fmt.Sprint(f1.ID))
	s.HandleMarkFeedRead(w, r)
	assertStatus(t, w, 200)

	// Only f2's articles remain unread
	w = httptest.NewRecorder()
	s.HandleGetArticles(w, authReq("GET", "/api/articles?view=unread", ""))
	var list []json.RawMessage
	decodeJSON(t, w, &list)
	if len(list) != 2 {
		t.Errorf("expected 2 unread, got %d", len(list))
	}
}

func TestMarkFeedReadInvalidID(t *testing.T) {
	s := newTestServer(t)
	w := httptest.NewRecorder()
	r := authReq("POST", "/api/feeds/bad/mark-read", "")
	r.SetPathValue("id", "bad")
	s.HandleMarkFeedRead(w, r)
	assertStatus(t, w, 400)
}

// --------------- Counts ---------------

func TestCounts(t *testing.T) {
	s := newTestServer(t)
	feed := seedFeed(t, s, "counted", nil, 4)

	w := httptest.NewRecorder()
	s.HandleGetCounts(w, authReq("GET", "/api/counts", ""))
	assertStatus(t, w, 200)

	var counts struct {
		Total   int64            `json:"total"`
		Unread  int64            `json:"unread"`
		Starred int64            `json:"starred"`
		Feeds   map[string]int64 `json:"feeds"`
	}
	decodeJSON(t, w, &counts)
	if counts.Total != 4 {
		t.Errorf("total = %d, want 4", counts.Total)
	}
	if counts.Unread != 4 {
		t.Errorf("unread = %d, want 4", counts.Unread)
	}
	if counts.Starred != 0 {
		t.Errorf("starred = %d, want 0", counts.Starred)
	}
	fidStr := fmt.Sprint(feed.ID)
	if counts.Feeds[fidStr] != 4 {
		t.Errorf("feed count = %d, want 4", counts.Feeds[fidStr])
	}
}

// --------------- Category-filtered Articles ---------------

func TestArticlesByCategory(t *testing.T) {
	s := newTestServer(t)
	q := dbgen.New(s.DB)
	ctx := context.Background()

	// Ensure user exists
	email := "test@example.com"
	now := time.Now()
	_ = q.UpsertUser(ctx, dbgen.UpsertUserParams{
		ID: "testuser", Email: &email, CreatedAt: now, LastSeen: now,
	})

	cat, err := q.CreateCategory(ctx, dbgen.CreateCategoryParams{UserID: "testuser", Title: "Go"})
	if err != nil {
		t.Fatal(err)
	}
	catID := cat.ID

	seedFeed(t, s, "go-blog", &catID, 3)
	seedFeed(t, s, "uncategorized", nil, 2) // no category

	t.Run("by category", func(t *testing.T) {
		w := httptest.NewRecorder()
		s.HandleGetArticles(w, authReq("GET", "/api/articles?category_id="+fmt.Sprint(catID), ""))
		assertStatus(t, w, 200)
		var list []json.RawMessage
		decodeJSON(t, w, &list)
		if len(list) != 3 {
			t.Errorf("got %d, want 3", len(list))
		}
	})

	t.Run("by category unread", func(t *testing.T) {
		w := httptest.NewRecorder()
		s.HandleGetArticles(w, authReq("GET", "/api/articles?category_id="+fmt.Sprint(catID)+"&view=unread", ""))
		assertStatus(t, w, 200)
		var list []json.RawMessage
		decodeJSON(t, w, &list)
		if len(list) != 3 {
			t.Errorf("got %d, want 3", len(list))
		}
	})

	t.Run("uncategorized (catId=0)", func(t *testing.T) {
		w := httptest.NewRecorder()
		s.HandleGetArticles(w, authReq("GET", "/api/articles?category_id=0", ""))
		assertStatus(t, w, 200)
		var list []json.RawMessage
		decodeJSON(t, w, &list)
		if len(list) != 2 {
			t.Errorf("got %d uncategorized, want 2", len(list))
		}
	})

	t.Run("uncategorized unread (catId=0)", func(t *testing.T) {
		w := httptest.NewRecorder()
		s.HandleGetArticles(w, authReq("GET", "/api/articles?category_id=0&view=unread", ""))
		assertStatus(t, w, 200)
		var list []json.RawMessage
		decodeJSON(t, w, &list)
		if len(list) != 2 {
			t.Errorf("got %d, want 2", len(list))
		}
	})
}

// --------------- Refresh ---------------

func TestRefresh(t *testing.T) {
	s := newTestServer(t)
	w := httptest.NewRecorder()
	s.HandleRefresh(w, authReq("POST", "/api/feeds/refresh", ""))
	assertStatus(t, w, 200)
	var m map[string]string
	decodeJSON(t, w, &m)
	if m["status"] != "refreshing" {
		t.Errorf("status = %q, want refreshing", m["status"])
	}
}

// --------------- Reorder ---------------

func TestReorderCategories(t *testing.T) {
	s := newTestServer(t)
	q := dbgen.New(s.DB)
	ctx := context.Background()
	email := "test@example.com"
	now := time.Now()
	_ = q.UpsertUser(ctx, dbgen.UpsertUserParams{
		ID: "testuser", Email: &email, CreatedAt: now, LastSeen: now,
	})
	c1, _ := q.CreateCategory(ctx, dbgen.CreateCategoryParams{UserID: "testuser", Title: "A"})
	c2, _ := q.CreateCategory(ctx, dbgen.CreateCategoryParams{UserID: "testuser", Title: "B"})

	body := fmt.Sprintf(`[{"id":%d,"order":2},{"id":%d,"order":1}]`, c1.ID, c2.ID)
	w := httptest.NewRecorder()
	s.HandleReorderCategories(w, authReq("POST", "/api/categories/reorder", body))
	assertStatus(t, w, 200)
}

func TestReorderCategoriesBadJSON(t *testing.T) {
	s := newTestServer(t)
	w := httptest.NewRecorder()
	s.HandleReorderCategories(w, authReq("POST", "/api/categories/reorder", "not json"))
	assertStatus(t, w, 400)
}

func TestReorderFeeds(t *testing.T) {
	s := newTestServer(t)
	f := seedFeed(t, s, "rf", nil, 0)
	body := fmt.Sprintf(`[{"id":%d,"order":1}]`, f.ID)
	w := httptest.NewRecorder()
	s.HandleReorderFeeds(w, authReq("POST", "/api/feeds/reorder", body))
	assertStatus(t, w, 200)
}

func TestReorderFeedsBadJSON(t *testing.T) {
	s := newTestServer(t)
	w := httptest.NewRecorder()
	s.HandleReorderFeeds(w, authReq("POST", "/api/feeds/reorder", "bad"))
	assertStatus(t, w, 400)
}

// --------------- OPML ---------------

func TestOPMLParseAndGenerate(t *testing.T) {
	opmlXML := `<?xml version="1.0" encoding="UTF-8"?>
<opml version="2.0">
  <head><title>Test</title></head>
  <body>
    <outline text="Tech" title="Tech">
      <outline type="rss" text="Go Blog" xmlUrl="https://go.dev/blog/feed.atom" htmlUrl="https://go.dev/blog"/>
    </outline>
    <outline type="rss" text="HN" xmlUrl="https://hnrss.org/newest"/>
  </body>
</opml>`

	t.Run("parse", func(t *testing.T) {
		feeds, err := ParseOPML(strings.NewReader(opmlXML))
		if err != nil {
			t.Fatalf("ParseOPML: %v", err)
		}
		if len(feeds) != 2 {
			t.Fatalf("got %d feeds, want 2", len(feeds))
		}
		if feeds[0].Category != "Tech" {
			t.Errorf("feed[0].Category = %q, want Tech", feeds[0].Category)
		}
		if feeds[0].URL != "https://go.dev/blog/feed.atom" {
			t.Errorf("feed[0].URL = %q", feeds[0].URL)
		}
		if feeds[1].Category != "" {
			t.Errorf("feed[1].Category = %q, want empty", feeds[1].Category)
		}
	})

	t.Run("parse invalid xml", func(t *testing.T) {
		_, err := ParseOPML(strings.NewReader("not xml"))
		if err == nil {
			t.Error("expected error for invalid XML")
		}
	})

	t.Run("generate", func(t *testing.T) {
		exports := []FeedExport{
			{URL: "https://go.dev/feed", Title: "Go Blog", SiteURL: "https://go.dev", Category: "Tech"},
			{URL: "https://hn.com/rss", Title: "HN", SiteURL: "https://hn.com", Category: ""},
		}
		data, err := GenerateOPML("Test Export", exports)
		if err != nil {
			t.Fatalf("GenerateOPML: %v", err)
		}
		xml := string(data)
		if !strings.Contains(xml, "go.dev/feed") {
			t.Error("missing go.dev/feed in output")
		}
		if !strings.Contains(xml, "Tech") {
			t.Error("missing Tech category")
		}
		if !strings.Contains(xml, "hn.com/rss") {
			t.Error("missing hn.com/rss")
		}
	})

	t.Run("generate empty", func(t *testing.T) {
		data, err := GenerateOPML("Empty", nil)
		if err != nil {
			t.Fatalf("GenerateOPML: %v", err)
		}
		if !strings.Contains(string(data), "Empty") {
			t.Error("missing title")
		}
	})

	t.Run("roundtrip", func(t *testing.T) {
		exports := []FeedExport{
			{URL: "https://a.com/rss", Title: "A", Category: "Cat1"},
			{URL: "https://b.com/rss", Title: "B", Category: "Cat1"},
			{URL: "https://c.com/rss", Title: "C", Category: ""},
		}
		data, _ := GenerateOPML("RT", exports)
		parsed, err := ParseOPML(strings.NewReader(string(data)))
		if err != nil {
			t.Fatalf("roundtrip parse: %v", err)
		}
		if len(parsed) != 3 {
			t.Errorf("roundtrip got %d feeds, want 3", len(parsed))
		}
	})
}

func TestExportOPML(t *testing.T) {
	s := newTestServer(t)
	seedFeed(t, s, "export-feed", nil, 1)
	w := httptest.NewRecorder()
	s.HandleExportOPML(w, authReq("GET", "/api/opml/export", ""))
	assertStatus(t, w, 200)
	if ct := w.Header().Get("Content-Type"); ct != "application/xml" {
		t.Errorf("Content-Type = %q", ct)
	}
	if !strings.Contains(w.Body.String(), "export-feed") {
		t.Error("OPML should contain feed title")
	}
}

// --------------- Auth / Sessions ---------------

func TestAuthSessions(t *testing.T) {
	t.Run("create and validate", func(t *testing.T) {
		sid := createSession()
		if sid == "" {
			t.Fatal("empty session id")
		}
		if !validateSession(sid) {
			t.Error("session should be valid")
		}
	})

	t.Run("invalid session", func(t *testing.T) {
		if validateSession("nonexistent") {
			t.Error("nonexistent session should be invalid")
		}
	})

	t.Run("delete session", func(t *testing.T) {
		sid := createSession()
		deleteSession(sid)
		if validateSession(sid) {
			t.Error("deleted session should be invalid")
		}
	})

	t.Run("cleanup expired", func(t *testing.T) {
		// Manually insert an expired session
		sessionsLock.Lock()
		sessions["expired-test"] = time.Now().Add(-time.Hour)
		sessionsLock.Unlock()

		cleanupSessions()
		if validateSession("expired-test") {
			t.Error("expired session should be cleaned up")
		}
	})
}

func TestAuthMode(t *testing.T) {
	t.Run("default none", func(t *testing.T) {
		t.Setenv("GORSS_AUTH_MODE", "")
		if GetAuthMode() != AuthModeNone {
			t.Errorf("got %v, want none", GetAuthMode())
		}
	})
	t.Run("password", func(t *testing.T) {
		t.Setenv("GORSS_AUTH_MODE", "password")
		if GetAuthMode() != AuthModePassword {
			t.Errorf("got %v", GetAuthMode())
		}
	})
	t.Run("proxy", func(t *testing.T) {
		t.Setenv("GORSS_AUTH_MODE", "proxy")
		if GetAuthMode() != AuthModeProxy {
			t.Errorf("got %v", GetAuthMode())
		}
	})
}

func TestLoginLogout(t *testing.T) {
	s := newTestServer(t)
	t.Setenv("GORSS_AUTH_MODE", "password")
	t.Setenv("GORSS_PASSWORD", "secret123")

	t.Run("login page GET", func(t *testing.T) {
		w := httptest.NewRecorder()
		s.HandleLogin(w, httptest.NewRequest("GET", "/login", nil))
		assertStatus(t, w, 200)
		if !strings.Contains(w.Body.String(), "password") {
			t.Error("login page should contain password field")
		}
	})

	t.Run("login wrong password", func(t *testing.T) {
		r := httptest.NewRequest("POST", "/login", strings.NewReader("password=wrong"))
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		w := httptest.NewRecorder()
		s.HandleLogin(w, r)
		assertStatus(t, w, 200) // re-renders login page
		if !strings.Contains(w.Body.String(), "Invalid") {
			t.Error("should show invalid password error")
		}
	})

	t.Run("login correct password", func(t *testing.T) {
		r := httptest.NewRequest("POST", "/login", strings.NewReader("password=secret123"))
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		w := httptest.NewRecorder()
		s.HandleLogin(w, r)
		if w.Code != http.StatusFound {
			t.Errorf("expected 302 redirect, got %d", w.Code)
		}
		cookie := w.Header().Get("Set-Cookie")
		if !strings.Contains(cookie, "gorss_session") {
			t.Error("expected session cookie")
		}
	})

	t.Run("logout", func(t *testing.T) {
		r := httptest.NewRequest("POST", "/logout", nil)
		r.AddCookie(&http.Cookie{Name: "gorss_session", Value: createSession()})
		w := httptest.NewRecorder()
		s.HandleLogout(w, r)
		if w.Code != http.StatusFound {
			t.Errorf("expected 302, got %d", w.Code)
		}
	})
}
