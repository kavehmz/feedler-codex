package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

func (a *App) FeedCount(ctx context.Context) (int, error) {
	var count int
	err := a.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM feeds`).Scan(&count)
	return count, err
}

func (a *App) EnsureFolder(ctx context.Context, name string) (*int64, error) {
	name = cleanText(name)
	if name == "" {
		return nil, nil
	}
	now := formatTime(time.Now())
	_, err := a.db.ExecContext(ctx, `INSERT OR IGNORE INTO folders (name, created_at) VALUES (?, ?)`, name, now)
	if err != nil {
		return nil, err
	}
	var id int64
	if err := a.db.QueryRowContext(ctx, `SELECT id FROM folders WHERE name = ? COLLATE NOCASE`, name).Scan(&id); err != nil {
		return nil, err
	}
	return &id, nil
}

func (a *App) CreateFeed(ctx context.Context, title, url, siteURL string, folderID *int64) (int64, error) {
	title = cleanText(title)
	url = strings.TrimSpace(url)
	siteURL = strings.TrimSpace(siteURL)
	if url == "" {
		return 0, errors.New("feed url is required")
	}
	if title == "" {
		title = url
	}
	now := formatTime(time.Now())
	res, err := a.db.ExecContext(ctx, `INSERT INTO feeds (folder_id, title, url, site_url, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)`, nullableInt(folderID), title, url, siteURL, now, now)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (a *App) UpsertFeed(ctx context.Context, title, url, siteURL string, folderID *int64) (int64, error) {
	title = cleanText(title)
	url = strings.TrimSpace(url)
	siteURL = strings.TrimSpace(siteURL)
	if url == "" {
		return 0, errors.New("feed url is required")
	}
	if title == "" {
		title = url
	}
	now := formatTime(time.Now())
	_, err := a.db.ExecContext(ctx, `
		INSERT INTO feeds (folder_id, title, url, site_url, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(url) DO UPDATE SET
			folder_id = excluded.folder_id,
			title = excluded.title,
			site_url = CASE WHEN excluded.site_url != '' THEN excluded.site_url ELSE feeds.site_url END,
			updated_at = excluded.updated_at
	`, nullableInt(folderID), title, url, siteURL, now, now)
	if err != nil {
		return 0, err
	}
	var id int64
	if err := a.db.QueryRowContext(ctx, `SELECT id FROM feeds WHERE url = ?`, url).Scan(&id); err != nil {
		return 0, err
	}
	return id, nil
}

func (a *App) UpdateFeed(ctx context.Context, id int64, title string, folderID *int64) error {
	title = cleanText(title)
	if title == "" {
		return errors.New("title is required")
	}
	_, err := a.db.ExecContext(ctx, `UPDATE feeds SET title = ?, folder_id = ?, updated_at = ? WHERE id = ?`, title, nullableInt(folderID), formatTime(time.Now()), id)
	return err
}

func (a *App) DeleteFeed(ctx context.Context, id int64) error {
	_, err := a.db.ExecContext(ctx, `DELETE FROM feeds WHERE id = ?`, id)
	return err
}

func (a *App) UpdateFolder(ctx context.Context, id int64, name string) error {
	name = cleanText(name)
	if name == "" {
		return errors.New("folder name is required")
	}
	_, err := a.db.ExecContext(ctx, `UPDATE folders SET name = ? WHERE id = ?`, name, id)
	return err
}

func (a *App) DeleteFolder(ctx context.Context, id int64) error {
	_, err := a.db.ExecContext(ctx, `DELETE FROM folders WHERE id = ?`, id)
	return err
}

func (a *App) ListFolders(ctx context.Context) ([]Folder, error) {
	rows, err := a.db.QueryContext(ctx, `
		SELECT fo.id, fo.name, fo.sort_order,
			COALESCE(SUM(CASE WHEN it.id IS NOT NULL AND it.read_at IS NULL THEN 1 ELSE 0 END), 0) AS unread,
			COUNT(it.id) AS total
		FROM folders fo
		LEFT JOIN feeds fe ON fe.folder_id = fo.id
		LEFT JOIN items it ON it.feed_id = fe.id
		GROUP BY fo.id
		ORDER BY fo.sort_order, fo.name COLLATE NOCASE
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var folders []Folder
	for rows.Next() {
		var folder Folder
		if err := rows.Scan(&folder.ID, &folder.Name, &folder.SortOrder, &folder.Unread, &folder.Total); err != nil {
			return nil, err
		}
		folders = append(folders, folder)
	}
	return folders, rows.Err()
}

func (a *App) ListFeeds(ctx context.Context) ([]Feed, error) {
	rows, err := a.db.QueryContext(ctx, `
		SELECT fe.id, fe.folder_id, fe.title, fe.url, fe.site_url, fe.last_checked_at, fe.last_error,
			COALESCE(SUM(CASE WHEN it.id IS NOT NULL AND it.read_at IS NULL THEN 1 ELSE 0 END), 0) AS unread,
			COUNT(it.id) AS total
		FROM feeds fe
		LEFT JOIN items it ON it.feed_id = fe.id
		GROUP BY fe.id
		ORDER BY fe.title COLLATE NOCASE
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var feeds []Feed
	for rows.Next() {
		var feed Feed
		var folder sql.NullInt64
		var checked sql.NullString
		if err := rows.Scan(&feed.ID, &folder, &feed.Title, &feed.URL, &feed.SiteURL, &checked, &feed.LastError, &feed.Unread, &feed.Total); err != nil {
			return nil, err
		}
		if folder.Valid {
			feed.FolderID = &folder.Int64
		}
		if checked.Valid {
			feed.LastCheckedAt = parseDBTime(checked.String)
		}
		feeds = append(feeds, feed)
	}
	return feeds, rows.Err()
}

func (a *App) GetFeed(ctx context.Context, id int64) (Feed, error) {
	var feed Feed
	var folder sql.NullInt64
	var checked sql.NullString
	err := a.db.QueryRowContext(ctx, `SELECT id, folder_id, title, url, site_url, last_checked_at, last_error FROM feeds WHERE id = ?`, id).
		Scan(&feed.ID, &folder, &feed.Title, &feed.URL, &feed.SiteURL, &checked, &feed.LastError)
	if folder.Valid {
		feed.FolderID = &folder.Int64
	}
	if checked.Valid {
		feed.LastCheckedAt = parseDBTime(checked.String)
	}
	return feed, err
}

func (a *App) ListItems(ctx context.Context, scope string, scopeID int64, filter string, limit int) ([]Item, error) {
	where, args := scopeClause(scope, scopeID)
	if filter == "unread" {
		where = append(where, "it.read_at IS NULL")
	}
	if limit <= 0 || limit > 1000 {
		limit = 400
	}

	query := `
		SELECT it.id, it.feed_id, fe.title, fe.folder_id, it.title, it.link, it.author,
			CASE WHEN length(it.summary) > 1800 THEN substr(it.summary, 1, 1800) || '...' ELSE it.summary END,
			CASE WHEN length(it.content) > 3200 THEN substr(it.content, 1, 3200) || '...' ELSE it.content END,
			it.published_at, it.updated_at, it.read_at
		FROM items it
		JOIN feeds fe ON fe.id = it.feed_id
	`
	if len(where) > 0 {
		query += " WHERE " + strings.Join(where, " AND ")
	}
	query += ` ORDER BY COALESCE(it.published_at, it.created_at) DESC, it.id DESC LIMIT ?`
	args = append(args, limit)

	rows, err := a.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := []Item{}
	for rows.Next() {
		item, err := scanItem(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (a *App) UpsertItem(ctx context.Context, feedID int64, guid, title, link, author, summary, content string, publishedAt, updatedAt *time.Time) error {
	guid = strings.TrimSpace(guid)
	title = cleanText(title)
	link = strings.TrimSpace(link)
	if guid == "" {
		if link != "" {
			guid = link
		} else {
			guid = title
		}
	}
	if title == "" {
		title = "Untitled"
	}
	now := formatTime(time.Now())
	var published any
	if publishedAt != nil {
		published = formatTime(*publishedAt)
	} else {
		published = now
	}
	var updated any
	if updatedAt != nil {
		updated = formatTime(*updatedAt)
	}

	_, err := a.db.ExecContext(ctx, `
		INSERT INTO items (feed_id, guid, title, link, author, summary, content, published_at, updated_at, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(feed_id, guid) DO UPDATE SET
			title = excluded.title,
			link = excluded.link,
			author = excluded.author,
			summary = excluded.summary,
			content = excluded.content,
			published_at = excluded.published_at,
			updated_at = excluded.updated_at
	`, feedID, guid, title, link, cleanText(stripTags(author)), strings.TrimSpace(summary), strings.TrimSpace(content), published, updated, now)
	return err
}

func (a *App) MarkItemRead(ctx context.Context, id int64, read bool) error {
	var value any
	if read {
		value = formatTime(time.Now())
	}
	_, err := a.db.ExecContext(ctx, `UPDATE items SET read_at = ? WHERE id = ?`, value, id)
	return err
}

func (a *App) MarkScopeRead(ctx context.Context, scope string, scopeID int64) (int64, error) {
	where := []string{"read_at IS NULL"}
	args := []any{formatTime(time.Now())}
	switch scope {
	case "folder":
		where = append(where, `feed_id IN (SELECT id FROM feeds WHERE folder_id = ?)`)
		args = append(args, scopeID)
	case "feed":
		where = append(where, `feed_id = ?`)
		args = append(args, scopeID)
	}
	query := `UPDATE items SET read_at = ? WHERE ` + strings.Join(where, " AND ")
	res, err := a.db.ExecContext(ctx, query, args...)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func (a *App) UpdateFeedFetchStatus(ctx context.Context, id int64, errText string) error {
	_, err := a.db.ExecContext(ctx, `UPDATE feeds SET last_checked_at = ?, last_error = ?, updated_at = ? WHERE id = ?`, formatTime(time.Now()), errText, formatTime(time.Now()), id)
	return err
}

func (a *App) UpdateFeedSiteURL(ctx context.Context, id int64, siteURL string) error {
	siteURL = strings.TrimSpace(siteURL)
	if siteURL == "" {
		return nil
	}
	_, err := a.db.ExecContext(ctx, `UPDATE feeds SET site_url = ?, updated_at = ? WHERE id = ? AND site_url = ''`, siteURL, formatTime(time.Now()), id)
	return err
}

func (a *App) GetSettings(ctx context.Context) (Settings, error) {
	rows, err := a.db.QueryContext(ctx, `SELECT key, value FROM settings`)
	if err != nil {
		return Settings{}, err
	}
	defer rows.Close()

	settings := Settings{
		AutoMarkOnScroll: false,
		ListDensity:      "comfortable",
		DefaultFilter:    "unread",
		Timezone:         "Europe/Berlin",
	}
	for rows.Next() {
		var key, value string
		if err := rows.Scan(&key, &value); err != nil {
			return settings, err
		}
		switch key {
		case "auto_mark_on_scroll":
			settings.AutoMarkOnScroll = value == "true"
		case "list_density":
			settings.ListDensity = value
		case "default_filter":
			settings.DefaultFilter = value
		case "timezone":
			settings.Timezone = value
		}
	}
	return settings, rows.Err()
}

func (a *App) SaveSettings(ctx context.Context, settings Settings) error {
	if settings.ListDensity != "compact" {
		settings.ListDensity = "comfortable"
	}
	if settings.DefaultFilter != "all" {
		settings.DefaultFilter = "unread"
	}
	if strings.TrimSpace(settings.Timezone) == "" {
		settings.Timezone = "Europe/Berlin"
	}
	values := map[string]string{
		"auto_mark_on_scroll": strconvBool(settings.AutoMarkOnScroll),
		"list_density":        settings.ListDensity,
		"default_filter":      settings.DefaultFilter,
		"timezone":            settings.Timezone,
	}
	for key, value := range values {
		if _, err := a.db.ExecContext(ctx, `INSERT INTO settings (key, value) VALUES (?, ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value`, key, value); err != nil {
			return err
		}
	}
	return nil
}

func (a *App) ExportItems(ctx context.Context, scope string, scopeID int64, startUTC, endUTC time.Time) ([]Item, error) {
	where, args := scopeClause(scope, scopeID)
	where = append(where, "COALESCE(it.published_at, it.created_at) >= ?", "COALESCE(it.published_at, it.created_at) < ?")
	args = append(args, formatTime(startUTC), formatTime(endUTC))
	query := `
		SELECT it.id, it.feed_id, fe.title, fe.folder_id, it.title, it.link, it.author,
			it.summary, it.content, it.published_at, it.updated_at, it.read_at
		FROM items it
		JOIN feeds fe ON fe.id = it.feed_id
		WHERE ` + strings.Join(where, " AND ") + `
		ORDER BY COALESCE(it.published_at, it.created_at) DESC, it.id DESC
	`
	rows, err := a.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := []Item{}
	for rows.Next() {
		item, err := scanItem(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

type itemScanner interface {
	Scan(dest ...any) error
}

func scanItem(row itemScanner) (Item, error) {
	var item Item
	var folder sql.NullInt64
	var published, updated, read sql.NullString
	err := row.Scan(&item.ID, &item.FeedID, &item.FeedTitle, &folder, &item.Title, &item.Link, &item.Author, &item.Summary, &item.Content, &published, &updated, &read)
	if err != nil {
		return item, err
	}
	if folder.Valid {
		item.FolderID = &folder.Int64
	}
	if published.Valid {
		item.PublishedAt = parseDBTime(published.String)
	}
	if updated.Valid {
		item.UpdatedAt = parseDBTime(updated.String)
	}
	if read.Valid {
		item.ReadAt = parseDBTime(read.String)
	}
	item.ReaderURL = fmt.Sprintf("/?item=%d", item.ID)
	return item, nil
}

func scopeClause(scope string, scopeID int64) ([]string, []any) {
	switch scope {
	case "folder":
		return []string{"fe.folder_id = ?"}, []any{scopeID}
	case "feed":
		return []string{"fe.id = ?"}, []any{scopeID}
	default:
		return nil, nil
	}
}

func nullableInt(id *int64) any {
	if id == nil || *id == 0 {
		return nil
	}
	return *id
}

func strconvBool(value bool) string {
	if value {
		return "true"
	}
	return "false"
}
