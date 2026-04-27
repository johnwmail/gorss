package srv

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/johnwmail/gorss/db/dbgen"
)

func TestFeedFetcher_Fetch(t *testing.T) {
	// Test successful fetch
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/rss+xml")
		w.Header().Set("ETag", `"test-etag"`)
		w.Header().Set("Last-Modified", "Mon, 01 Jan 2024 00:00:00 GMT")
		fmt.Fprintf(w, `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0">
<channel>
<title>Test Feed</title>
<link>https://example.com</link>
<description>Test Description</description>
<item>
<guid>item-1</guid>
<link>https://example.com/1</link>
<title>Item 1</title>
<description>Summary 1</description>
<pubDate>Mon, 01 Jan 2024 00:00:00 GMT</pubDate>
</item>
</channel>
</rss>`)
	}))
	defer server.Close()

	fetcher := NewFeedFetcher()
	fetcher.AllowPrivateURLs = true
	result, err := fetcher.Fetch(context.Background(), server.URL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Title != "Test Feed" {
		t.Errorf("expected title 'Test Feed', got %q", result.Title)
	}
	if result.SiteURL != "https://example.com" {
		t.Errorf("expected site URL 'https://example.com', got %q", result.SiteURL)
	}
	if result.ETag != `"test-etag"` {
		t.Errorf("expected etag 'test-etag', got %q", result.ETag)
	}
	if len(result.Items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(result.Items))
	}
	if result.Items[0].GUID != "item-1" {
		t.Errorf("expected GUID 'item-1', got %q", result.Items[0].GUID)
	}
	if result.Items[0].Title != "Item 1" {
		t.Errorf("expected title 'Item 1', got %q", result.Items[0].Title)
	}
}

func TestFeedFetcher_FetchConditional_NotModified(t *testing.T) {
	// Test 304 Not Modified
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ifNoneMatch := r.Header.Get("If-None-Match")
		ifModifiedSince := r.Header.Get("If-Modified-Since")
		
		if ifNoneMatch == `"test-etag"` && ifModifiedSince == "Mon, 01 Jan 2024 00:00:00 GMT" {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	fetcher := NewFeedFetcher()
	fetcher.AllowPrivateURLs = true
	_, err := fetcher.FetchConditional(context.Background(), server.URL, `"test-etag"`, "Mon, 01 Jan 2024 00:00:00 GMT")
	if err != errNotModified {
		t.Fatalf("expected errNotModified, got %v", err)
	}
}

func TestFeedFetcher_Fetch_Error(t *testing.T) {
	// Test fetch error
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	fetcher := NewFeedFetcher()
	fetcher.AllowPrivateURLs = true
	_, err := fetcher.Fetch(context.Background(), server.URL)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestFilterOldItems(t *testing.T) {
	now := time.Now()
	oldItem := FeedItem{
		GUID:        "old",
		PublishedAt: &now,
	}
	
	cutoff := now.Add(-24 * time.Hour)
	oldItemOlder := FeedItem{
		GUID:        "old-older",
		PublishedAt: &cutoff,
	}
	
	noDateItem := FeedItem{
		GUID:        "no-date",
		PublishedAt: nil,
	}

	items := []FeedItem{oldItem, oldItemOlder, noDateItem}
	
	// Filter with cutoff 1 hour ago (should keep items from last hour)
	cutoff = now.Add(-1 * time.Hour)
	filtered := filterOldItems(items, cutoff)
	
	if len(filtered) != 2 {
		t.Errorf("expected 2 items, got %d", len(filtered))
	}
	
	// Should keep items with no date and recent items
	foundGUIDs := make(map[string]bool)
	for _, item := range filtered {
		foundGUIDs[item.GUID] = true
	}
	
	if !foundGUIDs["old"] {
		t.Error("expected to keep recent item")
	}
	if !foundGUIDs["no-date"] {
		t.Error("expected to keep item with no date")
	}
	if foundGUIDs["old-older"] {
		t.Error("expected to filter out old item")
	}
}

func TestShouldSkipFeed(t *testing.T) {
	tests := []struct {
		name     string
		feed     *dbgen.Feed
		expected bool
	}{
		{
			name:     "no errors",
			feed:     &dbgen.Feed{ErrorCount: 0},
			expected: false,
		},
		{
			name: "no last updated",
			feed: &dbgen.Feed{
				ErrorCount:   1,
				LastUpdated:  nil,
			},
			expected: false,
		},
		{
			name: "within backoff period",
			feed: &dbgen.Feed{
				ErrorCount:   1,
				LastUpdated:  &[]time.Time{time.Now().Add(-1 * time.Hour)}[0],
			},
			expected: true, // 2^1 = 2 hours backoff, only 1 hour passed
		},
		{
			name: "after backoff period",
			feed: &dbgen.Feed{
				ErrorCount:   1,
				LastUpdated:  &[]time.Time{time.Now().Add(-3 * time.Hour)}[0],
			},
			expected: false, // 2^1 = 2 hours backoff, 3 hours passed
		},
		{
			name: "max backoff cap",
			feed: &dbgen.Feed{
				ErrorCount:   10,
				LastUpdated:  &[]time.Time{time.Now().Add(-20 * time.Hour)}[0],
			},
			expected: true, // 2^10 would be 1024 hours, capped at 24, only 20 passed
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := shouldSkipFeed(tt.feed)
			if result != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestShouldSkipFeed_BackoffProgression(t *testing.T) {
	// Test exponential backoff progression
	for errorCount := 1; errorCount <= 5; errorCount++ {
		expectedBackoffHours := 1 << errorCount // 2^errorCount
		if expectedBackoffHours > maxBackoffHours {
			expectedBackoffHours = maxBackoffHours
		}
		
		lastUpdate := time.Now().Add(time.Duration(-expectedBackoffHours+1) * time.Hour)
		feed := &dbgen.Feed{
			ErrorCount:   int64(errorCount),
			LastUpdated:  &lastUpdate,
		}
		
		// Should skip because we're 1 hour before the backoff expires
		if !shouldSkipFeed(feed) {
			t.Errorf("expected to skip feed with error_count=%d, backoff=%dh", errorCount, expectedBackoffHours)
		}
		
		// Should not skip after backoff expires
		lastUpdate = time.Now().Add(time.Duration(-expectedBackoffHours-1) * time.Hour)
		feed.LastUpdated = &lastUpdate
		if shouldSkipFeed(feed) {
			t.Errorf("expected not to skip feed after backoff expires with error_count=%d", errorCount)
		}
	}
}

func TestFeedFetcher_InvalidXML(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/rss+xml")
		w.Write([]byte("not valid xml"))
	}))
	defer server.Close()

	fetcher := NewFeedFetcher()
	fetcher.AllowPrivateURLs = true
	_, err := fetcher.Fetch(context.Background(), server.URL)
	if err == nil {
		t.Error("expected error for invalid XML, got nil")
	}
	if !strings.Contains(err.Error(), "parse") {
		t.Errorf("expected parse error, got: %v", err)
	}
}

func TestFeedFetcher_MultipleItems(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/rss+xml")
		fmt.Fprintf(w, `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0">
<channel>
<title>Test Feed</title>
<link>https://example.com</link>
<item>
<guid>item-1</guid>
<link>https://example.com/1</link>
<title>Item 1</title>
</item>
<item>
<guid>item-2</guid>
<link>https://example.com/2</link>
<title>Item 2</title>
</item>
<item>
<link>https://example.com/3</link>
<title>Item 3 (no GUID)</title>
</item>
</channel>
</rss>`)
	}))
	defer server.Close()

	fetcher := NewFeedFetcher()
	fetcher.AllowPrivateURLs = true
	result, err := fetcher.Fetch(context.Background(), server.URL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.Items) != 3 {
		t.Fatalf("expected 3 items, got %d", len(result.Items))
	}

	// Item without GUID should use URL as GUID
	if result.Items[2].GUID != "https://example.com/3" {
		t.Errorf("expected GUID to fallback to URL, got %q", result.Items[2].GUID)
	}
}
