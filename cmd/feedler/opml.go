package main

import (
	"context"
	"encoding/xml"
	"io"
	"os"
	"strings"
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
	Outlines []opmlOutline `xml:"outline"`
}

func (s *Store) ImportOPML(ctx context.Context, path string) (int, error) {
	file, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer file.Close()
	return s.ImportOPMLReader(ctx, file)
}

func (s *Store) ImportOPMLReader(ctx context.Context, reader io.Reader) (int, error) {
	var doc opmlDocument
	if err := xml.NewDecoder(reader).Decode(&doc); err != nil {
		return 0, err
	}

	count := 0
	var walk func([]opmlOutline, []string) error
	walk = func(outlines []opmlOutline, path []string) error {
		for _, outline := range outlines {
			title := firstNonEmpty(outline.Title, outline.Text)
			xmlURL := strings.TrimSpace(outline.XMLURL)

			if xmlURL != "" {
				category := strings.Join(path, " / ")
				if category == "" {
					category = "Uncategorized"
				}
				if err := s.UpsertFeed(ctx, Feed{
					Title:    title,
					SiteURL:  strings.TrimSpace(outline.HTMLURL),
					FeedURL:  xmlURL,
					Category: category,
				}); err != nil {
					return err
				}
				count++
				continue
			}

			nextPath := path
			if title != "" {
				nextPath = append(append([]string{}, path...), title)
			}
			if len(outline.Outlines) > 0 {
				if err := walk(outline.Outlines, nextPath); err != nil {
					return err
				}
			}
		}
		return nil
	}

	if err := walk(doc.Body.Outlines, nil); err != nil {
		return 0, err
	}
	return count, nil
}
