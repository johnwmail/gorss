package srv

import (
	"context"
	"database/sql"
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"srv.exe.dev/db/dbgen"
)

// JSON response helper
func jsonResponse(w http.ResponseWriter, data any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(data)
}

func jsonError(w http.ResponseWriter, msg string, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

// getUserID extracts user ID from exe.dev headers
func getUserID(r *http.Request) string {
	return strings.TrimSpace(r.Header.Get("X-ExeDev-UserID"))
}

func getUserEmail(r *http.Request) string {
	return strings.TrimSpace(r.Header.Get("X-ExeDev-Email"))
}

// ensureUser creates or updates user record
func (s *Server) ensureUser(r *http.Request) (string, error) {
	userID := getUserID(r)
	if userID == "" {
		userID = "anonymous"
	}

	email := getUserEmail(r)
	now := time.Now()

	q := dbgen.New(s.DB)
	err := q.UpsertUser(r.Context(), dbgen.UpsertUserParams{
		ID:        userID,
		Email:     &email,
		CreatedAt: now,
		LastSeen:  now,
	})
	return userID, err
}

// HandleHealth returns health status
func (s *Server) HandleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

// HandleGetFeeds returns all feeds for the user
func (s *Server) HandleGetFeeds(w http.ResponseWriter, r *http.Request) {
	userID, err := s.ensureUser(r)
	if err != nil {
		slog.Warn("ensure user", "error", err)
	}

	q := dbgen.New(s.DB)
	feeds, err := q.GetFeedsOrdered(r.Context(), userID)
	if err != nil {
		jsonError(w, "failed to get feeds", http.StatusInternalServerError)
		return
	}
	if feeds == nil {
		feeds = []dbgen.Feed{}
	}
	jsonResponse(w, feeds)
}

// HandleSubscribe subscribes to a new feed
func (s *Server) HandleSubscribe(w http.ResponseWriter, r *http.Request) {
	userID, err := s.ensureUser(r)
	if err != nil {
		slog.Warn("ensure user", "error", err)
	}

	var req struct {
		URL        string `json:"url"`
		CategoryID *int64 `json:"category_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid request body", http.StatusBadRequest)
		return
	}

	if req.URL == "" {
		jsonError(w, "url is required", http.StatusBadRequest)
		return
	}

	// Fetch feed to get title
	result, err := s.fetcher.Fetch(r.Context(), req.URL)
	if err != nil {
		jsonError(w, "failed to fetch feed: "+err.Error(), http.StatusBadRequest)
		return
	}

	q := dbgen.New(s.DB)
	feed, err := q.CreateFeed(r.Context(), dbgen.CreateFeedParams{
		UserID:      userID,
		CategoryID:  req.CategoryID,
		Url:         req.URL,
		Title:       result.Title,
		SiteUrl:     result.SiteURL,
		Description: result.Description,
	})
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			jsonError(w, "already subscribed to this feed", http.StatusConflict)
			return
		}
		jsonError(w, "failed to create feed", http.StatusInternalServerError)
		return
	}

	// Store initial articles
	for _, item := range result.Items {
		_, _ = q.UpsertArticle(r.Context(), dbgen.UpsertArticleParams{
				FeedID:      feed.ID,
				Guid:        item.GUID,
				Url:         item.URL,
				Title:       item.Title,
				Author:      item.Author,
				Content:     item.Content,
				Summary:     item.Summary,
				PublishedAt: item.PublishedAt,
			})
	}

	jsonResponse(w, feed)
}

// HandleUnsubscribe removes a feed subscription
func (s *Server) HandleUnsubscribe(w http.ResponseWriter, r *http.Request) {
	userID, _ := s.ensureUser(r)
	feedID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		jsonError(w, "invalid feed id", http.StatusBadRequest)
		return
	}

	q := dbgen.New(s.DB)
	if err := q.DeleteFeed(r.Context(), dbgen.DeleteFeedParams{
		ID:     feedID,
		UserID: userID,
	}); err != nil {
		jsonError(w, "failed to delete feed", http.StatusInternalServerError)
		return
	}
	jsonResponse(w, map[string]string{"status": "ok"})
}

// queryArticlesByCategory runs a category-filtered article query.
// categoryID=0 means uncategorized (NULL category_id).
// unreadOnly=true filters to unread articles only.
func queryArticlesByCategory(ctx context.Context, db *sql.DB, userID string, categoryID int64, unreadOnly bool, limit, offset int64) ([]dbgen.GetArticlesRow, error) {
	var catFilter string
	var args []any

	if categoryID == 0 {
		catFilter = "f.category_id IS NULL"
		args = append(args, userID) // for s.user_id
		// no category arg
		args = append(args, userID) // for f.user_id
	} else {
		catFilter = "f.category_id = ?"
		args = append(args, userID, categoryID, userID)
	}

	unreadFilter := ""
	if unreadOnly {
		unreadFilter = " AND (s.is_read IS NULL OR s.is_read = 0)"
	}

	query := `
SELECT a.id, a.feed_id, a.guid, a.url, a.title, a.author, a.content, a.summary, a.published_at, a.created_at,
  f.title as feed_title, f.site_url as feed_site_url,
  COALESCE(s.is_read, 0) as is_read,
  COALESCE(s.is_starred, 0) as is_starred
FROM articles a
JOIN feeds f ON a.feed_id = f.id
LEFT JOIN article_states s ON s.article_id = a.id AND s.user_id = ?
WHERE ` + catFilter + ` AND f.user_id = ?` + unreadFilter + `
ORDER BY a.published_at DESC
LIMIT ? OFFSET ?`

	args = append(args, limit, offset)

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var articles []dbgen.GetArticlesRow
	for rows.Next() {
		var a dbgen.GetArticlesRow
		if err := rows.Scan(
			&a.ID, &a.FeedID, &a.Guid, &a.Url, &a.Title, &a.Author,
			&a.Content, &a.Summary, &a.PublishedAt, &a.CreatedAt,
			&a.FeedTitle, &a.FeedSiteUrl, &a.IsRead, &a.IsStarred,
		); err != nil {
			return nil, err
		}
		articles = append(articles, a)
	}
	if articles == nil {
		articles = []dbgen.GetArticlesRow{}
	}
	return articles, rows.Err()
}

// HandleGetArticles returns articles with optional filters
// parsePagination extracts limit and offset from query params.
func parsePagination(r *http.Request) (limit, offset int64) {
	limit = 50
	if l := r.URL.Query().Get("limit"); l != "" {
		if parsed, err := strconv.ParseInt(l, 10, 64); err == nil {
			limit = parsed
		}
	}
	if o := r.URL.Query().Get("offset"); o != "" {
		if parsed, err := strconv.ParseInt(o, 10, 64); err == nil {
			offset = parsed
		}
	}
	return
}

// fetchArticles dispatches the correct query based on view/feed/category filters.
func (s *Server) fetchArticles(r *http.Request, userID, view, feedID, categoryID string, limit, offset int64) ([]dbgen.GetArticlesRow, error) {
	q := dbgen.New(s.DB)

	switch {
	case view == "starred":
		starred, err := q.GetStarredArticles(r.Context(), dbgen.GetStarredArticlesParams{
			UserID: userID, UserID_2: userID, Limit: limit, Offset: offset,
		})
		if err != nil {
			return nil, err
		}
		out := make([]dbgen.GetArticlesRow, len(starred))
		for i, a := range starred {
			out[i] = dbgen.GetArticlesRow(a)
		}
		return out, nil

	case categoryID != "" && (view == "unread" || view == "fresh"):
		cid, _ := strconv.ParseInt(categoryID, 10, 64)
		return queryArticlesByCategory(r.Context(), s.DB, userID, cid, true, limit, offset)

	case view == "unread" || view == "fresh":
		unread, err := q.GetUnreadArticles(r.Context(), dbgen.GetUnreadArticlesParams{
			UserID: userID, UserID_2: userID, Limit: limit, Offset: offset,
		})
		if err != nil {
			return nil, err
		}
		out := make([]dbgen.GetArticlesRow, len(unread))
		for i, a := range unread {
			out[i] = dbgen.GetArticlesRow(a)
		}
		return out, nil

	case categoryID != "":
		cid, _ := strconv.ParseInt(categoryID, 10, 64)
		return queryArticlesByCategory(r.Context(), s.DB, userID, cid, false, limit, offset)

	case feedID != "":
		fid, _ := strconv.ParseInt(feedID, 10, 64)
		byFeed, err := q.GetArticlesByFeed(r.Context(), dbgen.GetArticlesByFeedParams{
			UserID: userID, ID: fid, UserID_2: userID, Limit: limit, Offset: offset,
		})
		if err != nil {
			return nil, err
		}
		out := make([]dbgen.GetArticlesRow, len(byFeed))
		for i, a := range byFeed {
			out[i] = dbgen.GetArticlesRow(a)
		}
		return out, nil

	default:
		return q.GetArticles(r.Context(), dbgen.GetArticlesParams{
			UserID: userID, UserID_2: userID, Limit: limit, Offset: offset,
		})
	}
}

// HandleGetArticles returns articles with optional filters
func (s *Server) HandleGetArticles(w http.ResponseWriter, r *http.Request) {
	userID, _ := s.ensureUser(r)
	limit, offset := parsePagination(r)

	query := r.URL.Query()
	articles, err := s.fetchArticles(r, userID, query.Get("view"), query.Get("feed_id"), query.Get("category_id"), limit, offset)
	if err != nil {
		slog.Error("get articles", "error", err)
		jsonError(w, "failed to get articles", http.StatusInternalServerError)
		return
	}

	if articles == nil {
		articles = []dbgen.GetArticlesRow{}
	}
	jsonResponse(w, articles)
}

// HandleMarkRead marks an article as read
func (s *Server) HandleMarkRead(w http.ResponseWriter, r *http.Request) {
	userID, _ := s.ensureUser(r)
	articleID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		jsonError(w, "invalid article id", http.StatusBadRequest)
		return
	}

	q := dbgen.New(s.DB)
	now := time.Now()
	if err := q.SetArticleRead(r.Context(), dbgen.SetArticleReadParams{
		UserID:    userID,
		ArticleID: articleID,
		ReadAt:    &now,
	}); err != nil {
		jsonError(w, "failed to mark read", http.StatusInternalServerError)
		return
	}
	jsonResponse(w, map[string]string{"status": "ok"})
}

// HandleMarkUnread marks an article as unread
func (s *Server) HandleMarkUnread(w http.ResponseWriter, r *http.Request) {
	userID, _ := s.ensureUser(r)
	articleID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		jsonError(w, "invalid article id", http.StatusBadRequest)
		return
	}

	q := dbgen.New(s.DB)
	if err := q.SetArticleUnread(r.Context(), dbgen.SetArticleUnreadParams{
		UserID:    userID,
		ArticleID: articleID,
	}); err != nil {
		jsonError(w, "failed to mark unread", http.StatusInternalServerError)
		return
	}
	jsonResponse(w, map[string]string{"status": "ok"})
}

// HandleStar stars an article
func (s *Server) HandleStar(w http.ResponseWriter, r *http.Request) {
	userID, _ := s.ensureUser(r)
	articleID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		jsonError(w, "invalid article id", http.StatusBadRequest)
		return
	}

	q := dbgen.New(s.DB)
	now := time.Now()
	if err := q.SetArticleStarred(r.Context(), dbgen.SetArticleStarredParams{
		UserID:    userID,
		ArticleID: articleID,
		StarredAt: &now,
	}); err != nil {
		jsonError(w, "failed to star", http.StatusInternalServerError)
		return
	}
	jsonResponse(w, map[string]string{"status": "ok"})
}

// HandleUnstar unstars an article
func (s *Server) HandleUnstar(w http.ResponseWriter, r *http.Request) {
	userID, _ := s.ensureUser(r)
	articleID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		jsonError(w, "invalid article id", http.StatusBadRequest)
		return
	}

	q := dbgen.New(s.DB)
	if err := q.SetArticleUnstarred(r.Context(), dbgen.SetArticleUnstarredParams{
		UserID:    userID,
		ArticleID: articleID,
	}); err != nil {
		jsonError(w, "failed to unstar", http.StatusInternalServerError)
		return
	}
	jsonResponse(w, map[string]string{"status": "ok"})
}

// markCategoryRead marks all articles in a category as read.
func (s *Server) markCategoryRead(ctx context.Context, userID string, categoryID int64) error {
	now := time.Now()
	var catFilter string
	var args []any
	if categoryID == 0 {
		catFilter = "f.category_id IS NULL"
		args = []any{userID, now, userID}
	} else {
		catFilter = "f.category_id = ?"
		args = []any{userID, now, categoryID, userID}
	}
	query := `INSERT OR REPLACE INTO article_states (user_id, article_id, is_read, read_at, is_starred, starred_at)
		SELECT ?, a.id, 1, ?,
			COALESCE(s.is_starred, 0), s.starred_at
		FROM articles a
		JOIN feeds f ON a.feed_id = f.id
		LEFT JOIN article_states s ON s.article_id = a.id AND s.user_id = f.user_id
		WHERE ` + catFilter + ` AND f.user_id = ?`
	_, err := s.DB.ExecContext(ctx, query, args...)
	return err
}

// HandleMarkAllRead marks all articles as read (optionally filtered by feed or category)
func (s *Server) HandleMarkAllRead(w http.ResponseWriter, r *http.Request) {
	userID, _ := s.ensureUser(r)
	q := dbgen.New(s.DB)
	now := time.Now()

	switch {
	case r.URL.Query().Get("feed_id") != "":
		feedID, err := strconv.ParseInt(r.URL.Query().Get("feed_id"), 10, 64)
		if err != nil {
			jsonError(w, "invalid feed_id", http.StatusBadRequest)
			return
		}
		if err := q.MarkFeedRead(r.Context(), dbgen.MarkFeedReadParams{
			UserID: userID, ReadAt: &now, FeedID: feedID,
		}); err != nil {
			jsonError(w, "failed to mark feed read", http.StatusInternalServerError)
			return
		}
	case r.URL.Query().Get("category_id") != "":
		catID, err := strconv.ParseInt(r.URL.Query().Get("category_id"), 10, 64)
		if err != nil {
			jsonError(w, "invalid category_id", http.StatusBadRequest)
			return
		}
		if err := s.markCategoryRead(r.Context(), userID, catID); err != nil {
			jsonError(w, "failed to mark category read", http.StatusInternalServerError)
			return
		}
	default:
		if err := q.MarkAllRead(r.Context(), dbgen.MarkAllReadParams{
			UserID: userID, ReadAt: &now, UserID_2: userID,
		}); err != nil {
			jsonError(w, "failed to mark all read", http.StatusInternalServerError)
			return
		}
	}
	jsonResponse(w, map[string]string{"status": "ok"})
}

// HandleMarkFeedRead marks all articles in a feed as read
func (s *Server) HandleMarkFeedRead(w http.ResponseWriter, r *http.Request) {
	userID, _ := s.ensureUser(r)
	feedID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		jsonError(w, "invalid feed id", http.StatusBadRequest)
		return
	}

	q := dbgen.New(s.DB)
	now := time.Now()
	if err := q.MarkFeedRead(r.Context(), dbgen.MarkFeedReadParams{
		UserID: userID,
		ReadAt: &now,
		FeedID: feedID,
	}); err != nil {
		jsonError(w, "failed to mark feed read", http.StatusInternalServerError)
		return
	}
	jsonResponse(w, map[string]string{"status": "ok"})
}

// HandleRefresh triggers a feed refresh
func (s *Server) HandleRefresh(w http.ResponseWriter, r *http.Request) {
	go s.refreshAllFeeds(r.Context())
	jsonResponse(w, map[string]string{"status": "refreshing"})
}

// HandleGetCategories returns all categories
func (s *Server) HandleGetCategories(w http.ResponseWriter, r *http.Request) {
	userID, _ := s.ensureUser(r)
	q := dbgen.New(s.DB)

	categories, err := q.GetCategoriesOrdered(r.Context(), userID)
	if err != nil {
		jsonError(w, "failed to get categories", http.StatusInternalServerError)
		return
	}
	if categories == nil {
		categories = []dbgen.Category{}
	}
	jsonResponse(w, categories)
}

// HandleCreateCategory creates a new category
func (s *Server) HandleCreateCategory(w http.ResponseWriter, r *http.Request) {
	userID, _ := s.ensureUser(r)

	var req struct {
		Title string `json:"title"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid request body", http.StatusBadRequest)
		return
	}

	if req.Title == "" {
		jsonError(w, "title is required", http.StatusBadRequest)
		return
	}

	q := dbgen.New(s.DB)
	cat, err := q.CreateCategory(r.Context(), dbgen.CreateCategoryParams{
		UserID: userID,
		Title:  req.Title,
	})
	if err != nil {
		jsonError(w, "failed to create category", http.StatusInternalServerError)
		return
	}
	jsonResponse(w, cat)
}

// HandleGetCounts returns unread and starred counts, plus per-feed counts
func (s *Server) HandleGetCounts(w http.ResponseWriter, r *http.Request) {
	userID, _ := s.ensureUser(r)
	q := dbgen.New(s.DB)

	total, _ := q.GetTotalArticleCount(r.Context(), userID)
	unread, _ := q.GetUnreadCount(r.Context(), userID)
	starred, _ := q.GetStarredCount(r.Context(), userID)

	// Get per-feed unread counts
	feeds, _ := q.GetFeeds(r.Context(), userID)
	feedCounts := make(map[int64]int64)
	for _, f := range feeds {
		if f.UnreadCount > 0 {
			feedCounts[f.ID] = f.UnreadCount
		}
	}

	jsonResponse(w, map[string]interface{}{
		"total":   total,
		"unread":  unread,
		"starred": starred,
		"feeds":   feedCounts,
	})
}

// HandleExportOPML exports feeds as OPML
func (s *Server) HandleExportOPML(w http.ResponseWriter, r *http.Request) {
	userID, _ := s.ensureUser(r)
	q := dbgen.New(s.DB)

	feeds, err := q.GetFeeds(r.Context(), userID)
	if err != nil {
		http.Error(w, "failed to list feeds", http.StatusInternalServerError)
		return
	}

	// Get categories for mapping
	categories, _ := q.GetCategories(r.Context(), userID)
	catMap := make(map[int64]string)
	for _, c := range categories {
		catMap[c.ID] = c.Title
	}

	// Build export list
	var exports []FeedExport
	for _, f := range feeds {
		cat := ""
		if f.CategoryID != nil {
			cat = catMap[*f.CategoryID]
		}
		exports = append(exports, FeedExport{
			URL:      f.Url,
			Title:    f.Title,
			SiteURL:  stringVal(f.SiteUrl),
			Category: cat,
		})
	}

	opml, err := GenerateOPML("GoRSS Export", exports)
	if err != nil {
		http.Error(w, "failed to generate OPML", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/xml")
	w.Header().Set("Content-Disposition", "attachment; filename=gorss-feeds.opml")
	_, _ = w.Write(opml)
}

func stringVal(s string) string {
	return s
}

// HandleImportOPML imports feeds from OPML
// resolveCategoryMap builds a mapping of category name → ID, creating categories as needed.
func (s *Server) resolveCategoryMap(ctx context.Context, userID string, feeds []FeedImport) map[string]int64 {
	q := dbgen.New(s.DB)
	catMap := make(map[string]int64)
	for _, f := range feeds {
		if f.Category == "" || catMap[f.Category] != 0 {
			continue
		}
		cats, _ := q.GetCategories(ctx, userID)
		for _, c := range cats {
			if c.Title == f.Category {
				catMap[f.Category] = c.ID
				break
			}
		}
		if catMap[f.Category] == 0 {
			cat, err := q.CreateCategory(ctx, dbgen.CreateCategoryParams{UserID: userID, Title: f.Category})
			if err == nil {
				catMap[f.Category] = cat.ID
			}
		}
	}
	return catMap
}

// importSingleFeed fetches, creates and stores articles for one feed. Returns true if imported.
func (s *Server) importSingleFeed(ctx context.Context, userID string, f FeedImport, catMap map[string]int64) bool {
	q := dbgen.New(s.DB)

	// Check if already subscribed
	existing, _ := q.GetFeeds(ctx, userID)
	for _, e := range existing {
		if e.Url == f.URL {
			return false
		}
	}

	var catID *int64
	if f.Category != "" && catMap[f.Category] != 0 {
		id := catMap[f.Category]
		catID = &id
	}

	result, err := s.fetcher.Fetch(ctx, f.URL)
	if err != nil {
		slog.Warn("import feed fetch failed", "url", f.URL, "error", err)
		return false
	}

	feed, err := q.CreateFeed(ctx, dbgen.CreateFeedParams{
		UserID: userID, CategoryID: catID, Url: f.URL,
		Title: result.Title, SiteUrl: result.SiteURL, Description: result.Description,
	})
	if err != nil {
		slog.Warn("import feed create failed", "url", f.URL, "error", err)
		return false
	}

	for _, item := range result.Items {
		_, _ = q.UpsertArticle(ctx, dbgen.UpsertArticleParams{
			FeedID: feed.ID, Guid: item.GUID, Url: item.URL, Title: item.Title,
			Content: item.Content, Author: item.Author, PublishedAt: item.PublishedAt,
		})
	}
	return true
}

// HandleImportOPML imports feeds from OPML
func (s *Server) HandleImportOPML(w http.ResponseWriter, r *http.Request) {
	userID, _ := s.ensureUser(r)

	if err := r.ParseMultipartForm(10 << 20); err != nil {
		jsonError(w, "failed to parse form", http.StatusBadRequest)
		return
	}

	file, _, err := r.FormFile("file")
	if err != nil {
		jsonError(w, "no file provided", http.StatusBadRequest)
		return
	}
	defer func() { _ = file.Close() }()

	feeds, err := ParseOPML(file)
	if err != nil {
		jsonError(w, "failed to parse OPML: "+err.Error(), http.StatusBadRequest)
		return
	}

	catMap := s.resolveCategoryMap(r.Context(), userID, feeds)

	imported := 0
	for _, f := range feeds {
		if s.importSingleFeed(r.Context(), userID, f, catMap) {
			imported++
		}
	}

	jsonResponse(w, map[string]int{
		"imported": imported,
		"skipped":  len(feeds) - imported,
		"total":    len(feeds),
	})
}

// HandleReorderCategories updates category sort orders
func (s *Server) HandleReorderCategories(w http.ResponseWriter, r *http.Request) {
	userID, _ := s.ensureUser(r)
	q := dbgen.New(s.DB)

	var req []struct {
		ID    int64 `json:"id"`
		Order int64 `json:"order"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid request", http.StatusBadRequest)
		return
	}

	for _, item := range req {
		_ = q.UpdateCategorySortOrder(r.Context(), dbgen.UpdateCategorySortOrderParams{
			SortOrder: item.Order,
			ID:        item.ID,
			UserID:    userID,
		})
	}

	jsonResponse(w, map[string]string{"status": "ok"})
}

// HandleReorderFeeds updates feed sort orders and optionally moves feeds between categories
func (s *Server) HandleReorderFeeds(w http.ResponseWriter, r *http.Request) {
	userID, _ := s.ensureUser(r)
	q := dbgen.New(s.DB)

	var req []struct {
		ID         int64  `json:"id"`
		Order      int64  `json:"order"`
		CategoryID *int64 `json:"category_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid request", http.StatusBadRequest)
		return
	}

	for _, item := range req {
		if item.CategoryID != nil {
			_ = q.UpdateFeedCategory(r.Context(), dbgen.UpdateFeedCategoryParams{
				CategoryID: item.CategoryID,
				SortOrder:  item.Order,
				ID:         item.ID,
				UserID:     userID,
			})
		} else {
			_ = q.UpdateFeedSortOrder(r.Context(), dbgen.UpdateFeedSortOrderParams{
				SortOrder: item.Order,
				ID:        item.ID,
				UserID:    userID,
			})
		}
	}

	jsonResponse(w, map[string]string{"status": "ok"})
}
