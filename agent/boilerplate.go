package main

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/noematic-eu/ai-rag-agent/model"
)

var (
	copyrightLinePatterns = []string{
		"propriété intellectuelle",
		"propriete intellectuelle",
		"l122-4",
		"l335-2",
		"représentation ou reproduction",
		"representation ou reproduction",
		"sans le consentement",
		"sans leconsentement",
		"contrefaçon",
		"contrefacon",
		"all rights reserved",
		"copyright",
		"©",
	}
	chapitreTitleRe       = regexp.MustCompile(`(?i)chapitre\s+\d+[^\n]{0,120}`)
	sectionHeadingTitleRe = regexp.MustCompile(`(?i)(?:^|\n)([A-ZÉÈÊÀÂÙÛÎÏÔÖÇ][^\n]{10,80}(?:vente|commercial|proposition|présentation|presentation|entreprise)[^\n]{0,60})`)
)

func isCopyrightBoilerplate(line string) bool {
	lower := strings.ToLower(strings.TrimSpace(line))
	if lower == "" {
		return false
	}
	for _, pattern := range copyrightLinePatterns {
		if strings.Contains(lower, pattern) {
			return true
		}
	}
	return false
}

func isBoilerplateTitle(title string) bool {
	title = strings.TrimSpace(title)
	if title == "" {
		return false
	}
	if isCopyrightBoilerplate(title) {
		return true
	}
	// Long lines dominated by legal/copyright vocabulary are usually PDF headers.
	if len(title) > 120 && copyrightKeywordHits(title) >= 2 {
		return true
	}
	return false
}

func copyrightKeywordHits(text string) int {
	lower := strings.ToLower(text)
	hits := 0
	for _, pattern := range copyrightLinePatterns {
		if strings.Contains(lower, pattern) {
			hits++
		}
	}
	return hits
}

// stripCopyrightLines removes lines that are copyright/legal boilerplate.
func stripCopyrightLines(text string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return ""
	}
	lines := strings.Split(text, "\n")
	cleaned := make([]string, 0, len(lines))
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if isCopyrightBoilerplate(trimmed) {
			continue
		}
		cleaned = append(cleaned, trimmed)
	}
	if len(cleaned) == 0 {
		return ""
	}
	return strings.Join(cleaned, "\n")
}

// stripCopyrightInline removes copyright boilerplate embedded in a single line (PDF artifacts).
func stripCopyrightInline(text string) string {
	text = strings.TrimSpace(text)
	if text == "" || !isBoilerplateTitle(text) {
		return stripCopyrightLines(text)
	}
	// Try to keep content after the boilerplate sentence ends.
	for _, sep := range []string{"Texte:", "texte:", "\n\n"} {
		if idx := strings.Index(text, sep); idx >= 0 {
			rest := strings.TrimSpace(text[idx+len(sep):])
			if rest != "" {
				return stripCopyrightLines(rest)
			}
		}
	}
	return stripCopyrightLines(text)
}

// deriveChunkTitle picks a human-readable section title from body when title is boilerplate.
func deriveChunkTitle(title, body string, position int) string {
	title = strings.TrimSpace(title)
	body = strings.TrimSpace(body)
	if title != "" && !isBoilerplateTitle(title) {
		return title
	}
	if m := chapitreTitleRe.FindString(body); m != "" {
		return strings.TrimSpace(m)
	}
	if m := sectionHeadingTitleRe.FindStringSubmatch(body); len(m) > 1 {
		return strings.TrimSpace(m[1])
	}
	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || isCopyrightBoilerplate(line) {
			continue
		}
		if len(line) >= 12 && len(line) <= 100 && !strings.HasSuffix(line, ".") {
			return line
		}
	}
	if position >= 0 {
		return fmtSectionFallback(position)
	}
	return "Section"
}

func fmtSectionFallback(position int) string {
	return fmt.Sprintf("Section-%d", position+1)
}

// humanizeDocStem turns a filename stem into a readable book label.
func humanizeDocStem(stem string) string {
	stem = strings.TrimSpace(stem)
	stem = strings.TrimSuffix(stem, ".pdf")
	stem = strings.TrimSuffix(stem, ".md")
	stem = strings.ReplaceAll(stem, ".", " ")
	stem = strings.ReplaceAll(stem, "_", " ")
	return strings.TrimSpace(stem)
}

// displaySectionPath builds a sanitized section= label for LLM prompts.
func displaySectionPath(doc model.LegalDocument) string {
	book := humanizeDocStem(strings.TrimSpace(doc.BookTitle))
	section := strings.TrimSpace(doc.Title)
	body := strings.TrimSpace(doc.Content)

	if isBoilerplateTitle(section) {
		section = deriveChunkTitle(section, body, -1)
	}
	if section == "" || section == book {
		section = deriveChunkTitle("", body, -1)
	}
	if book != "" && section != "" && book != section {
		return book + " -> " + section
	}
	if section != "" {
		return section
	}
	if book != "" {
		return book
	}
	return "extrait"
}
