package main

type Feed struct {
	ID            int64  `json:"id"`
	Title         string `json:"title"`
	SiteURL       string `json:"site_url"`
	FeedURL       string `json:"feed_url"`
	Category      string `json:"category"`
	LastError     string `json:"last_error,omitempty"`
	LastFetchedAt string `json:"last_fetched_at,omitempty"`
	UnreadCount   int    `json:"unread_count"`
	TotalCount    int    `json:"total_count"`
}

type Item struct {
	ID           int64  `json:"id"`
	FeedID       int64  `json:"feed_id"`
	FeedTitle    string `json:"feed_title"`
	FeedCategory string `json:"feed_category"`
	Title        string `json:"title"`
	Link         string `json:"link"`
	Summary      string `json:"summary"`
	Content      string `json:"content"`
	Author       string `json:"author,omitempty"`
	ImageURL     string `json:"image_url,omitempty"`
	PublishedAt  string `json:"published_at"`
	ReadAt       string `json:"read_at,omitempty"`
	Read         bool   `json:"read"`
}

type ItemQuery struct {
	FeedID   int64
	Category string
	Status   string
	Range    string
	Search   string
	Limit    int
	Offset   int
}

type FeedItem struct {
	FeedID      int64
	GUID        string
	Title       string
	Link        string
	Summary     string
	Content     string
	Author      string
	ImageURL    string
	PublishedAt string
}
