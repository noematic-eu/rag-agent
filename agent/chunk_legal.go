package main

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/noematic-eu/ai-rag-agent/model"
)

var (
	legalArticleHeaderRe = regexp.MustCompile(`(?i)^ARTICLE\s+(?:PREMIER|\d+)\.?$`)
	legalTitreHeaderRe   = regexp.MustCompile(`(?i)^Titre\s+`)
	legalArticleDetectRe = regexp.MustCompile(`(?im)^ARTICLE\s+(?:PREMIER|\d+)`)
)

type legalSection struct {
	title       string
	sectionPath string
	text        string
}

func hasLegalArticleStructure(content string) bool {
	return legalArticleDetectRe.MatchString(content)
}

func extractArticleNumber(title string) string {
	upper := strings.ToUpper(strings.TrimSpace(title))
	if strings.HasPrefix(upper, "ARTICLE PREMIER") {
		return "1"
	}
	var num strings.Builder
	for _, r := range upper {
		if r >= '0' && r <= '9' {
			num.WriteRune(r)
		} else if num.Len() > 0 {
			break
		}
	}
	return num.String()
}

func splitByLegalArticles(content string) []legalSection {
	lines := strings.Split(content, "\n")
	var sections []legalSection
	var currentTitle string
	var currentPath string
	var buf strings.Builder

	flush := func() {
		text := strings.TrimSpace(buf.String())
		if text == "" {
			return
		}
		title := currentTitle
		if title == "" {
			title = "PRÉAMBULE"
		}
		sections = append(sections, legalSection{
			title:       title,
			sectionPath: currentPath,
			text:        text,
		})
		buf.Reset()
	}

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			if buf.Len() > 0 {
				buf.WriteString("\n")
			}
			continue
		}

		if legalTitreHeaderRe.MatchString(trimmed) {
			currentPath = trimmed
			continue
		}

		if legalArticleHeaderRe.MatchString(trimmed) {
			flush()
			currentTitle = trimmed
			buf.WriteString(trimmed)
			buf.WriteString("\n\n")
			continue
		}

		if currentTitle == "" && strings.EqualFold(trimmed, "PRÉAMBULE") {
			currentTitle = trimmed
			buf.WriteString(trimmed)
			buf.WriteString("\n\n")
			continue
		}

		if buf.Len() == 0 && currentTitle == "" {
			currentTitle = "PRÉAMBULE"
		}
		buf.WriteString(line)
		buf.WriteString("\n")
	}
	flush()
	return sections
}

const legalContextSentences = 1

func extractBoundarySentence(text string, fromEnd bool) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return ""
	}
	parts := strings.Split(text, ". ")
	for i := range parts {
		parts[i] = strings.TrimSpace(parts[i])
	}
	if fromEnd {
		for i := len(parts) - 1; i >= 0; i-- {
			if len(parts[i]) > 20 {
				return parts[i]
			}
		}
		return parts[len(parts)-1]
	}
	for _, p := range parts {
		if len(p) > 20 {
			return p
		}
	}
	if len(parts) > 0 {
		return parts[0]
	}
	return ""
}

func wrapWithNeighborContext(sections []legalSection) []legalSection {
	if legalContextSentences <= 0 || len(sections) == 0 {
		return sections
	}
	out := make([]legalSection, len(sections))
	copy(out, sections)
	for i := range out {
		if i > 0 {
			if prev := extractBoundarySentence(sections[i-1].text, true); prev != "" {
				out[i].text = "[Contexte précédent] " + prev + ".\n\n" + out[i].text
			}
		}
		if i+1 < len(sections) {
			if next := extractBoundarySentence(sections[i+1].text, false); next != "" {
				out[i].text = out[i].text + "\n\n[Contexte suivant] " + next
			}
		}
	}
	return out
}

func appendLegalSections(chunks *[]model.Chunk, chunkIndex *int, docID string, sections []legalSection, config ChunkConfig) {
	sections = wrapWithNeighborContext(sections)
	for _, sec := range sections {
		if sec.text == "" {
			continue
		}
		sectionPath := sec.sectionPath
		if sectionPath == "" {
			sectionPath = sec.title
		} else {
			sectionPath = sectionPath + " -> " + sec.title
		}
		title := sec.title
		if sectionExceedsChunkLimit(sec.text, config) {
			appendTokenSplitChunks(chunks, chunkIndex, docID, title, sectionPath, sec.text, config)
			continue
		}
		*chunks = append(*chunks, model.Chunk{
			Metadata: model.ChunkMetadata{
				DocID:       docID,
				ChunkID:     fmt.Sprintf("%s-chunk-%d", docID, *chunkIndex),
				Title:       title,
				SectionPath: sectionPath,
				Source:      "markdown",
				Position:    *chunkIndex,
				Article:     extractArticleNumber(title),
			},
			Text: sec.text,
		})
		*chunkIndex++
	}
}
