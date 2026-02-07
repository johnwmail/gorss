package srv

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/mmcdole/gofeed"
	"srv.exe.dev/db/dbgen"
)

// FeedFetcher handles RSS/Atom feed fetching and parsing
type FeedFetcher struct {
	parser *gofeed.Parser
}

func NewFeedFetcher() *FeedFetcher {
	return &FeedFetcher{
		parser: gofeed.NewParser(),
	}
}

// FetchResult contains the parsed feed data
type FetchResult struct {
	Title       string
	SiteURL     string
	Description string
	Items       []FeedItem
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

// Fetch fetches and parses a feed URL
func (f *FeedFetcher) Fetch(ctx context.Context, url string) (*FetchResult, error) {
	feed, err := f.parser.ParseURLWithContext(url, ctx)
	if err != nil {
		return nil, fmt.Errorf("parse feed: %w", err)
	}

	result := &FetchResult{
		Title:       feed.Title,
		SiteURL:     feed.Link,
		Description: feed.Description,
		Items:       make([]FeedItem, 0, len(feed.Items)),
	}

	for _, item := range feed.Items {
		fi := FeedItem{
			GUID:    item.GUID,
			URL:     item.Link,
			Title:   item.Title,
			Content: item.Content,
			Summary: item.Description,
		}

		// Use GUID or URL as fallback
		if fi.GUID == "" {
			fi.GUID = item.Link
		}

		// Get author
		if item.Author != nil {
			fi.Author = item.Author.Name
		}

		// Get published date
		if item.PublishedParsed != nil {
			fi.PublishedAt = item.PublishedParsed
		} else if item.UpdatedParsed != nil {
			fi.PublishedAt = item.UpdatedParsed
		}

		result.Items = append(result.Items, fi)
	}

	return result, nil
}

// RefreshFeed fetches a feed and stores new articles
func (s *Server) RefreshFeed(ctx context.Context, feedID int64) error {
	q := dbgen.New(s.DB)

	// Get feed info
	feeds, err := q.GetAllFeedsForRefresh(ctx, 1)
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
	result, err := s.fetcher.Fetch(ctx, feed.Url)
	now := time.Now()

	if err != nil {
		errStr := err.Error()
		q.UpdateFeedMeta(ctx, dbgen.UpdateFeedMetaParams{
			ID:          feed.ID,
			Title:       feed.Title,
			SiteUrl:     feed.SiteUrl,
			Description: feed.Description,
			LastUpdated: &now,
			LastError:   &errStr,
		})
		return fmt.Errorf("fetch feed %s: %w", feed.Url, err)
	}

	// Update feed metadata
	title := result.Title
	if title == "" {
		title = feed.Title
	}
	err = q.UpdateFeedMeta(ctx, dbgen.UpdateFeedMetaParams{
		ID:          feed.ID,
		Title:       title,
		SiteUrl:     result.SiteURL,
		Description: result.Description,
		LastUpdated: &now,
		LastError:   nil,
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
	feeds, err := q.GetAllFeedsForRefresh(ctx, 100)
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
