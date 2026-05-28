package main

import (
	"fmt"
	"html"
	"net/http"
	"strings"
	"time"
)

func exportWindow(now time.Time, rangeName string) (time.Time, time.Time, string) {
	startOfDay := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	switch rangeName {
	case "week":
		offset := (int(now.Weekday()) + 6) % 7
		start := startOfDay.AddDate(0, 0, -offset)
		return start, start.AddDate(0, 0, 7), "This Week Reads"
	default:
		return startOfDay, startOfDay.AddDate(0, 0, 1), "Today Reads"
	}
}

func renderMarkdownExport(title, scope string, loc *time.Location, start, end time.Time, items []Item, r *http.Request) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# %s\n\n", title)
	fmt.Fprintf(&b, "- Scope: %s\n", scope)
	fmt.Fprintf(&b, "- Timezone: %s\n", loc.String())
	fmt.Fprintf(&b, "- Window: %s to %s\n", start.Format("2006-01-02 15:04"), end.Format("2006-01-02 15:04"))
	fmt.Fprintf(&b, "- Items: %d\n\n", len(items))

	if len(items) == 0 {
		b.WriteString("No articles found for this selection.\n")
		return b.String()
	}

	for _, item := range items {
		fmt.Fprintf(&b, "## %s\n\n", markdownEscape(item.Title))
		fmt.Fprintf(&b, "- Feed: %s\n", markdownEscape(item.FeedTitle))
		if item.PublishedAt != nil {
			fmt.Fprintf(&b, "- Published: %s\n", item.PublishedAt.In(loc).Format("2006-01-02 15:04 MST"))
		}
		if item.Author != "" {
			fmt.Fprintf(&b, "- Author: %s\n", markdownEscape(item.Author))
		}
		if item.Link != "" {
			fmt.Fprintf(&b, "- Original: <%s>\n", item.Link)
		}
		fmt.Fprintf(&b, "- Feedler: <%s>\n\n", absoluteReaderURL(r, item.ReaderURL))
		excerpt := item.Summary
		if excerpt == "" {
			excerpt = item.Content
		}
		excerpt = markdownExcerpt(excerpt)
		if excerpt != "" {
			fmt.Fprintf(&b, "%s\n\n", excerpt)
		}
	}
	return b.String()
}

func scopeLabel(scope string, id int64) string {
	switch scope {
	case "folder":
		return fmt.Sprintf("Folder #%d", id)
	case "feed":
		return fmt.Sprintf("Feed #%d", id)
	default:
		return "All articles"
	}
}

func absoluteReaderURL(r *http.Request, path string) string {
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	host := r.Host
	if host == "" {
		host = "localhost:8080"
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	return scheme + "://" + host + path
}

func markdownExcerpt(value string) string {
	value = html.UnescapeString(stripTags(value))
	value = cleanText(value)
	if len(value) > 900 {
		value = value[:900] + "..."
	}
	return value
}

func stripTags(value string) string {
	var b strings.Builder
	inTag := false
	for _, r := range value {
		switch r {
		case '<':
			inTag = true
		case '>':
			inTag = false
		default:
			if !inTag {
				b.WriteRune(r)
			}
		}
	}
	return b.String()
}

func markdownEscape(value string) string {
	value = strings.ReplaceAll(value, "\n", " ")
	return strings.TrimSpace(value)
}
