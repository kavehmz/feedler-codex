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
};

const els = {
  feedSummary: document.querySelector("#feedSummary"),
  refreshButton: document.querySelector("#refreshButton"),
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
  markVisibleReadButton: document.querySelector("#markVisibleReadButton"),
  exportButton: document.querySelector("#exportButton"),
  exportModal: document.querySelector("#exportModal"),
  closeExportButton: document.querySelector("#closeExportButton"),
  exportPeriod: document.querySelector("#exportPeriod"),
  exportStatus: document.querySelector("#exportStatus"),
  exportText: document.querySelector("#exportText"),
  copyExportButton: document.querySelector("#copyExportButton"),
  downloadExportButton: document.querySelector("#downloadExportButton"),
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
}

async function loadItems() {
  const params = new URLSearchParams();
  params.set("status", state.status);
  params.set("range", state.range);
  params.set("limit", "120");
  if (state.feedId) params.set("feed_id", String(state.feedId));
  if (state.category !== "all") params.set("category", state.category);
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
      button.addEventListener("click", () => {
        state.category = category;
        state.feedId = null;
        loadItems();
        renderFeeds();
      });
      return button;
    }),
  );

  els.feedList.replaceChildren(
    ...state.feeds.map((feed) => {
      const button = document.createElement("button");
      button.className = "feed-row";
      button.type = "button";
      button.classList.toggle("active", state.feedId === feed.id);
      button.title = feed.last_error ? `${feed.title}\n${feed.last_error}` : feed.title;
      button.innerHTML = `<span></span><span></span>`;
      button.children[0].textContent = feed.title;
      button.children[1].textContent = feed.unread_count;
      button.addEventListener("click", () => {
        state.feedId = feed.id;
        state.category = "all";
        loadItems();
        renderFeeds();
      });
      return button;
    }),
  );

  if (totalItems === 0) {
    els.resultLine.textContent = "Refreshing imported feeds";
  }
}

function renderItems() {
  els.resultLine.textContent = `${state.items.length} item${state.items.length === 1 ? "" : "s"}`;
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
    const payload = await api(`/api/items/${id}`, {
      method: "PATCH",
      body: JSON.stringify({ read: true }),
    });
    state.selectedItem = payload.item;
    state.items = state.items.map((candidate) => (candidate.id === id ? payload.item : candidate));
    renderReader(payload.item);
    renderItems();
    await loadFeeds();
  }
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
  toggle.addEventListener("click", async () => {
    const payload = await api(`/api/items/${item.id}`, {
      method: "PATCH",
      body: JSON.stringify({ read: !item.read }),
    });
    state.selectedItem = payload.item;
    state.items = state.items.map((candidate) => (candidate.id === item.id ? payload.item : candidate));
    renderReader(payload.item);
    renderItems();
    await loadFeeds();
  });

  els.readerPane.querySelector(".article-body").textContent = item.content || item.summary || "No article text was provided by this feed.";
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

async function openExport() {
  els.exportModal.classList.remove("hidden");
  await refreshExportText();
}

async function refreshExportText() {
  const params = new URLSearchParams();
  params.set("period", els.exportPeriod.value);
  params.set("status", els.exportStatus.value);
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

function debounce(fn, delay) {
  let timer = null;
  return (...args) => {
    window.clearTimeout(timer);
    timer = window.setTimeout(() => fn(...args), delay);
  };
}

function bindEvents() {
  els.refreshButton.addEventListener("click", refreshFeeds);
  els.allCategoriesButton.addEventListener("click", () => {
    state.category = "all";
    state.feedId = null;
    loadItems();
    renderFeeds();
  });
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
      document.querySelectorAll(".segmented button").forEach((candidate) => candidate.classList.remove("active"));
      button.classList.add("active");
      state.status = button.dataset.status;
      loadItems();
    });
  });
  els.markVisibleReadButton.addEventListener("click", async () => {
    const ids = state.items.filter((item) => !item.read).map((item) => item.id);
    if (ids.length === 0) return;
    await api("/api/read", { method: "POST", body: JSON.stringify({ ids, read: true }) });
    await loadFeeds();
    await loadItems();
  });
  els.opmlInput.addEventListener("change", () => {
    const [file] = els.opmlInput.files;
    if (file) importOPML(file);
    els.opmlInput.value = "";
  });
  els.exportButton.addEventListener("click", openExport);
  els.closeExportButton.addEventListener("click", () => els.exportModal.classList.add("hidden"));
  els.exportPeriod.addEventListener("change", refreshExportText);
  els.exportStatus.addEventListener("change", refreshExportText);
  els.copyExportButton.addEventListener("click", () => navigator.clipboard.writeText(els.exportText.value));
  els.downloadExportButton.addEventListener("click", downloadExport);
  els.exportModal.addEventListener("click", (event) => {
    if (event.target === els.exportModal) els.exportModal.classList.add("hidden");
  });
}

async function boot() {
  bindEvents();
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
