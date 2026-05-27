package main

import (
	"fmt"
	"strings"
	"time"
)

func BuildMarkdownExport(period string, status string, baseURL string, items []Item) string {
	var b strings.Builder
	title := exportTitle(period, status)
	fmt.Fprintf(&b, "# %s\n\n", title)
	fmt.Fprintf(&b, "Generated: %s\n", time.Now().Format(time.RFC1123))
	fmt.Fprintf(&b, "Items: %d\n\n", len(items))

	if len(items) == 0 {
		b.WriteString("No items matched this export.\n")
		return b.String()
	}

	for _, item := range items {
		itemURL := fmt.Sprintf("%s/?item=%d", strings.TrimRight(baseURL, "/"), item.ID)
		fmt.Fprintf(&b, "## %s\n\n", markdownLine(item.Title))
		if item.FeedTitle != "" {
			fmt.Fprintf(&b, "- Source: %s", markdownLine(item.FeedTitle))
			if item.FeedCategory != "" {
				fmt.Fprintf(&b, " (%s)", markdownLine(item.FeedCategory))
			}
			b.WriteString("\n")
		}
		if item.PublishedAt != "" {
			fmt.Fprintf(&b, "- Published: %s\n", item.PublishedAt)
		}
		if item.Link != "" {
			fmt.Fprintf(&b, "- Original: %s\n", item.Link)
		}
		fmt.Fprintf(&b, "- Feedler: %s\n", itemURL)
		if item.Summary != "" {
			fmt.Fprintf(&b, "\n%s\n", markdownLine(truncateRunes(item.Summary, 900)))
		}
		b.WriteString("\n")
	}

	return b.String()
}

func exportTitle(period string, status string) string {
	scope := "All Reads"
	switch period {
	case "today":
		scope = "Today Reads"
	case "week":
		scope = "This Week Reads"
	case "unread":
		scope = "Unread Reads"
	}
	if status == "unread" && period != "unread" {
		return scope + " - Unread"
	}
	return scope
}

func markdownLine(value string) string {
	value = strings.ReplaceAll(value, "\r", " ")
	value = strings.ReplaceAll(value, "\n", " ")
	return strings.Join(strings.Fields(value), " ")
}
