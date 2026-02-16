package srv

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/mmcdole/gofeed"
	"github.com/johnwmail/gorss/db"
	"github.com/johnwmail/gorss/db/dbgen"
)

// FeedFetcher handles RSS/Atom feed fetching and parsing
type FeedFetcher struct {
	parser *gofeed.Parser
	client *http.Client
}

func NewFeedFetcher() *FeedFetcher {
	return &FeedFetcher{
		parser: gofeed.NewParser(),
		client: &http.Client{Timeout: 30 * time.Second},
	}
}

// FeedFetchResult contains the parsed feed data
type FeedFetchResult struct {
	Title       string
	SiteURL     string
	Description string
	Items       []FeedItem
	// HTTP caching headers from the response
	ETag         string
	LastModified string
}

type FeedItem struct {
	GUID        string
	URL         string
	Title       string
	Author      string
	Content     string
	Summary     string
	PublishedAt *time.Time
}

// errNotModified is returned when the server responds with 304 Not Modified.
var errNotModified = fmt.Errorf("feed not modified")

// Fetch fetches and parses a feed URL (unconditional GET, used for initial subscribe).
func (f *FeedFetcher) Fetch(ctx context.Context, url string) (*FeedFetchResult, error) {
	return f.fetchWithCaching(ctx, url, "", "")
}

// FetchConditional does a conditional GET using saved ETag/Last-Modified.
// Returns errNotModified if the server says nothing changed.
func (f *FeedFetcher) FetchConditional(ctx context.Context, url, etag, lastModified string) (*FeedFetchResult, error) {
	return f.fetchWithCaching(ctx, url, etag, lastModified)
}

func (f *FeedFetcher) fetchWithCaching(ctx context.Context, url, etag, lastModified string) (*FeedFetchResult, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("User-Agent", "GoRSS/1.0 (feed reader)")

	// Conditional GET headers
	if etag != "" {
		req.Header.Set("If-None-Match", etag)
	}
	if lastModified != "" {
		req.Header.Set("If-Modified-Since", lastModified)
	}

	resp, err := f.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusNotModified {
		return nil, errNotModified
	}

	feed, err := f.parser.Parse(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("parse feed: %w", err)
	}

	result := &FeedFetchResult{
		Title:        feed.Title,
		SiteURL:      feed.Link,
		Description:  feed.Description,
		Items:        make([]FeedItem, 0, len(feed.Items)),
		ETag:         resp.Header.Get("ETag"),
		LastModified: resp.Header.Get("Last-Modified"),
	}

	for _, item := range feed.Items {
		fi := FeedItem{
			GUID:    item.GUID,
			URL:     item.Link,
			Title:   item.Title,
			Content: item.Content,
			Summary: item.Description,
		}

		if fi.GUID == "" {
			fi.GUID = item.Link
		}

		if item.Author != nil {
			fi.Author = item.Author.Name
		}

		if item.PublishedParsed != nil {
			fi.PublishedAt = item.PublishedParsed
		} else if item.UpdatedParsed != nil {
			fi.PublishedAt = item.UpdatedParsed
		}

		result.Items = append(result.Items, fi)
	}

	return result, nil
}

// filterOldItems removes items older than the cutoff from the result in place.
func filterOldItems(items []FeedItem, cutoff time.Time) []FeedItem {
	filtered := items[:0]
	for _, item := range items {
		// Keep items with no published date (can't determine age)
		if item.PublishedAt == nil || item.PublishedAt.After(cutoff) {
			filtered = append(filtered, item)
		}
	}
	return filtered
}

// maxBackoffHours caps exponential backoff at 24 hours.
const maxBackoffHours = 24

// shouldSkipFeed returns true if a feed with consecutive errors should be
// skipped this refresh cycle (exponential backoff).
func shouldSkipFeed(feed *dbgen.Feed) bool {
	if feed.ErrorCount == 0 || feed.LastUpdated == nil {
		return false
	}
	// Backoff: 2^errorCount hours, capped at 24h
	backoffHours := 1 << min(feed.ErrorCount, 5) // 2, 4, 8, 16, 32 → capped at 24
	if backoffHours > maxBackoffHours {
		backoffHours = maxBackoffHours
	}
	nextAllowed := feed.LastUpdated.Add(time.Duration(backoffHours) * time.Hour)
	return time.Now().Before(nextAllowed)
}

// RefreshFeed fetches a feed and stores new articles
func (s *Server) RefreshFeed(ctx context.Context, feedID int64) error {
	q := dbgen.New(s.DB)

	feeds, err := q.GetAllFeedsForRefresh(ctx, 1000)
	if err != nil {
		return fmt.Errorf("get feed: %w", err)
	}

	var feed *dbgen.Feed
	for _, f := range feeds {
		if f.ID == feedID {
			feed = &f
			break
		}
	}
	if feed == nil {
		return fmt.Errorf("feed not found: %d", feedID)
	}

	return s.refreshFeedInternal(ctx, q, feed)
}

func (s *Server) refreshFeedInternal(ctx context.Context, q *dbgen.Queries, feed *dbgen.Feed) error {
	// Skip feeds in error backoff
	if shouldSkipFeed(feed) {
		slog.Debug("skipping feed (backoff)", "feed_id", feed.ID, "error_count", feed.ErrorCount)
		return nil
	}

	// Use conditional GET with saved caching headers
	result, err := s.fetcher.FetchConditional(ctx, feed.Url, feed.Etag, feed.LastModified)
	now := time.Now()

	if err == errNotModified {
		slog.Debug("feed not modified (304)", "feed_id", feed.ID, "title", feed.Title)
		// Update last_updated timestamp, reset error count, keep caching headers
		_ = q.UpdateFeedMeta(ctx, dbgen.UpdateFeedMetaParams{
			ID:           feed.ID,
			Title:        feed.Title,
			SiteUrl:      feed.SiteUrl,
			Description:  feed.Description,
			LastUpdated:  &now,
			LastError:    nil,
			Etag:         feed.Etag,
			LastModified: feed.LastModified,
			ErrorCount:   0,
		})
		return nil
	}

	if err != nil {
		errStr := err.Error()
		_ = q.UpdateFeedMeta(ctx, dbgen.UpdateFeedMetaParams{
			ID:           feed.ID,
			Title:        feed.Title,
			SiteUrl:      feed.SiteUrl,
			Description:  feed.Description,
			LastUpdated:  &now,
			LastError:    &errStr,
			Etag:         feed.Etag,
			LastModified: feed.LastModified,
			ErrorCount:   feed.ErrorCount + 1,
		})
		return fmt.Errorf("fetch feed %s: %w", feed.Url, err)
	}

	// Filter out articles older than purge threshold
	if s.PurgeDays > 0 {
		cutoff := time.Now().AddDate(0, 0, -s.PurgeDays)
		beforeCount := len(result.Items)
		result.Items = filterOldItems(result.Items, cutoff)
		if skipped := beforeCount - len(result.Items); skipped > 0 {
			slog.Debug("filtered old articles", "feed_id", feed.ID, "skipped", skipped, "cutoff_days", s.PurgeDays)
		}
	}

	// Update feed metadata with new caching headers, reset error count
	title := result.Title
	if title == "" {
		title = feed.Title
	}
	err = q.UpdateFeedMeta(ctx, dbgen.UpdateFeedMetaParams{
		ID:           feed.ID,
		Title:        title,
		SiteUrl:      result.SiteURL,
		Description:  result.Description,
		LastUpdated:  &now,
		LastError:    nil,
		Etag:         result.ETag,
		LastModified: result.LastModified,
		ErrorCount:   0,
	})
	if err != nil {
		slog.Warn("update feed meta", "error", err, "feed_id", feed.ID)
	}

	// Insert articles
	for _, item := range result.Items {
		_, err := q.UpsertArticle(ctx, dbgen.UpsertArticleParams{
			FeedID:      feed.ID,
			Guid:        item.GUID,
			Url:         item.URL,
			Title:       item.Title,
			Author:      item.Author,
			Content:     item.Content,
			Summary:     item.Summary,
			PublishedAt: item.PublishedAt,
		})
		if err != nil {
			slog.Warn("upsert article", "error", err, "guid", item.GUID)
		}
	}

	slog.Info("refreshed feed", "feed_id", feed.ID, "title", title, "articles", len(result.Items))
	return nil
}

// StartBackgroundRefresh starts a goroutine that periodically refreshes all feeds
func (s *Server) StartBackgroundRefresh(ctx context.Context, interval time.Duration) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				slog.Info("stopping background feed refresh")
				return
			case <-ticker.C:
				s.refreshAllFeeds(ctx)
			}
		}
	}()
}

func (s *Server) refreshAllFeeds(ctx context.Context) {
	q := dbgen.New(s.DB)
	feeds, err := q.GetAllFeedsForRefresh(ctx, 1000)
	if err != nil {
		slog.Error("get feeds for refresh", "error", err)
		return
	}

	for _, feed := range feeds {
		if err := s.refreshFeedInternal(ctx, q, &feed); err != nil {
			slog.Warn("refresh feed", "error", err, "feed_id", feed.ID)
		}
		// Small delay between feeds to be nice to servers
		time.Sleep(time.Second)
	}
}

// StartAutoPurge starts a goroutine that periodically purges old read articles
func (s *Server) StartAutoPurge(ctx context.Context) {
	if s.PurgeDays <= 0 {
		return
	}
	go func() {
		// Run purge once at startup after a short delay
		time.Sleep(30 * time.Second)
		s.purgeOldArticles()

		// Then run daily
		ticker := time.NewTicker(24 * time.Hour)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				slog.Info("stopping auto-purge")
				return
			case <-ticker.C:
				s.purgeOldArticles()
			}
		}
	}()
}

func (s *Server) purgeOldArticles() {
	q := dbgen.New(s.DB)
	ctx := context.Background()

	cutoff := time.Now().AddDate(0, 0, -s.PurgeDays)

	count, err := q.CountOldReadArticles(ctx, &cutoff)
	if err != nil {
		slog.Error("count old articles", "error", err)
		return
	}

	if count == 0 {
		slog.Debug("no old read articles to purge")
		return
	}

	result, err := q.PurgeOldReadArticles(ctx, &cutoff)
	if err != nil {
		slog.Error("purge old articles", "error", err)
		return
	}

	deleted, _ := result.RowsAffected()
	slog.Info("purged old read articles", "count", deleted, "cutoff_days", s.PurgeDays)
}

// StartPeriodicBackup starts a goroutine that backs up the database periodically.
func (s *Server) StartPeriodicBackup(ctx context.Context, backupDir string, interval time.Duration, keep int) {
	// Run an initial backup shortly after startup
	go func() {
		select {
		case <-time.After(30 * time.Second):
		case <-ctx.Done():
			return
		}
		s.runBackup(backupDir, keep)

		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				s.runBackup(backupDir, keep)
			case <-ctx.Done():
				return
			}
		}
	}()
}

func (s *Server) runBackup(backupDir string, keep int) {
	path, err := db.Backup(s.DB, backupDir)
	if err != nil {
		slog.Error("database backup failed", "error", err)
		return
	}
	slog.Info("database backup complete", "path", path)

	if err := db.PruneBackups(backupDir, keep); err != nil {
		slog.Warn("backup pruning failed", "error", err)
	}
}
