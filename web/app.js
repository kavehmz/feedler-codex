(function () {
  const state = {
    folders: [],
    feeds: [],
    items: [],
    settings: null,
    scope: { type: "all", id: 0, label: "All Articles" },
    filter: "",
    selectedId: 0,
    query: "",
    pendingRead: new Set(),
  };

  const els = {};

  document.addEventListener("DOMContentLoaded", init);

  function init() {
    [
      "syncStatus",
      "feedTree",
      "allScope",
      "allUnread",
      "scopeTitle",
      "scopeMeta",
      "filterUnread",
      "filterAll",
      "refreshButton",
      "markAllButton",
      "exportButton",
      "addFeedButton",
      "settingsButton",
      "shortcutButton",
      "itemList",
      "itemCount",
      "articlePanel",
      "searchInput",
      "toast",
      "modalBackdrop",
      "modalTitle",
      "modalBody",
      "modalClose",
    ].forEach((id) => {
      els[id] = document.getElementById(id);
    });

    els.allScope.addEventListener("click", () => setScope("all", 0, "All Articles"));
    els.feedTree.addEventListener("click", handleTreeClick);
    els.filterUnread.addEventListener("click", () => setFilter("unread"));
    els.filterAll.addEventListener("click", () => setFilter("all"));
    els.refreshButton.addEventListener("click", refreshScope);
    els.markAllButton.addEventListener("click", markSelectedScopeRead);
    els.exportButton.addEventListener("click", showExportModal);
    els.addFeedButton.addEventListener("click", showAddFeedModal);
    els.settingsButton.addEventListener("click", showSettingsModal);
    els.shortcutButton.addEventListener("click", showShortcutsModal);
    els.itemList.addEventListener("click", handleItemClick);
    els.itemList.addEventListener("scroll", debounce(handleAutoMarkOnScroll, 160));
    els.searchInput.addEventListener("input", () => {
      state.query = els.searchInput.value.trim().toLowerCase();
      renderItems();
    });
    els.modalClose.addEventListener("click", closeModal);
    els.modalBackdrop.addEventListener("click", (event) => {
      if (event.target === els.modalBackdrop) closeModal();
    });
    document.addEventListener("keydown", handleKeyboard);

    const itemParam = new URLSearchParams(location.search).get("item");
    if (itemParam) state.selectedId = Number(itemParam) || 0;
    loadState();
  }

  async function api(path, options) {
    const response = await fetch(path, {
      ...options,
      headers: {
        "Content-Type": "application/json",
        ...(options && options.headers ? options.headers : {}),
      },
    });
    if (!response.ok) {
      let message = response.statusText;
      try {
        const payload = await response.json();
        message = payload.error || message;
      } catch (_) {
        message = await response.text();
      }
      throw new Error(message);
    }
    const contentType = response.headers.get("Content-Type") || "";
    if (contentType.includes("application/json")) return response.json();
    return response.text();
  }

  async function loadState() {
    setStatus("Loading");
    try {
      const params = new URLSearchParams({
        scope: state.scope.type,
        id: String(state.scope.id || 0),
        filter: state.filter || "",
      });
      const payload = await api("/api/state?" + params.toString());
      state.folders = payload.folders || [];
      state.feeds = payload.feeds || [];
      state.items = payload.items || [];
      state.settings = payload.settings;
      if (!state.filter) state.filter = state.settings.default_filter || "unread";
      document.body.classList.toggle("compact", state.settings.list_density === "compact");
      if (!state.selectedId || !state.items.some((item) => item.id === state.selectedId)) {
        state.selectedId = state.items.length ? state.items[0].id : 0;
      }
      render();
      setStatus("Ready");
    } catch (error) {
      setStatus("Error");
      toast(error.message);
    }
  }

  function render() {
    renderSidebar();
    renderTopbar();
    renderItems();
    renderArticle();
  }

  function renderSidebar() {
    const totalUnread = state.feeds.reduce((sum, feed) => sum + feed.unread, 0);
    els.allUnread.textContent = String(totalUnread);
    els.allScope.classList.toggle("active", state.scope.type === "all");

    const feedsByFolder = new Map();
    state.feeds.forEach((feed) => {
      const key = feed.folder_id || 0;
      if (!feedsByFolder.has(key)) feedsByFolder.set(key, []);
      feedsByFolder.get(key).push(feed);
    });

    const parts = [];
    const unfiled = feedsByFolder.get(0) || [];
    if (unfiled.length) {
      parts.push(`<div class="folder-row"><span class="folder-title">Unfiled</span><span class="count">${folderUnread(unfiled)}</span></div>`);
      unfiled.forEach((feed) => parts.push(feedRow(feed)));
    }

    state.folders.forEach((folder) => {
      const active = state.scope.type === "folder" && state.scope.id === folder.id ? " active" : "";
      parts.push(`
        <button class="folder-row${active}" data-folder-id="${folder.id}" title="${escapeAttr(folder.name)}">
          <span class="folder-title">${escapeHTML(folder.name)}</span>
          <span class="count">${folder.unread}</span>
        </button>
      `);
      (feedsByFolder.get(folder.id) || []).forEach((feed) => parts.push(feedRow(feed)));
    });
    els.feedTree.innerHTML = parts.join("");
  }

  function feedRow(feed) {
    const active = state.scope.type === "feed" && state.scope.id === feed.id ? " active" : "";
    const error = feed.last_error ? `<button class="mini-button error-pill" data-retry-feed="${feed.id}" title="${escapeAttr(feed.last_error)}">Retry</button>` : `<span></span>`;
    return `
      <div class="feed-row${active}" data-feed-id="${feed.id}" role="button" tabindex="0" title="${escapeAttr(feed.title)}">
        <span class="feed-title">${escapeHTML(feed.title)}</span>
        <span class="count">${feed.unread}</span>
        ${error}
        <button class="mini-button" data-edit-feed="${feed.id}">Edit</button>
      </div>
    `;
  }

  function renderTopbar() {
    els.scopeTitle.textContent = state.scope.label;
    const total = state.items.length;
    const unread = state.items.filter((item) => !item.read_at).length;
    els.scopeMeta.textContent = `${total} shown, ${unread} unread`;
    els.filterUnread.classList.toggle("active", state.filter === "unread");
    els.filterAll.classList.toggle("active", state.filter === "all");
  }

  function renderItems() {
    const items = filteredItems();
    els.itemCount.textContent = String(items.length);
    if (!items.length) {
      els.itemList.innerHTML = `<div class="empty-state">No articles</div>`;
      return;
    }
    els.itemList.innerHTML = items
      .map((item) => {
        const active = item.id === state.selectedId ? " active" : "";
        const unread = item.read_at ? "" : " unread";
        return `
          <button class="item-row${active}${unread}" data-item-id="${item.id}">
            <span class="item-title">${escapeHTML(item.title)}</span>
            <span class="item-meta">${escapeHTML(item.feed_title)}${item.published_at ? " - " + formatDate(item.published_at) : ""}</span>
            <span class="item-excerpt">${escapeHTML(articleExcerpt(item))}</span>
          </button>
        `;
      })
      .join("");
  }

  function renderArticle() {
    const item = state.items.find((candidate) => candidate.id === state.selectedId);
    if (!item) {
      els.articlePanel.innerHTML = `<div class="empty-state">Select an article</div>`;
      return;
    }
    const body = articleText(item);
    els.articlePanel.innerHTML = `
      <div class="article-header">
        <div class="article-meta">${escapeHTML(item.feed_title)}${item.published_at ? " - " + formatDateTime(item.published_at) : ""}</div>
        <h2>${escapeHTML(item.title)}</h2>
        <div class="article-actions">
          ${item.link ? `<button class="primary" id="openOriginalButton">Open Original</button>` : ""}
          <button id="toggleReadButton">${item.read_at ? "Mark Unread" : "Mark Read"}</button>
          <button id="copyReaderLinkButton">Copy Feedler Link</button>
        </div>
      </div>
      <div class="article-body">${escapeHTML(body || "No article text available.")}</div>
    `;
    const original = document.getElementById("openOriginalButton");
    if (original) original.addEventListener("click", () => window.open(item.link, "_blank", "noopener"));
    document.getElementById("toggleReadButton").addEventListener("click", () => markItemRead(item.id, !item.read_at));
    document.getElementById("copyReaderLinkButton").addEventListener("click", () => {
      navigator.clipboard.writeText(location.origin + item.reader_url);
      toast("Reader link copied");
    });
  }

  function handleTreeClick(event) {
    const retry = event.target.closest("[data-retry-feed]");
    if (retry) {
      event.stopPropagation();
      retryFeed(Number(retry.dataset.retryFeed));
      return;
    }
    const edit = event.target.closest("[data-edit-feed]");
    if (edit) {
      event.stopPropagation();
      showEditFeedModal(Number(edit.dataset.editFeed));
      return;
    }
    const feed = event.target.closest("[data-feed-id]");
    if (feed) {
      const id = Number(feed.dataset.feedId);
      const model = state.feeds.find((candidate) => candidate.id === id);
      if (model) setScope("feed", id, model.title);
      return;
    }
    const folder = event.target.closest("[data-folder-id]");
    if (folder) {
      const id = Number(folder.dataset.folderId);
      const model = state.folders.find((candidate) => candidate.id === id);
      if (model) setScope("folder", id, model.name);
    }
  }

  function handleItemClick(event) {
    const row = event.target.closest("[data-item-id]");
    if (!row) return;
    selectItem(Number(row.dataset.itemId), true);
  }

  function selectItem(id, markRead) {
    state.selectedId = id;
    history.replaceState(null, "", `/?item=${id}`);
    const item = state.items.find((candidate) => candidate.id === id);
    if (item && markRead && !item.read_at) {
      markItemRead(id, true, { silent: true });
    }
    renderItems();
    renderArticle();
  }

  async function markItemRead(id, read, options) {
    if (state.pendingRead.has(id)) return;
    state.pendingRead.add(id);
    const item = state.items.find((candidate) => candidate.id === id);
    const previous = item ? item.read_at : null;
    if (item) item.read_at = read ? new Date().toISOString() : null;
    render();
    try {
      await api(`/api/items/${id}/read`, {
        method: "PATCH",
        body: JSON.stringify({ read }),
      });
      if (!options || !options.silent) toast(read ? "Marked read" : "Marked unread");
      await loadState();
    } catch (error) {
      if (item) item.read_at = previous;
      toast(error.message);
      render();
    } finally {
      state.pendingRead.delete(id);
    }
  }

  function handleAutoMarkOnScroll() {
    if (!state.settings || !state.settings.auto_mark_on_scroll) return;
    const listTop = els.itemList.getBoundingClientRect().top;
    const rows = els.itemList.querySelectorAll(".item-row.unread");
    rows.forEach((row) => {
      if (row.getBoundingClientRect().bottom < listTop + 2) {
        markItemRead(Number(row.dataset.itemId), true, { silent: true });
      }
    });
  }

  function setScope(type, id, label) {
    state.scope = { type, id, label };
    state.selectedId = 0;
    history.replaceState(null, "", "/");
    loadState();
  }

  function setFilter(filter) {
    state.filter = filter;
    state.selectedId = 0;
    loadState();
  }

  async function refreshScope() {
    setStatus("Refreshing");
    try {
      await api("/api/refresh", {
        method: "POST",
        body: JSON.stringify({ scope: state.scope.type, id: state.scope.id }),
      });
      toast("Refresh complete");
      await loadState();
    } catch (error) {
      toast(error.message);
      await loadState();
    }
  }

  async function retryFeed(id) {
    setStatus("Retrying");
    try {
      await api(`/api/feeds/${id}/refresh`, { method: "POST", body: "{}" });
      toast("Feed refreshed");
    } catch (error) {
      toast(error.message);
    }
    await loadState();
  }

  async function markSelectedScopeRead() {
    try {
      const result = await api("/api/mark-read", {
        method: "POST",
        body: JSON.stringify({ scope: state.scope.type, id: state.scope.id }),
      });
      toast(`${result.count} articles marked read`);
      await loadState();
    } catch (error) {
      toast(error.message);
    }
  }

  function showAddFeedModal() {
    showModal(
      "Add Feed",
      `
        <form class="form-grid" id="addFeedForm">
          <div class="field">
            <label for="feedUrl">Feed URL</label>
            <input id="feedUrl" name="url" type="url" required placeholder="https://example.com/feed.xml" />
          </div>
          <div class="field">
            <label for="feedTitle">Title</label>
            <input id="feedTitle" name="title" placeholder="Optional" />
          </div>
          ${folderSelectHTML("addFolder")}
          <div class="field">
            <label for="newFolder">New folder</label>
            <input id="newFolder" name="newFolder" placeholder="Optional" />
          </div>
          <div class="modal-actions">
            <button type="button" class="subtle" data-close>Cancel</button>
            <button type="submit" class="primary">Add</button>
          </div>
        </form>
      `
    );
    document.getElementById("addFeedForm").addEventListener("submit", async (event) => {
      event.preventDefault();
      const form = new FormData(event.currentTarget);
      try {
        const folderId = await resolveFolderID(form.get("addFolder"), form.get("newFolder"));
        await api("/api/feeds", {
          method: "POST",
          body: JSON.stringify({ url: form.get("url"), title: form.get("title"), folder_id: folderId }),
        });
        closeModal();
        toast("Feed added");
        await loadState();
      } catch (error) {
        toast(error.message);
      }
    });
  }

  function showEditFeedModal(id) {
    const feed = state.feeds.find((candidate) => candidate.id === id);
    if (!feed) return;
    showModal(
      "Edit Feed",
      `
        <form class="form-grid" id="editFeedForm">
          <div class="field">
            <label for="editFeedTitle">Title</label>
            <input id="editFeedTitle" name="title" required value="${escapeAttr(feed.title)}" />
          </div>
          <div class="field">
            <label>URL</label>
            <input value="${escapeAttr(feed.url)}" readonly />
          </div>
          ${folderSelectHTML("editFolder", feed.folder_id || 0)}
          <div class="field">
            <label for="editNewFolder">New folder</label>
            <input id="editNewFolder" name="newFolder" placeholder="Optional" />
          </div>
          ${feed.last_error ? `<div class="form-note">Last error: ${escapeHTML(feed.last_error)}</div>` : ""}
          <div class="modal-actions">
            <button type="button" class="danger" id="deleteFeedButton">Delete</button>
            <button type="button" id="retryFeedButton">Retry</button>
            <button type="button" class="subtle" data-close>Cancel</button>
            <button type="submit" class="primary">Save</button>
          </div>
        </form>
      `
    );
    document.getElementById("editFeedForm").addEventListener("submit", async (event) => {
      event.preventDefault();
      const form = new FormData(event.currentTarget);
      try {
        const folderId = await resolveFolderID(form.get("editFolder"), form.get("newFolder"));
        await api(`/api/feeds/${id}`, {
          method: "PATCH",
          body: JSON.stringify({ title: form.get("title"), folder_id: folderId }),
        });
        closeModal();
        await loadState();
      } catch (error) {
        toast(error.message);
      }
    });
    document.getElementById("deleteFeedButton").addEventListener("click", async () => {
      if (!confirm("Delete this feed and its articles?")) return;
      try {
        await api(`/api/feeds/${id}`, { method: "DELETE", body: "{}" });
        closeModal();
        if (state.scope.type === "feed" && state.scope.id === id) setScope("all", 0, "All Articles");
        else await loadState();
      } catch (error) {
        toast(error.message);
      }
    });
    document.getElementById("retryFeedButton").addEventListener("click", () => retryFeed(id));
  }

  function showSettingsModal() {
    const settings = state.settings || {};
    const browserTZ = Intl.DateTimeFormat().resolvedOptions().timeZone || "Europe/Berlin";
    showModal(
      "Settings",
      `
        <form class="form-grid" id="settingsForm">
          <label class="checkbox-row">
            <input type="checkbox" name="autoMark" ${settings.auto_mark_on_scroll ? "checked" : ""} />
            <span>Mark unread articles as read when they scroll above the article list</span>
          </label>
          <div class="field">
            <label for="density">List density</label>
            <select id="density" name="density">
              <option value="comfortable" ${settings.list_density !== "compact" ? "selected" : ""}>Comfortable</option>
              <option value="compact" ${settings.list_density === "compact" ? "selected" : ""}>Compact</option>
            </select>
          </div>
          <div class="field">
            <label for="defaultFilter">Default filter</label>
            <select id="defaultFilter" name="defaultFilter">
              <option value="unread" ${settings.default_filter !== "all" ? "selected" : ""}>Unread</option>
              <option value="all" ${settings.default_filter === "all" ? "selected" : ""}>All</option>
            </select>
          </div>
          <div class="field">
            <label for="timezone">Timezone</label>
            <input id="timezone" name="timezone" value="${escapeAttr(settings.timezone || browserTZ)}" />
          </div>
          <div class="modal-actions">
            <button type="button" class="subtle" data-close>Cancel</button>
            <button type="submit" class="primary">Save</button>
          </div>
        </form>
      `
    );
    document.getElementById("settingsForm").addEventListener("submit", async (event) => {
      event.preventDefault();
      const form = new FormData(event.currentTarget);
      try {
        await api("/api/settings", {
          method: "PATCH",
          body: JSON.stringify({
            auto_mark_on_scroll: form.get("autoMark") === "on",
            list_density: form.get("density"),
            default_filter: form.get("defaultFilter"),
            timezone: form.get("timezone") || browserTZ,
          }),
        });
        closeModal();
        state.filter = "";
        await loadState();
      } catch (error) {
        toast(error.message);
      }
    });
  }

  function showExportModal() {
    const timezone = Intl.DateTimeFormat().resolvedOptions().timeZone || (state.settings && state.settings.timezone) || "Europe/Berlin";
    showModal(
      "Export",
      `
        <form class="form-grid" id="exportForm">
          <div class="field">
            <label for="exportRange">Range</label>
            <select id="exportRange" name="range">
              <option value="today">Today</option>
              <option value="week">This week</option>
            </select>
          </div>
          <div class="field">
            <label for="exportTimezone">Timezone</label>
            <input id="exportTimezone" name="timezone" value="${escapeAttr(timezone)}" />
          </div>
          <div class="modal-actions">
            <button type="button" class="subtle" data-close>Cancel</button>
            <button type="submit" class="primary">Download Markdown</button>
          </div>
        </form>
      `
    );
    document.getElementById("exportForm").addEventListener("submit", (event) => {
      event.preventDefault();
      const form = new FormData(event.currentTarget);
      const params = new URLSearchParams({
        scope: state.scope.type,
        id: String(state.scope.id || 0),
        range: form.get("range"),
        timezone: form.get("timezone") || timezone,
      });
      closeModal();
      window.location.href = "/api/export?" + params.toString();
    });
  }

  function showShortcutsModal() {
    showModal(
      "Shortcuts",
      `
        <div class="shortcut-table">
          <span class="kbd">?</span><span>Show shortcuts</span>
          <span class="kbd">j</span><span>Next article</span>
          <span class="kbd">k</span><span>Previous article</span>
          <span class="kbd">m</span><span>Toggle read</span>
          <span class="kbd">Shift M</span><span>Mark selection read</span>
          <span class="kbd">r</span><span>Refresh selection</span>
          <span class="kbd">e</span><span>Export selection</span>
          <span class="kbd">a</span><span>Add feed</span>
          <span class="kbd">s</span><span>Settings</span>
          <span class="kbd">/</span><span>Search</span>
          <span class="kbd">Enter</span><span>Open original</span>
        </div>
      `
    );
  }

  function showModal(title, bodyHTML) {
    els.modalTitle.textContent = title;
    els.modalBody.innerHTML = bodyHTML;
    els.modalBackdrop.hidden = false;
    els.modalBody.querySelectorAll("[data-close]").forEach((button) => button.addEventListener("click", closeModal));
    const first = els.modalBody.querySelector("input, select, button");
    if (first) setTimeout(() => first.focus(), 20);
  }

  function closeModal() {
    els.modalBackdrop.hidden = true;
    els.modalBody.innerHTML = "";
  }

  function folderSelectHTML(name, selected) {
    const options = [`<option value="0">No folder</option>`]
      .concat(
        state.folders.map((folder) => {
          const isSelected = Number(selected || 0) === folder.id ? "selected" : "";
          return `<option value="${folder.id}" ${isSelected}>${escapeHTML(folder.name)}</option>`;
        })
      )
      .join("");
    return `
      <div class="field">
        <label>Folder</label>
        <select name="${name}">${options}</select>
      </div>
    `;
  }

  async function resolveFolderID(selected, newName) {
    const name = String(newName || "").trim();
    if (name) {
      const result = await api("/api/folders", {
        method: "POST",
        body: JSON.stringify({ name }),
      });
      return result.id;
    }
    const id = Number(selected || 0);
    return id ? id : null;
  }

  function handleKeyboard(event) {
    const tag = event.target.tagName;
    const typing = tag === "INPUT" || tag === "TEXTAREA" || tag === "SELECT";
    if (typing && event.key !== "Escape") return;
    if (event.key === "Escape") {
      closeModal();
      return;
    }
    if (!els.modalBackdrop.hidden) return;
    if (event.key === "?") {
      showShortcutsModal();
      event.preventDefault();
      return;
    }
    if (event.key === "/") {
      els.searchInput.focus();
      event.preventDefault();
      return;
    }
    if (event.key === "j") {
      moveSelection(1);
      event.preventDefault();
      return;
    }
    if (event.key === "k") {
      moveSelection(-1);
      event.preventDefault();
      return;
    }
    if (event.key === "m" && event.shiftKey) {
      markSelectedScopeRead();
      event.preventDefault();
      return;
    }
    if (event.key === "m") {
      const item = state.items.find((candidate) => candidate.id === state.selectedId);
      if (item) markItemRead(item.id, !item.read_at);
      event.preventDefault();
      return;
    }
    if (event.key === "r") {
      refreshScope();
      event.preventDefault();
      return;
    }
    if (event.key === "e") {
      showExportModal();
      event.preventDefault();
      return;
    }
    if (event.key === "a") {
      showAddFeedModal();
      event.preventDefault();
      return;
    }
    if (event.key === "s") {
      showSettingsModal();
      event.preventDefault();
      return;
    }
    if (event.key === "Enter") {
      const item = state.items.find((candidate) => candidate.id === state.selectedId);
      if (item && item.link) window.open(item.link, "_blank", "noopener");
    }
  }

  function moveSelection(delta) {
    const items = filteredItems();
    if (!items.length) return;
    const current = items.findIndex((item) => item.id === state.selectedId);
    const next = Math.max(0, Math.min(items.length - 1, (current < 0 ? 0 : current) + delta));
    selectItem(items[next].id, true);
    const row = els.itemList.querySelector(`[data-item-id="${items[next].id}"]`);
    if (row) row.scrollIntoView({ block: "nearest" });
  }

  function filteredItems() {
    if (!state.query) return state.items;
    return state.items.filter((item) => {
      const haystack = `${item.title} ${item.feed_title} ${articleExcerpt(item)}`.toLowerCase();
      return haystack.includes(state.query);
    });
  }

  function folderUnread(feeds) {
    return feeds.reduce((sum, feed) => sum + feed.unread, 0);
  }

  function articleText(item) {
    return stripTags(item.content || item.summary || "");
  }

  function articleExcerpt(item) {
    const text = stripTags(item.summary || item.content || "");
    return text.length > 180 ? text.slice(0, 180) + "..." : text;
  }

  function stripTags(value) {
    const div = document.createElement("div");
    div.innerHTML = value || "";
    div.querySelectorAll("script, style, iframe, object").forEach((node) => node.remove());
    return (div.textContent || "").replace(/\s+/g, " ").trim();
  }

  function formatDate(value) {
    return new Intl.DateTimeFormat(undefined, { month: "short", day: "numeric" }).format(new Date(value));
  }

  function formatDateTime(value) {
    return new Intl.DateTimeFormat(undefined, {
      dateStyle: "medium",
      timeStyle: "short",
    }).format(new Date(value));
  }

  function setStatus(value) {
    els.syncStatus.textContent = value;
  }

  function toast(message) {
    els.toast.textContent = message;
    els.toast.hidden = false;
    clearTimeout(toast.timer);
    toast.timer = setTimeout(() => {
      els.toast.hidden = true;
    }, 3200);
  }

  function debounce(fn, wait) {
    let timer = 0;
    return function (...args) {
      clearTimeout(timer);
      timer = setTimeout(() => fn.apply(this, args), wait);
    };
  }

  function escapeHTML(value) {
    return String(value == null ? "" : value)
      .replaceAll("&", "&amp;")
      .replaceAll("<", "&lt;")
      .replaceAll(">", "&gt;")
      .replaceAll('"', "&quot;")
      .replaceAll("'", "&#039;");
  }

  function escapeAttr(value) {
    return escapeHTML(value);
  }
})();
