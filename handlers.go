package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

func (a *App) handleState(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, errors.New("method not allowed"))
		return
	}
	ctx := r.Context()
	settings, err := a.GetSettings(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	scope := r.URL.Query().Get("scope")
	scopeID, _ := strconv.ParseInt(r.URL.Query().Get("id"), 10, 64)
	filter := r.URL.Query().Get("filter")
	if filter == "" {
		filter = settings.DefaultFilter
	}

	folders, err := a.ListFolders(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	feeds, err := a.ListFeeds(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	items, err := a.ListItems(ctx, scope, scopeID, filter, 500)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, StateResponse{Folders: folders, Feeds: feeds, Items: items, Settings: settings})
}

func (a *App) handleFeeds(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		var req struct {
			URL      string `json:"url"`
			Title    string `json:"title"`
			FolderID *int64 `json:"folder_id"`
		}
		if err := readJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		id, err := a.CreateFeed(r.Context(), req.Title, req.URL, "", req.FolderID)
		if err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			if err := a.RefreshFeed(ctx, id); err != nil {
				fmt.Printf("refresh new feed: %v\n", err)
			}
		}()
		writeJSON(w, http.StatusCreated, map[string]any{"id": id})
	default:
		writeError(w, http.StatusMethodNotAllowed, errors.New("method not allowed"))
	}
}

func (a *App) handleFeedPath(w http.ResponseWriter, r *http.Request) {
	id, tail, err := parsePathID(r.URL.Path, "/api/feeds/")
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if tail == "refresh" {
		if r.Method != http.MethodPost {
			writeError(w, http.StatusMethodNotAllowed, errors.New("method not allowed"))
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
		defer cancel()
		if err := a.RefreshFeed(ctx, id); err != nil {
			writeError(w, http.StatusBadGateway, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
		return
	}

	switch r.Method {
	case http.MethodPatch:
		var req struct {
			Title    string `json:"title"`
			FolderID *int64 `json:"folder_id"`
		}
		if err := readJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		if err := a.UpdateFeed(r.Context(), id, req.Title, req.FolderID); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	case http.MethodDelete:
		if err := a.DeleteFeed(r.Context(), id); err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	default:
		writeError(w, http.StatusMethodNotAllowed, errors.New("method not allowed"))
	}
}

func (a *App) handleFolders(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, errors.New("method not allowed"))
		return
	}
	var req struct {
		Name string `json:"name"`
	}
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	id, err := a.EnsureFolder(r.Context(), req.Name)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if id == nil {
		writeError(w, http.StatusBadRequest, errors.New("folder name is required"))
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"id": *id})
}

func (a *App) handleFolderPath(w http.ResponseWriter, r *http.Request) {
	id, _, err := parsePathID(r.URL.Path, "/api/folders/")
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	switch r.Method {
	case http.MethodPatch:
		var req struct {
			Name string `json:"name"`
		}
		if err := readJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		if err := a.UpdateFolder(r.Context(), id, req.Name); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	case http.MethodDelete:
		if err := a.DeleteFolder(r.Context(), id); err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	default:
		writeError(w, http.StatusMethodNotAllowed, errors.New("method not allowed"))
	}
}

func (a *App) handleItemPath(w http.ResponseWriter, r *http.Request) {
	id, tail, err := parsePathID(r.URL.Path, "/api/items/")
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if tail != "read" || r.Method != http.MethodPatch {
		writeError(w, http.StatusMethodNotAllowed, errors.New("method not allowed"))
		return
	}
	var req struct {
		Read bool `json:"read"`
	}
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if err := a.MarkItemRead(r.Context(), id, req.Read); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (a *App) handleMarkScopeRead(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, errors.New("method not allowed"))
		return
	}
	var req struct {
		Scope string `json:"scope"`
		ID    int64  `json:"id"`
	}
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	count, err := a.MarkScopeRead(r.Context(), req.Scope, req.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "count": count})
}

func (a *App) handleRefresh(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, errors.New("method not allowed"))
		return
	}
	var req struct {
		Scope string `json:"scope"`
		ID    int64  `json:"id"`
	}
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Minute)
	defer cancel()
	if err := a.RefreshScope(ctx, req.Scope, req.ID); err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (a *App) handleSettings(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		settings, err := a.GetSettings(r.Context())
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, http.StatusOK, settings)
	case http.MethodPatch:
		var settings Settings
		if err := readJSON(r, &settings); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		if err := a.SaveSettings(r.Context(), settings); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	default:
		writeError(w, http.StatusMethodNotAllowed, errors.New("method not allowed"))
	}
}

func (a *App) handleExport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, errors.New("method not allowed"))
		return
	}
	scope := r.URL.Query().Get("scope")
	scopeID, _ := strconv.ParseInt(r.URL.Query().Get("id"), 10, 64)
	rangeName := r.URL.Query().Get("range")
	if rangeName == "" {
		rangeName = "today"
	}
	tz := strings.TrimSpace(r.URL.Query().Get("timezone"))
	if tz == "" {
		settings, _ := a.GetSettings(r.Context())
		tz = settings.Timezone
	}

	location, err := time.LoadLocation(tz)
	if err != nil {
		writeError(w, http.StatusBadRequest, fmt.Errorf("unknown timezone %q", tz))
		return
	}

	start, end, title := exportWindow(time.Now().In(location), rangeName)
	items, err := a.ExportItems(r.Context(), scope, scopeID, start.UTC(), end.UTC())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	md := renderMarkdownExport(title, scopeLabel(scope, scopeID), location, start, end, items, r)
	filename := strings.ToLower(strings.ReplaceAll(title, " ", "-")) + ".md"
	w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="`+filename+`"`)
	_, _ = w.Write([]byte(md))
}
