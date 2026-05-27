const defaultSettings = {
  markReadOnScroll: false,
  density: "comfortable",
  defaultStatus: "unread",
  defaultRange: "all",
};

const state = {
  feeds: [],
  items: [],
  selectedItem: null,
  category: "all",
  feedId: null,
  status: "unread",
  range: "all",
  search: "",
  refreshTimer: null,
  scrollReadTimer: null,
  settings: loadSettings(),
};

state.status = state.settings.defaultStatus;
state.range = state.settings.defaultRange;

const els = {
  feedSummary: document.querySelector("#feedSummary"),
  refreshButton: document.querySelector("#refreshButton"),
  manageFeedsButton: document.querySelector("#manageFeedsButton"),
  settingsButton: document.querySelector("#settingsButton"),
  shortcutsButton: document.querySelector("#shortcutsButton"),
  refreshStatus: document.querySelector("#refreshStatus"),
  opmlInput: document.querySelector("#opmlInput"),
  allCategoriesButton: document.querySelector("#allCategoriesButton"),
  allUnreadCount: document.querySelector("#allUnreadCount"),
  categoryList: document.querySelector("#categoryList"),
  feedList: document.querySelector("#feedList"),
  searchInput: document.querySelector("#searchInput"),
  rangeSelect: document.querySelector("#rangeSelect"),
  resultLine: document.querySelector("#resultLine"),
  itemList: document.querySelector("#itemList"),
  readerPane: document.querySelector("#readerPane"),
  markSelectionReadButton: document.querySelector("#markSelectionReadButton"),
  exportButton: document.querySelector("#exportButton"),
  exportModal: document.querySelector("#exportModal"),
  closeExportButton: document.querySelector("#closeExportButton"),
  exportScope: document.querySelector("#exportScope"),
  exportPeriod: document.querySelector("#exportPeriod"),
  exportStatus: document.querySelector("#exportStatus"),
  exportText: document.querySelector("#exportText"),
  copyExportButton: document.querySelector("#copyExportButton"),
  downloadExportButton: document.querySelector("#downloadExportButton"),
  manageModal: document.querySelector("#manageModal"),
  closeManageButton: document.querySelector("#closeManageButton"),
  addFeedForm: document.querySelector("#addFeedForm"),
  newFeedURL: document.querySelector("#newFeedURL"),
  newFeedTitle: document.querySelector("#newFeedTitle"),
  newFeedCategory: document.querySelector("#newFeedCategory"),
  feedManagerList: document.querySelector("#feedManagerList"),
  settingsModal: document.querySelector("#settingsModal"),
  closeSettingsButton: document.querySelector("#closeSettingsButton"),
  scrollReadSetting: document.querySelector("#scrollReadSetting"),
  densitySetting: document.querySelector("#densitySetting"),
  defaultStatusSetting: document.querySelector("#defaultStatusSetting"),
  defaultRangeSetting: document.querySelector("#defaultRangeSetting"),
  shortcutsModal: document.querySelector("#shortcutsModal"),
  closeShortcutsButton: document.querySelector("#closeShortcutsButton"),
};

async function api(path, options = {}) {
  const headers = options.body instanceof FormData ? {} : { "Content-Type": "application/json" };
  const response = await fetch(path, { ...options, headers: { ...headers, ...(options.headers || {}) } });
  if (!response.ok) {
    let message = `${response.status} ${response.statusText}`;
    try {
      const payload = await response.json();
      message = payload.error || message;
    } catch (_) {
      message = await response.text();
    }
    throw new Error(message);
  }
  const contentType = response.headers.get("content-type") || "";
  if (contentType.includes("application/json")) {
    return response.json();
  }
  return response.text();
}

async function loadFeeds() {
  const payload = await api("/api/feeds");
  state.feeds = payload.feeds || [];
  renderFeeds();
  renderFeedManager();
}

async function loadItems() {
  const params = new URLSearchParams();
  params.set("status", state.status);
  params.set("range", state.range);
  params.set("limit", "120");
  params.set("timezone", userTimezone());
  applyScopeToParams(params);
  if (state.search) params.set("q", state.search);

  const payload = await api(`/api/items?${params}`);
  state.items = payload.items || [];
  renderItems();

  if (state.selectedItem && !state.items.some((item) => item.id === state.selectedItem.id)) {
    return;
  }
  if (!state.selectedItem && state.items.length > 0) {
    renderReader(null);
  }
}

function renderFeeds() {
  const totalUnread = state.feeds.reduce((sum, feed) => sum + feed.unread_count, 0);
  const totalItems = state.feeds.reduce((sum, feed) => sum + feed.total_count, 0);
  els.feedSummary.textContent = `${state.feeds.length} feeds, ${totalUnread} unread`;
  els.allUnreadCount.textContent = totalUnread;
  els.allCategoriesButton.classList.toggle("active", state.category === "all" && !state.feedId);

  const categories = new Map();
  for (const feed of state.feeds) {
    const current = categories.get(feed.category) || { unread: 0, total: 0 };
    current.unread += feed.unread_count;
    current.total += feed.total_count;
    categories.set(feed.category, current);
  }

  els.categoryList.replaceChildren(
    ...Array.from(categories.entries()).map(([category, counts]) => {
      const button = document.createElement("button");
      button.className = "nav-row";
      button.type = "button";
      button.classList.toggle("active", state.category === category && !state.feedId);
      button.innerHTML = `<span></span><span></span>`;
      button.children[0].textContent = category;
      button.children[1].textContent = counts.unread;
      button.addEventListener("click", () => selectScope({ category, feedId: null }));
      return button;
    }),
  );

  els.feedList.replaceChildren(
    ...state.feeds.map((feed) => {
      const button = document.createElement("button");
      button.className = "feed-row";
      button.type = "button";
      button.classList.toggle("active", state.feedId === feed.id);
      button.classList.toggle("error", Boolean(feed.last_error));
      button.title = feed.last_error ? `${feed.title}\n${feed.last_error}` : feed.title;
      button.innerHTML = `<span></span><span></span>`;
      button.children[0].textContent = `${feed.last_error ? "! " : ""}${feed.title}`;
      button.children[1].textContent = feed.unread_count;
      button.addEventListener("click", () => selectScope({ category: "all", feedId: feed.id }));
      return button;
    }),
  );

  if (totalItems === 0) {
    els.resultLine.textContent = "Refreshing imported feeds";
  }
}

function renderItems() {
  els.resultLine.textContent = `${state.items.length} item${state.items.length === 1 ? "" : "s"} in ${scopeLabel()}`;
  if (state.items.length === 0) {
    const empty = document.createElement("div");
    empty.className = "item-row read";
    empty.innerHTML = `<span></span><div><h3 class="item-title">No items found</h3><p class="item-summary">Refresh feeds or change the current filter.</p></div>`;
    els.itemList.replaceChildren(empty);
    return;
  }

  els.itemList.replaceChildren(
    ...state.items.map((item) => {
      const button = document.createElement("button");
      button.className = "item-row";
      button.type = "button";
      button.dataset.itemId = String(item.id);
      button.classList.toggle("read", item.read);
      button.classList.toggle("active", state.selectedItem?.id === item.id);
      button.innerHTML = `
        <span class="unread-dot"></span>
        <div>
          <h3 class="item-title"></h3>
          <p class="item-meta"></p>
          <p class="item-summary"></p>
        </div>
      `;
      button.querySelector(".item-title").textContent = item.title;
      button.querySelector(".item-meta").textContent = `${item.feed_title} · ${formatDate(item.published_at)}`;
      button.querySelector(".item-summary").textContent = item.summary || item.content || "";
      button.addEventListener("click", () => selectItem(item.id));
      return button;
    }),
  );
}

function renderFeedManager() {
  if (!els.feedManagerList) return;
  els.feedManagerList.replaceChildren(
    ...state.feeds.map((feed) => {
      const row = document.createElement("div");
      row.className = "feed-manage-row";
      row.dataset.feedId = String(feed.id);
      row.innerHTML = `
        <div class="feed-manage-main">
          <input class="manage-title" type="text" />
          <input class="manage-category" type="text" />
          <div class="feed-url"></div>
          <div class="feed-error"></div>
        </div>
        <div class="feed-manage-actions">
          <button class="button secondary save-feed" type="button">Save</button>
          <button class="button secondary retry-feed" type="button">Retry</button>
          <button class="button danger delete-feed" type="button">Delete</button>
        </div>
      `;
      row.querySelector(".manage-title").value = feed.title;
      row.querySelector(".manage-category").value = feed.category;
      row.querySelector(".feed-url").textContent = feed.feed_url;
      row.querySelector(".feed-error").textContent = feed.last_error ? `Error: ${feed.last_error}` : "";
      row.querySelector(".save-feed").addEventListener("click", () => saveManagedFeed(feed.id, row));
      row.querySelector(".retry-feed").addEventListener("click", () => retryFeed(feed.id));
      row.querySelector(".delete-feed").addEventListener("click", () => deleteFeed(feed.id, feed.title));
      return row;
    }),
  );
}

function renderReader(item) {
  if (!item) {
    els.readerPane.innerHTML = `
      <div class="empty-reader">
        <h2>Select a story</h2>
        <p>Open an item from the list to read it, mark it, or jump to the original source.</p>
      </div>`;
    return;
  }

  els.readerPane.innerHTML = `
    <header class="article-header">
      <div class="article-kicker"></div>
      <h2></h2>
      <div class="article-meta"></div>
      <div class="article-actions">
        <a class="button primary" target="_blank" rel="noreferrer">Open original</a>
        <button class="button secondary" id="toggleReadButton" type="button"></button>
      </div>
    </header>
    <div class="article-body"></div>
  `;
  els.readerPane.querySelector(".article-kicker").textContent = item.feed_title;
  els.readerPane.querySelector("h2").textContent = item.title;
  els.readerPane.querySelector(".article-meta").textContent = [item.feed_category, formatDate(item.published_at), item.author]
    .filter(Boolean)
    .join(" · ");
  const original = els.readerPane.querySelector("a");
  original.href = item.link || "#";
  original.classList.toggle("disabled", !item.link);

  const toggle = els.readerPane.querySelector("#toggleReadButton");
  toggle.textContent = item.read ? "Mark unread" : "Mark read";
  toggle.addEventListener("click", () => toggleSelectedRead());

  els.readerPane.querySelector(".article-body").textContent = item.content || item.summary || "No article text was provided by this feed.";
}

function selectScope({ category, feedId }) {
  state.category = category;
  state.feedId = feedId;
  loadItems();
  renderFeeds();
}

async function selectItem(id) {
  let item = state.items.find((candidate) => candidate.id === id);
  if (!item) {
    const payload = await api(`/api/items/${id}`);
    item = payload.item;
  }

  state.selectedItem = item;
  renderReader(item);
  updateURLItem(item.id);

  if (!item.read) {
    await setItemRead(item.id, true);
  } else {
    renderItems();
  }
}

async function setItemRead(id, read) {
  const payload = await api(`/api/items/${id}`, {
    method: "PATCH",
    body: JSON.stringify({ read }),
  });
  state.selectedItem = payload.item;
  state.items = state.items.map((candidate) => (candidate.id === id ? payload.item : candidate));
  renderReader(payload.item);
  renderItems();
  await loadFeeds();
}

async function toggleSelectedRead() {
  if (!state.selectedItem) return;
  await setItemRead(state.selectedItem.id, !state.selectedItem.read);
}

async function refreshFeeds() {
  await api("/api/refresh", { method: "POST", body: JSON.stringify({}) });
  pollRefresh(true);
}

async function pollRefresh(force = false) {
  const status = await api("/api/refresh");
  renderRefreshStatus(status);
  if (status.running && !state.refreshTimer) {
    state.refreshTimer = window.setInterval(async () => {
      const next = await api("/api/refresh");
      renderRefreshStatus(next);
      if (!next.running) {
        window.clearInterval(state.refreshTimer);
        state.refreshTimer = null;
        await loadFeeds();
        await loadItems();
      }
    }, 1600);
  }
  if (force && !status.running) {
    await loadFeeds();
    await loadItems();
  }
}

function renderRefreshStatus(status) {
  els.refreshStatus.classList.toggle("running", Boolean(status.running));
  if (status.running) {
    els.refreshStatus.textContent = `Refreshing ${status.done || 0}/${status.total || "?"} feeds`;
    return;
  }
  if (status.finished_at) {
    const suffix = status.errors?.length ? `, ${status.errors.length} errors` : "";
    els.refreshStatus.textContent = `Last refresh: ${status.items || 0} items${suffix}`;
    return;
  }
  els.refreshStatus.textContent = "Idle";
}

async function importOPML(file) {
  const body = new FormData();
  body.append("opml", file);
  await api("/api/import", { method: "POST", body });
  pollRefresh(true);
  await loadFeeds();
}

async function addFeed(event) {
  event.preventDefault();
  const payload = await api("/api/feeds", {
    method: "POST",
    body: JSON.stringify({
      feed_url: els.newFeedURL.value.trim(),
      title: els.newFeedTitle.value.trim(),
      category: els.newFeedCategory.value.trim() || "Uncategorized",
    }),
  });
  els.addFeedForm.reset();
  await loadFeeds();
  await retryFeed(payload.feed.id);
}

async function saveManagedFeed(id, row) {
  await api(`/api/feeds/${id}`, {
    method: "PATCH",
    body: JSON.stringify({
      title: row.querySelector(".manage-title").value.trim(),
      category: row.querySelector(".manage-category").value.trim(),
    }),
  });
  await loadFeeds();
  await loadItems();
}

async function retryFeed(id) {
  els.refreshStatus.textContent = "Retrying feed";
  try {
    await api(`/api/feeds/${id}/refresh`, { method: "POST", body: JSON.stringify({}) });
  } catch (error) {
    els.refreshStatus.textContent = error.message;
  }
  await loadFeeds();
  await loadItems();
}

async function deleteFeed(id, title) {
  if (!window.confirm(`Delete "${title}" and all saved items from it?`)) return;
  await api(`/api/feeds/${id}`, { method: "DELETE" });
  if (state.feedId === id) {
    state.feedId = null;
    state.category = "all";
  }
  await loadFeeds();
  await loadItems();
}

async function markSelectionRead() {
  const body = scopedBody({ read: true, status: "unread" });
  const payload = await api("/api/read", { method: "POST", body: JSON.stringify(body) });
  els.refreshStatus.textContent = `Marked ${payload.updated || 0} item${payload.updated === 1 ? "" : "s"} read`;
  await loadFeeds();
  await loadItems();
}

async function markScrolledItemsRead() {
  if (!state.settings.markReadOnScroll || state.items.length === 0) return;
  const listRect = els.itemList.getBoundingClientRect();
  const ids = [];
  for (const row of els.itemList.querySelectorAll(".item-row[data-item-id]")) {
    const id = Number(row.dataset.itemId);
    const item = state.items.find((candidate) => candidate.id === id);
    if (!item || item.read) continue;
    if (row.getBoundingClientRect().bottom < listRect.top) {
      ids.push(id);
      row.classList.add("read");
    }
  }
  if (ids.length === 0) return;
  state.items = state.items.map((item) => (ids.includes(item.id) ? { ...item, read: true, read_at: new Date().toISOString() } : item));
  await api("/api/read", { method: "POST", body: JSON.stringify({ ids, read: true }) });
  await loadFeeds();
}

async function openExport() {
  els.exportModal.classList.remove("hidden");
  els.exportScope.textContent = `Export scope: ${scopeLabel()}`;
  await refreshExportText();
}

async function refreshExportText() {
  const params = new URLSearchParams();
  params.set("period", els.exportPeriod.value);
  params.set("status", els.exportStatus.value);
  params.set("timezone", userTimezone());
  applyScopeToParams(params);
  const text = await api(`/api/export?${params}`);
  els.exportText.value = text;
}

function downloadExport() {
  const blob = new Blob([els.exportText.value], { type: "text/markdown" });
  const link = document.createElement("a");
  link.href = URL.createObjectURL(blob);
  link.download = `feedler-${els.exportPeriod.value}.md`;
  link.click();
  URL.revokeObjectURL(link.href);
}

function applyScopeToParams(params) {
  if (state.feedId) {
    params.set("feed_id", String(state.feedId));
  } else if (state.category !== "all") {
    params.set("category", state.category);
  }
}

function scopedBody(extra = {}) {
  const body = { ...extra, timezone: userTimezone() };
  if (state.feedId) {
    body.feed_id = state.feedId;
  } else if (state.category !== "all") {
    body.category = state.category;
  }
  return body;
}

function scopeLabel() {
  if (state.feedId) {
    return state.feeds.find((feed) => feed.id === state.feedId)?.title || "selected feed";
  }
  if (state.category !== "all") {
    return state.category;
  }
  return "all feeds";
}

function loadSettings() {
  try {
    return { ...defaultSettings, ...JSON.parse(localStorage.getItem("feedler.settings") || "{}") };
  } catch (_) {
    return { ...defaultSettings };
  }
}

function saveSettings() {
  localStorage.setItem("feedler.settings", JSON.stringify(state.settings));
}

function syncSettingsUI() {
  document.body.classList.toggle("density-compact", state.settings.density === "compact");
  els.scrollReadSetting.checked = state.settings.markReadOnScroll;
  els.densitySetting.value = state.settings.density;
  els.defaultStatusSetting.value = state.settings.defaultStatus;
  els.defaultRangeSetting.value = state.settings.defaultRange;
  els.rangeSelect.value = state.range;
  document.querySelectorAll(".segmented button").forEach((button) => {
    button.classList.toggle("active", button.dataset.status === state.status);
  });
}

function updateSetting(key, value) {
  state.settings[key] = value;
  saveSettings();
  syncSettingsUI();
}

function userTimezone() {
  return Intl.DateTimeFormat().resolvedOptions().timeZone || "";
}

function formatDate(value) {
  if (!value) return "";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
  return new Intl.DateTimeFormat(undefined, {
    month: "short",
    day: "numeric",
    hour: "2-digit",
    minute: "2-digit",
  }).format(date);
}

function updateURLItem(id) {
  const url = new URL(window.location.href);
  url.searchParams.set("item", String(id));
  window.history.replaceState({}, "", url);
}

function openModal(modal) {
  modal.classList.remove("hidden");
}

function closeModal(modal) {
  modal.classList.add("hidden");
}

function closeAllModals() {
  [els.exportModal, els.manageModal, els.settingsModal, els.shortcutsModal].forEach(closeModal);
}

function selectAdjacent(delta) {
  if (state.items.length === 0) return;
  const currentIndex = state.selectedItem ? state.items.findIndex((item) => item.id === state.selectedItem.id) : -1;
  const nextIndex = Math.min(Math.max(currentIndex + delta, 0), state.items.length - 1);
  selectItem(state.items[nextIndex].id);
}

function debounce(fn, delay) {
  let timer = null;
  return (...args) => {
    window.clearTimeout(timer);
    timer = window.setTimeout(() => fn(...args), delay);
  };
}

function bindEvents() {
  els.refreshButton.addEventListener("click", refreshFeeds);
  els.manageFeedsButton.addEventListener("click", () => openModal(els.manageModal));
  els.settingsButton.addEventListener("click", () => openModal(els.settingsModal));
  els.shortcutsButton.addEventListener("click", () => openModal(els.shortcutsModal));
  els.allCategoriesButton.addEventListener("click", () => selectScope({ category: "all", feedId: null }));
  els.searchInput.addEventListener(
    "input",
    debounce(() => {
      state.search = els.searchInput.value.trim();
      loadItems();
    }, 220),
  );
  els.rangeSelect.addEventListener("change", () => {
    state.range = els.rangeSelect.value;
    loadItems();
  });
  document.querySelectorAll(".segmented button").forEach((button) => {
    button.addEventListener("click", () => {
      state.status = button.dataset.status;
      syncSettingsUI();
      loadItems();
    });
  });
  els.markSelectionReadButton.addEventListener("click", markSelectionRead);
  els.opmlInput.addEventListener("change", () => {
    const [file] = els.opmlInput.files;
    if (file) importOPML(file);
    els.opmlInput.value = "";
  });
  els.exportButton.addEventListener("click", openExport);
  els.closeExportButton.addEventListener("click", () => closeModal(els.exportModal));
  els.exportPeriod.addEventListener("change", refreshExportText);
  els.exportStatus.addEventListener("change", refreshExportText);
  els.copyExportButton.addEventListener("click", () => navigator.clipboard.writeText(els.exportText.value));
  els.downloadExportButton.addEventListener("click", downloadExport);
  els.addFeedForm.addEventListener("submit", addFeed);
  els.closeManageButton.addEventListener("click", () => closeModal(els.manageModal));
  els.closeSettingsButton.addEventListener("click", () => closeModal(els.settingsModal));
  els.closeShortcutsButton.addEventListener("click", () => closeModal(els.shortcutsModal));
  els.scrollReadSetting.addEventListener("change", () => updateSetting("markReadOnScroll", els.scrollReadSetting.checked));
  els.densitySetting.addEventListener("change", () => updateSetting("density", els.densitySetting.value));
  els.defaultStatusSetting.addEventListener("change", () => updateSetting("defaultStatus", els.defaultStatusSetting.value));
  els.defaultRangeSetting.addEventListener("change", () => updateSetting("defaultRange", els.defaultRangeSetting.value));
  els.itemList.addEventListener("scroll", () => {
    window.clearTimeout(state.scrollReadTimer);
    state.scrollReadTimer = window.setTimeout(markScrolledItemsRead, 120);
  });
  [els.exportModal, els.manageModal, els.settingsModal, els.shortcutsModal].forEach((modal) => {
    modal.addEventListener("click", (event) => {
      if (event.target === modal) closeModal(modal);
    });
  });
  document.addEventListener("keydown", handleShortcuts);
}

function handleShortcuts(event) {
  const target = event.target;
  const isTextInput = ["INPUT", "TEXTAREA", "SELECT"].includes(target?.tagName);
  if (event.key === "Escape") {
    closeAllModals();
    if (target === els.searchInput) target.blur();
    return;
  }
  if (isTextInput) return;

  switch (event.key) {
    case "?":
      event.preventDefault();
      openModal(els.shortcutsModal);
      break;
    case "/":
      event.preventDefault();
      els.searchInput.focus();
      break;
    case "j":
      event.preventDefault();
      selectAdjacent(1);
      break;
    case "k":
      event.preventDefault();
      selectAdjacent(-1);
      break;
    case "r":
      event.preventDefault();
      refreshFeeds();
      break;
    case "e":
      event.preventDefault();
      openExport();
      break;
    case "a":
      event.preventDefault();
      markSelectionRead();
      break;
    case "m":
      event.preventDefault();
      toggleSelectedRead();
      break;
  }
}

async function boot() {
  bindEvents();
  syncSettingsUI();
  await loadFeeds();
  await loadItems();
  await pollRefresh();

  const selected = new URL(window.location.href).searchParams.get("item");
  if (selected) {
    const id = Number(selected);
    if (Number.isFinite(id)) selectItem(id);
  }
}

boot().catch((error) => {
  els.resultLine.textContent = error.message;
  console.error(error);
});
