package main

import (
	"context"
	"database/sql"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

//go:embed web/*
var embeddedWeb embed.FS

const appTimeLayout = "2006-01-02T15:04:05.000000000Z"

type App struct {
	db     *sql.DB
	static http.Handler
}

func main() {
	dbPath := getenv("DB_PATH", "./data/feedler.db")
	addr := getenv("ADDR", ":8080")

	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		log.Fatalf("create data directory: %v", err)
	}

	db, err := sql.Open("sqlite3", dbPath+"?_busy_timeout=5000&_journal_mode=WAL&_foreign_keys=on")
	if err != nil {
		log.Fatalf("open sqlite: %v", err)
	}
	defer db.Close()

	app, err := NewApp(db)
	if err != nil {
		log.Fatalf("initialize app: %v", err)
	}

	opmlPath := getenv("OPML_PATH", "./Feeds.opml")
	if err := app.ImportOPMLIfEmpty(context.Background(), opmlPath); err != nil {
		log.Printf("opml import skipped: %v", err)
	}

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
		defer cancel()
		if err := app.RefreshScope(ctx, "all", 0); err != nil {
			log.Printf("startup refresh: %v", err)
		}
	}()

	go app.backgroundRefresh()

	mux := http.NewServeMux()
	app.routes(mux)

	log.Printf("Feedler listening on %s", addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatal(err)
	}
}

func NewApp(db *sql.DB) (*App, error) {
	if err := migrate(db); err != nil {
		return nil, err
	}
	sub, err := fs.Sub(embeddedWeb, "web")
	if err != nil {
		return nil, err
	}
	return &App{db: db, static: http.FileServer(http.FS(sub))}, nil
}

func (a *App) routes(mux *http.ServeMux) {
	mux.HandleFunc("/api/state", a.handleState)
	mux.HandleFunc("/api/export", a.handleExport)
	mux.HandleFunc("/api/refresh", a.handleRefresh)
	mux.HandleFunc("/api/mark-read", a.handleMarkScopeRead)
	mux.HandleFunc("/api/settings", a.handleSettings)
	mux.HandleFunc("/api/feeds", a.handleFeeds)
	mux.HandleFunc("/api/feeds/", a.handleFeedPath)
	mux.HandleFunc("/api/folders", a.handleFolders)
	mux.HandleFunc("/api/folders/", a.handleFolderPath)
	mux.HandleFunc("/api/items/", a.handleItemPath)
	mux.HandleFunc("/", a.handleStatic)
}

func (a *App) handleStatic(w http.ResponseWriter, r *http.Request) {
	if strings.HasPrefix(r.URL.Path, "/api/") {
		http.NotFound(w, r)
		return
	}
	if r.URL.Path == "/" {
		http.ServeFileFS(w, r, mustSubFS(embeddedWeb, "web"), "index.html")
		return
	}
	if _, err := fs.Stat(mustSubFS(embeddedWeb, "web"), strings.TrimPrefix(r.URL.Path, "/")); err == nil {
		a.static.ServeHTTP(w, r)
		return
	}
	http.ServeFileFS(w, r, mustSubFS(embeddedWeb, "web"), "index.html")
}

func mustSubFS(root embed.FS, dir string) fs.FS {
	sub, err := fs.Sub(root, dir)
	if err != nil {
		panic(err)
	}
	return sub
}

func getenv(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

func migrate(db *sql.DB) error {
	statements := []string{
		`PRAGMA foreign_keys = ON`,
		`CREATE TABLE IF NOT EXISTS folders (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL UNIQUE COLLATE NOCASE,
			sort_order INTEGER NOT NULL DEFAULT 0,
			created_at TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS feeds (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			folder_id INTEGER REFERENCES folders(id) ON DELETE SET NULL,
			title TEXT NOT NULL,
			url TEXT NOT NULL UNIQUE,
			site_url TEXT NOT NULL DEFAULT '',
			last_checked_at TEXT,
			last_error TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS items (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			feed_id INTEGER NOT NULL REFERENCES feeds(id) ON DELETE CASCADE,
			guid TEXT NOT NULL,
			title TEXT NOT NULL,
			link TEXT NOT NULL DEFAULT '',
			author TEXT NOT NULL DEFAULT '',
			summary TEXT NOT NULL DEFAULT '',
			content TEXT NOT NULL DEFAULT '',
			published_at TEXT,
			updated_at TEXT,
			read_at TEXT,
			created_at TEXT NOT NULL,
			UNIQUE(feed_id, guid)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_items_feed_read ON items(feed_id, read_at)`,
		`CREATE INDEX IF NOT EXISTS idx_items_published ON items(published_at DESC)`,
		`CREATE TABLE IF NOT EXISTS settings (
			key TEXT PRIMARY KEY,
			value TEXT NOT NULL
		)`,
	}

	for _, stmt := range statements {
		if _, err := db.Exec(stmt); err != nil {
			return err
		}
	}

	defaults := map[string]string{
		"auto_mark_on_scroll": "false",
		"list_density":        "comfortable",
		"default_filter":      "unread",
		"timezone":            "Europe/Berlin",
	}
	for key, value := range defaults {
		if _, err := db.Exec(`INSERT OR IGNORE INTO settings (key, value) VALUES (?, ?)`, key, value); err != nil {
			return err
		}
	}
	return nil
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		log.Printf("write json: %v", err)
	}
}

func readJSON(r *http.Request, into any) error {
	defer r.Body.Close()
	return json.NewDecoder(r.Body).Decode(into)
}

func writeError(w http.ResponseWriter, status int, err error) {
	writeJSON(w, status, map[string]string{"error": err.Error()})
}

func parseID(value string) (int64, error) {
	id, err := strconv.ParseInt(value, 10, 64)
	if err != nil || id <= 0 {
		return 0, fmt.Errorf("invalid id %q", value)
	}
	return id, nil
}

func parsePathID(path, prefix string) (int64, string, error) {
	rest := strings.TrimPrefix(path, prefix)
	rest = strings.Trim(rest, "/")
	if rest == "" {
		return 0, "", errors.New("missing id")
	}
	parts := strings.Split(rest, "/")
	id, err := parseID(parts[0])
	if err != nil {
		return 0, "", err
	}
	tail := ""
	if len(parts) > 1 {
		tail = strings.Join(parts[1:], "/")
	}
	return id, tail, nil
}

func formatTime(t time.Time) string {
	return t.UTC().Format(appTimeLayout)
}

func parseDBTime(value string) *time.Time {
	if value == "" {
		return nil
	}
	t, err := time.Parse(appTimeLayout, value)
	if err != nil {
		return nil
	}
	return &t
}

func cleanText(value string) string {
	value = strings.ReplaceAll(value, "\u00a0", " ")
	value = strings.TrimSpace(value)
	return strings.Join(strings.Fields(value), " ")
}
