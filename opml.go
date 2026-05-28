package main

import (
	"context"
	"encoding/xml"
	"fmt"
	"os"
)

type opmlDocument struct {
	Body opmlBody `xml:"body"`
}

type opmlBody struct {
	Outlines []opmlOutline `xml:"outline"`
}

type opmlOutline struct {
	Text     string        `xml:"text,attr"`
	Title    string        `xml:"title,attr"`
	Type     string        `xml:"type,attr"`
	XMLURL   string        `xml:"xmlUrl,attr"`
	HTMLURL  string        `xml:"htmlUrl,attr"`
	Children []opmlOutline `xml:"outline"`
}

type opmlFeed struct {
	Title      string
	URL        string
	SiteURL    string
	FolderName string
}

func (a *App) ImportOPMLIfEmpty(ctx context.Context, path string) error {
	count, err := a.FeedCount(ctx)
	if err != nil {
		return err
	}
	if count > 0 {
		return nil
	}

	content, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	var doc opmlDocument
	if err := xml.Unmarshal(content, &doc); err != nil {
		return fmt.Errorf("parse opml: %w", err)
	}

	var feeds []opmlFeed
	for _, outline := range doc.Body.Outlines {
		feeds = collectOPMLFeeds(outline, "", feeds)
	}

	for _, feed := range feeds {
		folderID, err := a.EnsureFolder(ctx, feed.FolderName)
		if err != nil {
			return err
		}
		if _, err := a.UpsertFeed(ctx, feed.Title, feed.URL, feed.SiteURL, folderID); err != nil {
			return err
		}
	}
	return nil
}

func collectOPMLFeeds(outline opmlOutline, folder string, feeds []opmlFeed) []opmlFeed {
	name := outline.Title
	if name == "" {
		name = outline.Text
	}

	if outline.XMLURL != "" {
		title := name
		if title == "" {
			title = outline.XMLURL
		}
		return append(feeds, opmlFeed{
			Title:      title,
			URL:        outline.XMLURL,
			SiteURL:    outline.HTMLURL,
			FolderName: folder,
		})
	}

	nextFolder := folder
	if name != "" {
		nextFolder = name
	}
	for _, child := range outline.Children {
		feeds = collectOPMLFeeds(child, nextFolder, feeds)
	}
	return feeds
}
