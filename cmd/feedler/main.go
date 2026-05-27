package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

type Config struct {
	Addr     string
	DataDir  string
	WebDir   string
	OPMLPath string
}

func main() {
	cfg := loadConfig()

	if err := os.MkdirAll(cfg.DataDir, 0o755); err != nil {
		log.Fatalf("create data dir: %v", err)
	}

	store, err := OpenStore(filepath.Join(cfg.DataDir, "feedler.db"))
	if err != nil {
		log.Fatalf("open store: %v", err)
	}
	defer store.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	if err := store.Migrate(ctx); err != nil {
		cancel()
		log.Fatalf("migrate store: %v", err)
	}

	if cfg.OPMLPath != "" {
		if count, err := store.ImportOPML(ctx, cfg.OPMLPath); err != nil {
			log.Printf("opml import skipped: %v", err)
		} else {
			log.Printf("opml import complete: %d feed(s)", count)
		}
	}
	cancel()

	refresher := NewRefresher(store)
	refresher.Start("startup")
	go refresher.RefreshEvery(30 * time.Minute)

	server := &Server{
		store:     store,
		refresher: refresher,
		webDir:    cfg.WebDir,
	}

	log.Printf("feedler listening on %s", cfg.Addr)
	if err := http.ListenAndServe(cfg.Addr, server.Routes()); err != nil {
		log.Fatal(err)
	}
}

func loadConfig() Config {
	return Config{
		Addr:     getenv("ADDR", ":8080"),
		DataDir:  getenv("DATA_DIR", "./data"),
		WebDir:   getenv("WEB_DIR", "./web"),
		OPMLPath: getenv("OPML_PATH", "./Feeds.opml"),
	}
}

func getenv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
