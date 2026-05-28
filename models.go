package main

import "time"

type Folder struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	SortOrder int    `json:"sort_order"`
	Unread    int    `json:"unread"`
	Total     int    `json:"total"`
}

type Feed struct {
	ID            int64      `json:"id"`
	FolderID      *int64     `json:"folder_id"`
	Title         string     `json:"title"`
	URL           string     `json:"url"`
	SiteURL       string     `json:"site_url"`
	LastCheckedAt *time.Time `json:"last_checked_at"`
	LastError     string     `json:"last_error"`
	Unread        int        `json:"unread"`
	Total         int        `json:"total"`
}

type Item struct {
	ID          int64      `json:"id"`
	FeedID      int64      `json:"feed_id"`
	FeedTitle   string     `json:"feed_title"`
	FolderID    *int64     `json:"folder_id"`
	Title       string     `json:"title"`
	Link        string     `json:"link"`
	Author      string     `json:"author"`
	Summary     string     `json:"summary"`
	Content     string     `json:"content"`
	PublishedAt *time.Time `json:"published_at"`
	UpdatedAt   *time.Time `json:"updated_at"`
	ReadAt      *time.Time `json:"read_at"`
	ReaderURL   string     `json:"reader_url"`
}

type Settings struct {
	AutoMarkOnScroll bool   `json:"auto_mark_on_scroll"`
	ListDensity      string `json:"list_density"`
	DefaultFilter    string `json:"default_filter"`
	Timezone         string `json:"timezone"`
}

type StateResponse struct {
	Folders  []Folder `json:"folders"`
	Feeds    []Feed   `json:"feeds"`
	Items    []Item   `json:"items"`
	Settings Settings `json:"settings"`
}
