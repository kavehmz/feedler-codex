package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/mmcdole/gofeed"
)

func (a *App) backgroundRefresh() {
	ticker := time.NewTicker(30 * time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
		if err := a.RefreshScope(ctx, "all", 0); err != nil {
			log.Printf("background refresh: %v", err)
		}
		cancel()
	}
}

func (a *App) RefreshScope(ctx context.Context, scope string, scopeID int64) error {
	feeds, err := a.feedsForScope(ctx, scope, scopeID)
	if err != nil {
		return err
	}
	var lastErr error
	for _, feed := range feeds {
		if err := a.RefreshFeed(ctx, feed.ID); err != nil {
			lastErr = err
		}
	}
	return lastErr
}

func (a *App) feedsForScope(ctx context.Context, scope string, scopeID int64) ([]Feed, error) {
	feeds, err := a.ListFeeds(ctx)
	if err != nil {
		return nil, err
	}
	filtered := feeds[:0]
	for _, feed := range feeds {
		switch scope {
		case "folder":
			if feed.FolderID != nil && *feed.FolderID == scopeID {
				filtered = append(filtered, feed)
			}
		case "feed":
			if feed.ID == scopeID {
				filtered = append(filtered, feed)
			}
		default:
			filtered = append(filtered, feed)
		}
	}
	return filtered, nil
}

func (a *App) RefreshFeed(ctx context.Context, id int64) error {
	feed, err := a.GetFeed(ctx, id)
	if err != nil {
		return err
	}

	parser := gofeed.NewParser()
	parser.UserAgent = "Feedler/0.1 (+https://localhost)"
	parser.Client = &http.Client{Timeout: 20 * time.Second}

	parsed, err := parser.ParseURLWithContext(feed.URL, ctx)
	if err != nil {
		errText := cleanFetchError(err)
		_ = a.UpdateFeedFetchStatus(context.Background(), id, errText)
		return fmt.Errorf("%s: %s", feed.Title, errText)
	}

	if parsed.Link != "" {
		_ = a.UpdateFeedSiteURL(ctx, id, parsed.Link)
	}

	for _, item := range parsed.Items {
		if item == nil {
			continue
		}
		guid := item.GUID
		if guid == "" {
			guid = item.Link
		}
		if guid == "" {
			guid = item.Title
		}
		published := item.PublishedParsed
		if published == nil {
			published = item.UpdatedParsed
		}
		updated := item.UpdatedParsed
		if err := a.UpsertItem(ctx, id, guid, item.Title, item.Link, itemAuthor(item), item.Description, item.Content, published, updated); err != nil {
			_ = a.UpdateFeedFetchStatus(context.Background(), id, err.Error())
			return err
		}
	}

	if err := a.UpdateFeedFetchStatus(ctx, id, ""); err != nil {
		return err
	}
	return nil
}

func itemAuthor(item *gofeed.Item) string {
	if item == nil {
		return ""
	}
	if len(item.Authors) > 0 && item.Authors[0] != nil {
		return item.Authors[0].Name
	}
	if item.Author != nil {
		return item.Author.Name
	}
	return ""
}

func cleanFetchError(err error) string {
	text := cleanText(err.Error())
	text = strings.TrimPrefix(text, "failed to parse feed: ")
	if len(text) > 400 {
		text = text[:400]
	}
	return text
}
