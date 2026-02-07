package srv

import (
	"encoding/xml"
	"fmt"
	"io"
	"time"
)

// OPML structures for import/export
type OPML struct {
	XMLName xml.Name `xml:"opml"`
	Version string   `xml:"version,attr"`
	Head    OPMLHead `xml:"head"`
	Body    OPMLBody `xml:"body"`
}

type OPMLHead struct {
	Title       string `xml:"title"`
	DateCreated string `xml:"dateCreated,omitempty"`
}

type OPMLBody struct {
	Outlines []OPMLOutline `xml:"outline"`
}

type OPMLOutline struct {
	Text     string        `xml:"text,attr"`
	Title    string        `xml:"title,attr,omitempty"`
	Type     string        `xml:"type,attr,omitempty"`
	XMLURL   string        `xml:"xmlUrl,attr,omitempty"`
	HTMLURL  string        `xml:"htmlUrl,attr,omitempty"`
	Outlines []OPMLOutline `xml:"outline,omitempty"`
}

// ParseOPML parses an OPML file and returns a flat list of feed URLs
func ParseOPML(r io.Reader) ([]FeedImport, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("read opml: %w", err)
	}

	var opml OPML
	if err := xml.Unmarshal(data, &opml); err != nil {
		return nil, fmt.Errorf("parse opml: %w", err)
	}

	var feeds []FeedImport
	extractFeeds(opml.Body.Outlines, "", &feeds)
	return feeds, nil
}

// FeedImport represents a feed to import
type FeedImport struct {
	URL      string
	Title    string
	Category string
}

// extractFeeds recursively extracts feeds from OPML outlines
func extractFeeds(outlines []OPMLOutline, category string, feeds *[]FeedImport) {
	for _, o := range outlines {
		if o.XMLURL != "" {
			// This is a feed
			title := o.Title
			if title == "" {
				title = o.Text
			}
			*feeds = append(*feeds, FeedImport{
				URL:      o.XMLURL,
				Title:    title,
				Category: category,
			})
		} else if len(o.Outlines) > 0 {
			// This is a category/folder
			catName := o.Title
			if catName == "" {
				catName = o.Text
			}
			extractFeeds(o.Outlines, catName, feeds)
		}
	}
}

// GenerateOPML creates an OPML export from feeds
func GenerateOPML(title string, feeds []FeedExport) ([]byte, error) {
	opml := OPML{
		Version: "2.0",
		Head: OPMLHead{
			Title:       title,
			DateCreated: time.Now().Format(time.RFC1123),
		},
	}

	// Group feeds by category
	categories := make(map[string][]FeedExport)
	for _, f := range feeds {
		cat := f.Category
		if cat == "" {
			cat = "_uncategorized"
		}
		categories[cat] = append(categories[cat], f)
	}

	// Build outlines
	for cat, catFeeds := range categories {
		if cat == "_uncategorized" {
			// Add uncategorized feeds at root level
			for _, f := range catFeeds {
				opml.Body.Outlines = append(opml.Body.Outlines, OPMLOutline{
					Text:    f.Title,
					Title:   f.Title,
					Type:    "rss",
					XMLURL:  f.URL,
					HTMLURL: f.SiteURL,
				})
			}
		} else {
			// Add category with feeds
			catOutline := OPMLOutline{
				Text:  cat,
				Title: cat,
			}
			for _, f := range catFeeds {
				catOutline.Outlines = append(catOutline.Outlines, OPMLOutline{
					Text:    f.Title,
					Title:   f.Title,
					Type:    "rss",
					XMLURL:  f.URL,
					HTMLURL: f.SiteURL,
				})
			}
			opml.Body.Outlines = append(opml.Body.Outlines, catOutline)
		}
	}

	output, err := xml.MarshalIndent(opml, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal opml: %w", err)
	}

	return append([]byte(xml.Header), output...), nil
}

// FeedExport represents a feed for export
type FeedExport struct {
	URL      string
	Title    string
	SiteURL  string
	Category string
}
