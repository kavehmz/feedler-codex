package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/mmcdole/gofeed"
)

type RefreshStatus struct {
	Running    bool     `json:"running"`
	Source     string   `json:"source,omitempty"`
	StartedAt  string   `json:"started_at,omitempty"`
	FinishedAt string   `json:"finished_at,omitempty"`
	Total      int      `json:"total"`
	Done       int      `json:"done"`
	Items      int      `json:"items"`
	Errors     []string `json:"errors"`
}

type Refresher struct {
	store  *Store
	mu     sync.Mutex
	status RefreshStatus
}

func NewRefresher(store *Store) *Refresher {
	return &Refresher{store: store}
}

func (r *Refresher) Start(source string) bool {
	r.mu.Lock()
	if r.status.Running {
		r.mu.Unlock()
		return false
	}
	r.status = RefreshStatus{
		Running:   true,
		Source:    source,
		StartedAt: nowString(),
		Errors:    []string{},
	}
	r.mu.Unlock()

	go r.run()
	return true
}

func (r *Refresher) RefreshEvery(interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for range ticker.C {
		r.Start("scheduled")
	}
}

func (r *Refresher) Status() RefreshStatus {
	r.mu.Lock()
	defer r.mu.Unlock()
	status := r.status
	status.Errors = append([]string{}, r.status.Errors...)
	return status
}

func (r *Refresher) run() {
	ctx := context.Background()
	feeds, err := r.store.ListFeeds(ctx)
	if err != nil {
		r.finishWithError(fmt.Sprintf("list feeds: %v", err))
		return
	}

	r.mu.Lock()
	r.status.Total = len(feeds)
	r.mu.Unlock()

	if len(feeds) == 0 {
		r.finish()
		return
	}

	workerCount := 6
	if len(feeds) < workerCount {
		workerCount = len(feeds)
	}

	jobs := make(chan Feed)
	var wg sync.WaitGroup
	for i := 0; i < workerCount; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for feed := range jobs {
				count, err := r.refreshFeed(feed)
				if err != nil {
					log.Printf("refresh %s: %v", feed.Title, err)
					_ = r.store.UpdateFeedAfterRefresh(ctx, feed.ID, "", "", err.Error())
					r.recordDone(0, fmt.Sprintf("%s: %v", feed.Title, err))
					continue
				}
				r.recordDone(count, "")
			}
		}()
	}

	for _, feed := range feeds {
		jobs <- feed
	}
	close(jobs)
	wg.Wait()
	r.finish()
}

func (r *Refresher) refreshFeed(feed Feed) (int, error) {
	parser := gofeed.NewParser()
	parser.Client = &http.Client{
		Timeout:   25 * time.Second,
		Transport: userAgentTransport{base: http.DefaultTransport},
	}

	parsed, err := parser.ParseURL(feed.FeedURL)
	if err != nil {
		return 0, err
	}

	ctx := context.Background()
	count := 0
	for _, parsedItem := range parsed.Items {
		publishedAt := nowString()
		if parsedItem.PublishedParsed != nil {
			publishedAt = parsedItem.PublishedParsed.UTC().Format(time.RFC3339)
		} else if parsedItem.UpdatedParsed != nil {
			publishedAt = parsedItem.UpdatedParsed.UTC().Format(time.RFC3339)
		}

		summary := cleanText(parsedItem.Description)
		content := cleanText(firstNonEmpty(parsedItem.Content, parsedItem.Description))
		if summary == "" {
			summary = truncateRunes(content, 500)
		}

		author := ""
		if parsedItem.Author != nil {
			author = strings.TrimSpace(parsedItem.Author.Name)
		}

		imageURL := ""
		if parsedItem.Image != nil {
			imageURL = strings.TrimSpace(parsedItem.Image.URL)
		} else if parsed.Image != nil {
			imageURL = strings.TrimSpace(parsed.Image.URL)
		}

		item := FeedItem{
			FeedID:      feed.ID,
			GUID:        firstNonEmpty(parsedItem.GUID, parsedItem.Link),
			Title:       cleanText(parsedItem.Title),
			Link:        strings.TrimSpace(parsedItem.Link),
			Summary:     summary,
			Content:     content,
			Author:      author,
			ImageURL:    imageURL,
			PublishedAt: publishedAt,
		}
		if item.GUID == "" {
			item.GUID = item.Title + "|" + item.PublishedAt
		}
		if err := r.store.UpsertItem(ctx, item); err != nil {
			return count, err
		}
		count++
	}

	return count, r.store.UpdateFeedAfterRefresh(ctx, feed.ID, cleanText(parsed.Title), strings.TrimSpace(parsed.Link), "")
}

func (r *Refresher) recordDone(items int, errText string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.status.Done++
	r.status.Items += items
	if errText != "" {
		if len(r.status.Errors) < 12 {
			r.status.Errors = append(r.status.Errors, errText)
		}
	}
}

func (r *Refresher) finishWithError(errText string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.status.Errors = append(r.status.Errors, errText)
	r.status.Running = false
	r.status.FinishedAt = nowString()
}

func (r *Refresher) finish() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.status.Running = false
	r.status.FinishedAt = nowString()
}

type userAgentTransport struct {
	base http.RoundTripper
}

func (t userAgentTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	base := t.base
	if base == nil {
		base = http.DefaultTransport
	}
	next := req.Clone(req.Context())
	next.Header.Set("User-Agent", "Feedler/0.1 (+https://localhost)")
	next.Header.Set("Accept", "application/rss+xml, application/atom+xml, application/xml, text/xml, */*")
	return base.RoundTrip(next)
}
