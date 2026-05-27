package main

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

type Store struct {
	db *sql.DB
}

func OpenStore(path string) (*Store, error) {
	dsn := fmt.Sprintf("file:%s?_busy_timeout=5000&_foreign_keys=on&_journal_mode=WAL", path)
	db, err := sql.Open("sqlite3", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	return &Store{db: db}, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) Migrate(ctx context.Context) error {
	statements := []string{
		`CREATE TABLE IF NOT EXISTS feeds (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			title TEXT NOT NULL,
			site_url TEXT NOT NULL DEFAULT '',
			feed_url TEXT NOT NULL UNIQUE,
			category TEXT NOT NULL DEFAULT 'Uncategorized',
			last_error TEXT NOT NULL DEFAULT '',
			last_fetched_at TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS items (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			feed_id INTEGER NOT NULL,
			guid TEXT NOT NULL,
			title TEXT NOT NULL,
			link TEXT NOT NULL DEFAULT '',
			summary TEXT NOT NULL DEFAULT '',
			content TEXT NOT NULL DEFAULT '',
			author TEXT NOT NULL DEFAULT '',
			image_url TEXT NOT NULL DEFAULT '',
			published_at TEXT NOT NULL,
			read_at TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL,
			FOREIGN KEY(feed_id) REFERENCES feeds(id) ON DELETE CASCADE,
			UNIQUE(feed_id, guid)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_items_feed_published ON items(feed_id, published_at DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_items_read_published ON items(read_at, published_at DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_feeds_category ON feeds(category, title)`,
	}

	for _, statement := range statements {
		if _, err := s.db.ExecContext(ctx, statement); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) UpsertFeed(ctx context.Context, feed Feed) error {
	now := nowString()
	if feed.Title == "" {
		feed.Title = feed.FeedURL
	}
	if feed.Category == "" {
		feed.Category = "Uncategorized"
	}

	_, err := s.db.ExecContext(ctx, `INSERT INTO feeds
		(title, site_url, feed_url, category, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(feed_url) DO UPDATE SET
			title = excluded.title,
			site_url = CASE WHEN excluded.site_url != '' THEN excluded.site_url ELSE feeds.site_url END,
			category = excluded.category,
			updated_at = excluded.updated_at`,
		feed.Title, feed.SiteURL, feed.FeedURL, feed.Category, now, now)
	return err
}

func (s *Store) ListFeeds(ctx context.Context) ([]Feed, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT
			f.id,
			f.title,
			f.site_url,
			f.feed_url,
			f.category,
			f.last_error,
			f.last_fetched_at,
			COUNT(i.id) AS total_count,
			COALESCE(SUM(CASE WHEN i.read_at = '' THEN 1 ELSE 0 END), 0) AS unread_count
		FROM feeds f
		LEFT JOIN items i ON i.feed_id = f.id
		GROUP BY f.id
		ORDER BY f.category COLLATE NOCASE, f.title COLLATE NOCASE`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var feeds []Feed
	for rows.Next() {
		var feed Feed
		if err := rows.Scan(
			&feed.ID,
			&feed.Title,
			&feed.SiteURL,
			&feed.FeedURL,
			&feed.Category,
			&feed.LastError,
			&feed.LastFetchedAt,
			&feed.TotalCount,
			&feed.UnreadCount,
		); err != nil {
			return nil, err
		}
		feeds = append(feeds, feed)
	}
	return feeds, rows.Err()
}

func (s *Store) UpdateFeedAfterRefresh(ctx context.Context, feedID int64, title, siteURL, lastError string) error {
	now := nowString()
	if lastError != "" {
		_, err := s.db.ExecContext(ctx, `UPDATE feeds SET last_error = ?, last_fetched_at = ?, updated_at = ? WHERE id = ?`,
			lastError, now, now, feedID)
		return err
	}

	_, err := s.db.ExecContext(ctx, `UPDATE feeds SET
			title = CASE WHEN ? != '' THEN ? ELSE title END,
			site_url = CASE WHEN ? != '' THEN ? ELSE site_url END,
			last_error = '',
			last_fetched_at = ?,
			updated_at = ?
		WHERE id = ?`,
		title, title, siteURL, siteURL, now, now, feedID)
	return err
}

func (s *Store) UpsertItem(ctx context.Context, item FeedItem) error {
	now := nowString()
	if item.GUID == "" {
		item.GUID = item.Link
	}
	if item.GUID == "" {
		item.GUID = item.Title
	}
	if item.Title == "" {
		item.Title = "Untitled"
	}
	if item.PublishedAt == "" {
		item.PublishedAt = now
	}

	_, err := s.db.ExecContext(ctx, `INSERT INTO items
		(feed_id, guid, title, link, summary, content, author, image_url, published_at, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(feed_id, guid) DO UPDATE SET
			title = excluded.title,
			link = excluded.link,
			summary = excluded.summary,
			content = excluded.content,
			author = excluded.author,
			image_url = excluded.image_url,
			published_at = excluded.published_at,
			updated_at = excluded.updated_at`,
		item.FeedID,
		item.GUID,
		item.Title,
		item.Link,
		item.Summary,
		item.Content,
		item.Author,
		item.ImageURL,
		item.PublishedAt,
		now,
		now)
	return err
}

func (s *Store) ListItems(ctx context.Context, q ItemQuery) ([]Item, error) {
	where, args := itemWhere(q)
	limit := q.Limit
	if limit <= 0 {
		limit = 80
	}
	if limit > 1000 {
		limit = 1000
	}
	offset := q.Offset
	if offset < 0 {
		offset = 0
	}

	args = append(args, limit, offset)
	query := fmt.Sprintf(`SELECT
			i.id,
			i.feed_id,
			f.title,
			f.category,
			i.title,
			i.link,
			i.summary,
			i.content,
			i.author,
			i.image_url,
			i.published_at,
			i.read_at
		FROM items i
		JOIN feeds f ON f.id = i.feed_id
		WHERE %s
		ORDER BY i.published_at DESC, i.id DESC
		LIMIT ? OFFSET ?`, strings.Join(where, " AND "))

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []Item
	for rows.Next() {
		item, err := scanItem(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Store) GetItem(ctx context.Context, id int64) (Item, error) {
	row := s.db.QueryRowContext(ctx, `SELECT
			i.id,
			i.feed_id,
			f.title,
			f.category,
			i.title,
			i.link,
			i.summary,
			i.content,
			i.author,
			i.image_url,
			i.published_at,
			i.read_at
		FROM items i
		JOIN feeds f ON f.id = i.feed_id
		WHERE i.id = ?`, id)
	return scanItem(row)
}

func (s *Store) SetItemRead(ctx context.Context, id int64, read bool) error {
	readAt := ""
	if read {
		readAt = nowString()
	}
	result, err := s.db.ExecContext(ctx, `UPDATE items SET read_at = ?, updated_at = ? WHERE id = ?`,
		readAt, nowString(), id)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (s *Store) SetItemsRead(ctx context.Context, ids []int64, read bool) (int64, error) {
	if len(ids) == 0 {
		return 0, nil
	}

	placeholders := make([]string, len(ids))
	args := make([]any, 0, len(ids)+2)
	readAt := ""
	if read {
		readAt = nowString()
	}
	args = append(args, readAt, nowString())
	for i, id := range ids {
		placeholders[i] = "?"
		args = append(args, id)
	}

	result, err := s.db.ExecContext(ctx,
		fmt.Sprintf(`UPDATE items SET read_at = ?, updated_at = ? WHERE id IN (%s)`, strings.Join(placeholders, ",")),
		args...)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

func itemWhere(q ItemQuery) ([]string, []any) {
	where := []string{"1=1"}
	args := []any{}

	if q.FeedID > 0 {
		where = append(where, "i.feed_id = ?")
		args = append(args, q.FeedID)
	}
	if q.Category != "" && q.Category != "all" {
		where = append(where, "f.category = ?")
		args = append(args, q.Category)
	}

	switch q.Status {
	case "read":
		where = append(where, "i.read_at != ''")
	case "all":
	default:
		where = append(where, "i.read_at = ''")
	}

	if q.Range != "" && q.Range != "all" {
		if start := rangeStart(q.Range); start != "" {
			where = append(where, "i.published_at >= ?")
			args = append(args, start)
		}
	}

	if q.Search != "" {
		needle := "%" + strings.ToLower(q.Search) + "%"
		where = append(where, "LOWER(i.title || ' ' || i.summary || ' ' || f.title) LIKE ?")
		args = append(args, needle)
	}

	return where, args
}

type itemScanner interface {
	Scan(dest ...any) error
}

func scanItem(scanner itemScanner) (Item, error) {
	var item Item
	if err := scanner.Scan(
		&item.ID,
		&item.FeedID,
		&item.FeedTitle,
		&item.FeedCategory,
		&item.Title,
		&item.Link,
		&item.Summary,
		&item.Content,
		&item.Author,
		&item.ImageURL,
		&item.PublishedAt,
		&item.ReadAt,
	); err != nil {
		return Item{}, err
	}
	item.Read = item.ReadAt != ""
	return item, nil
}

func rangeStart(name string) string {
	now := time.Now()
	switch name {
	case "today":
		start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
		return start.UTC().Format(time.RFC3339)
	case "week":
		return now.AddDate(0, 0, -7).UTC().Format(time.RFC3339)
	default:
		return ""
	}
}

func nowString() string {
	return time.Now().UTC().Format(time.RFC3339)
}
