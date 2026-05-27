package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type Server struct {
	store     *Store
	refresher *Refresher
	webDir    string
}

func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/health", s.handleHealth)
	mux.HandleFunc("/api/feeds", s.handleFeeds)
	mux.HandleFunc("/api/feeds/", s.handleFeed)
	mux.HandleFunc("/api/items", s.handleItems)
	mux.HandleFunc("/api/items/", s.handleItem)
	mux.HandleFunc("/api/read", s.handleBulkRead)
	mux.HandleFunc("/api/refresh", s.handleRefresh)
	mux.HandleFunc("/api/export", s.handleExport)
	mux.HandleFunc("/api/import", s.handleImport)
	mux.HandleFunc("/", s.handleStatic)
	return logRequests(mux)
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) handleFeeds(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		feeds, err := s.store.ListFeeds(r.Context())
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"feeds": feeds})
	case http.MethodPost:
		var request struct {
			Title    string `json:"title"`
			FeedURL  string `json:"feed_url"`
			Category string `json:"category"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			writeError(w, http.StatusBadRequest, errors.New("invalid json body"))
			return
		}
		feed, err := s.store.CreateFeed(r.Context(), Feed{
			Title:    request.Title,
			FeedURL:  request.FeedURL,
			Category: request.Category,
		})
		if err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusCreated, map[string]any{"feed": feed})
	default:
		methodNotAllowed(w)
		return
	}
}

func (s *Server) handleFeed(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/feeds/"), "/"), "/")
	if len(parts) == 0 || parts[0] == "" {
		http.NotFound(w, r)
		return
	}
	id, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, errors.New("invalid feed id"))
		return
	}

	if len(parts) == 2 && parts[1] == "refresh" {
		if r.Method != http.MethodPost {
			methodNotAllowed(w)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 35*time.Second)
		defer cancel()
		feed, count, err := s.refresher.RefreshFeed(ctx, id)
		if err != nil {
			status := http.StatusBadGateway
			if errors.Is(err, sql.ErrNoRows) {
				status = http.StatusNotFound
			}
			writeJSON(w, status, map[string]any{"feed": feed, "items": count, "error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"feed": feed, "items": count})
		return
	}
	if len(parts) != 1 {
		http.NotFound(w, r)
		return
	}

	switch r.Method {
	case http.MethodPatch:
		var request struct {
			Title    string `json:"title"`
			Category string `json:"category"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			writeError(w, http.StatusBadRequest, errors.New("invalid json body"))
			return
		}
		feed, err := s.store.UpdateFeed(r.Context(), id, request.Title, request.Category)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				writeError(w, http.StatusNotFound, errors.New("feed not found"))
				return
			}
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"feed": feed})
	case http.MethodDelete:
		if err := s.store.DeleteFeed(r.Context(), id); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				writeError(w, http.StatusNotFound, errors.New("feed not found"))
				return
			}
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"deleted": id})
	default:
		methodNotAllowed(w)
	}
}

func (s *Server) handleItems(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	items, err := s.store.ListItems(r.Context(), itemQueryFromRequest(r, 80))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (s *Server) handleItem(w http.ResponseWriter, r *http.Request) {
	idText := strings.TrimPrefix(r.URL.Path, "/api/items/")
	idText = strings.Trim(idText, "/")
	if idText == "" || strings.Contains(idText, "/") {
		http.NotFound(w, r)
		return
	}
	id, err := strconv.ParseInt(idText, 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, errors.New("invalid item id"))
		return
	}

	switch r.Method {
	case http.MethodGet:
		item, err := s.store.GetItem(r.Context(), id)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				writeError(w, http.StatusNotFound, errors.New("item not found"))
				return
			}
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"item": item})
	case http.MethodPatch:
		var request struct {
			Read *bool `json:"read"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			writeError(w, http.StatusBadRequest, errors.New("invalid json body"))
			return
		}
		if request.Read == nil {
			writeError(w, http.StatusBadRequest, errors.New("read is required"))
			return
		}
		if err := s.store.SetItemRead(r.Context(), id, *request.Read); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				writeError(w, http.StatusNotFound, errors.New("item not found"))
				return
			}
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		item, err := s.store.GetItem(r.Context(), id)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"item": item})
	default:
		methodNotAllowed(w)
	}
}

func (s *Server) handleBulkRead(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	var request struct {
		IDs      []int64 `json:"ids"`
		Read     bool    `json:"read"`
		FeedID   int64   `json:"feed_id"`
		Category string  `json:"category"`
		Status   string  `json:"status"`
		Range    string  `json:"range"`
		Search   string  `json:"q"`
		Timezone string  `json:"timezone"`
	}
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		writeError(w, http.StatusBadRequest, errors.New("invalid json body"))
		return
	}
	var affected int64
	var err error
	if len(request.IDs) > 0 {
		affected, err = s.store.SetItemsRead(r.Context(), request.IDs, request.Read)
	} else {
		status := request.Status
		if status == "" {
			status = "unread"
		}
		affected, err = s.store.SetItemsMatchingRead(r.Context(), ItemQuery{
			FeedID:   request.FeedID,
			Category: request.Category,
			Status:   status,
			Range:    request.Range,
			Search:   strings.TrimSpace(request.Search),
			Timezone: request.Timezone,
		}, request.Read)
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"updated": affected})
}

func (s *Server) handleRefresh(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, s.refresher.Status())
	case http.MethodPost:
		started := s.refresher.Start("manual")
		statusCode := http.StatusAccepted
		if !started {
			statusCode = http.StatusOK
		}
		writeJSON(w, statusCode, s.refresher.Status())
	default:
		methodNotAllowed(w)
	}
}

func (s *Server) handleImport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}

	file, _, err := r.FormFile("opml")
	if err != nil {
		writeError(w, http.StatusBadRequest, errors.New("multipart field 'opml' is required"))
		return
	}
	defer file.Close()

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	count, err := s.store.ImportOPMLReader(ctx, file)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	s.refresher.Start("import")
	writeJSON(w, http.StatusOK, map[string]any{"feeds": count})
}

func (s *Server) handleExport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	query := itemQueryFromRequest(r, 1000)
	period := r.URL.Query().Get("period")
	if period == "" {
		period = "today"
	}
	query.Range = period
	if period == "unread" {
		query.Range = "all"
		query.Status = "unread"
	}
	if query.Status == "" {
		query.Status = "all"
	}

	items, err := s.store.ListItems(r.Context(), query)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	markdown := BuildMarkdownExport(period, query.Status, query.Timezone, baseURL(r), items)
	w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"feedler-%s.md\"", period))
	_, _ = w.Write([]byte(markdown))
}

func (s *Server) handleStatic(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		methodNotAllowed(w)
		return
	}

	requestPath := strings.TrimPrefix(filepath.Clean(r.URL.Path), string(filepath.Separator))
	if requestPath == "." || requestPath == "" {
		http.ServeFile(w, r, filepath.Join(s.webDir, "index.html"))
		return
	}

	fullPath := filepath.Join(s.webDir, requestPath)
	if info, err := os.Stat(fullPath); err == nil && !info.IsDir() {
		http.ServeFile(w, r, fullPath)
		return
	}

	http.ServeFile(w, r, filepath.Join(s.webDir, "index.html"))
}

func itemQueryFromRequest(r *http.Request, defaultLimit int) ItemQuery {
	values := r.URL.Query()
	limit := parseInt(values.Get("limit"), defaultLimit)
	offset := parseInt(values.Get("offset"), 0)
	feedID, _ := strconv.ParseInt(values.Get("feed_id"), 10, 64)
	return ItemQuery{
		FeedID:   feedID,
		Category: values.Get("category"),
		Status:   values.Get("status"),
		Range:    values.Get("range"),
		Search:   strings.TrimSpace(values.Get("q")),
		Timezone: values.Get("timezone"),
		Limit:    limit,
		Offset:   offset,
	}
}

func parseInt(value string, fallback int) int {
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func baseURL(r *http.Request) string {
	scheme := r.Header.Get("X-Forwarded-Proto")
	if scheme == "" {
		scheme = "http"
		if r.TLS != nil {
			scheme = "https"
		}
	}
	return scheme + "://" + r.Host
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeError(w http.ResponseWriter, status int, err error) {
	writeJSON(w, status, map[string]any{"error": err.Error()})
}

func methodNotAllowed(w http.ResponseWriter) {
	writeError(w, http.StatusMethodNotAllowed, errors.New("method not allowed"))
}
