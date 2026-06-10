package main

import (
	"html"
	"regexp"
	"strings"

	md "github.com/JohannesKaufmann/html-to-markdown"
	"github.com/PuerkitoBio/goquery"

	"github.com/noematic-eu/ai-rag-agent/model"
)

var htmlTagPattern = regexp.MustCompile(`<[^>]+>`)

const (
	contentTypeMarkdown = "markdown"
	contentTypeHTML     = "html"
)

func normalizeDocumentContent(doc model.LegalDocument) (model.LegalDocument, error) {
	normalized := doc
	contentType := strings.ToLower(strings.TrimSpace(normalized.ContentType))
	if contentType == "" {
		contentType = contentTypeMarkdown
	}

	switch contentType {
	case contentTypeHTML:
		normalized.OriginalContent = normalized.Content
		markdownContent, err := htmlToMarkdown(normalized.Content)
		if err != nil {
			return model.LegalDocument{}, err
		}
		normalized.Content = markdownContent
		normalized.ContentType = contentTypeHTML
	default:
		normalized.ContentType = contentTypeMarkdown
		if normalized.OriginalContent == "" {
			normalized.OriginalContent = normalized.Content
		}
		normalized.Content = stripCopyrightInline(normalized.Content)
	}

	return normalized, nil
}

func htmlToMarkdown(rawHTML string) (string, error) {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(rawHTML))
	if err != nil {
		return "", err
	}

	// Drop non-content elements for deterministic indexing.
	doc.Find("script,style,noscript").Each(func(_ int, selection *goquery.Selection) {
		selection.Remove()
	})

	sanitizedHTML, err := doc.Html()
	if err != nil {
		return "", err
	}

	converter := md.NewConverter("", true, nil)
	md, err := converter.ConvertString(sanitizedHTML)
	if err != nil {
		return "", err
	}

	return normalizeChunkText(strings.TrimSpace(md)), nil
}

// normalizeChunkText strips residual HTML, entities, and low-value bullet lines.
func normalizeChunkText(s string) string {
	s = html.UnescapeString(s)
	s = htmlTagPattern.ReplaceAllString(s, " ")

	lines := strings.Split(s, "\n")
	cleaned := make([]string, 0, len(lines))
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || trimmed == "•" || trimmed == "*" || trimmed == "-" {
			continue
		}
		cleaned = append(cleaned, trimmed)
	}
	s = strings.Join(cleaned, "\n")
	s = strings.Join(strings.Fields(s), " ")
	return strings.TrimSpace(s)
}
