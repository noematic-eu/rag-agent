package main

import (
	"os"
	"strings"
	"testing"

	"github.com/noematic-eu/ai-rag-agent/model"
)

func TestSplitByLegalArticles(t *testing.T) {
	sections := splitByLegalArticles(`PRÉAMBULE
Le peuple français proclame.

ARTICLE PREMIER.
La France est une République.

Titre II - LE PRÉSIDENT
ARTICLE 5.
Le Président veille.

ARTICLE 16.
Lorsque les institutions sont menacées.`)
	if len(sections) < 4 {
		t.Fatalf("expected at least 4 sections, got %d", len(sections))
	}
	var titles []string
	for _, s := range sections {
		titles = append(titles, s.title)
	}
	if !containsFold(titles, "ARTICLE 16.") && !containsFold(titles, "ARTICLE 16") {
		t.Fatalf("missing ARTICLE 16 in %v", titles)
	}
}

func TestChunkDocumentLegalConstitution(t *testing.T) {
	raw, err := os.ReadFile("../texts/constitution.md")
	if err != nil {
		t.Skipf("constitution fixture unavailable: %v", err)
	}
	content := string(raw)
	sections := splitByHeadings(content)
	legalSections := splitByLegalArticles(content)
	t.Logf("headings=%d legal=%d hasLegal=%v", len(sections), len(legalSections), hasLegalArticleStructure(content))
	doc := model.LegalDocument{
		ID:      "doc-constitution-test",
		Title:   "constitution",
		Content: content,
	}
	chunks, err := ChunkDocument(doc, DefaultChunkConfig())
	if err != nil {
		t.Fatalf("ChunkDocument: %v", err)
	}
	if len(chunks) < 50 {
		t.Fatalf("expected many article chunks, got %d", len(chunks))
	}

	var art16, art7 *model.Chunk
	for i := range chunks {
		switch chunks[i].Metadata.Article {
		case "16":
			art16 = &chunks[i]
		case "7":
			art7 = &chunks[i]
		}
	}
	if art16 == nil {
		t.Fatal("ARTICLE 16 chunk not found")
	}
	if art7 == nil {
		t.Fatal("ARTICLE 7 chunk not found")
	}
	if art16.Metadata.Article != "16" {
		t.Fatalf("ARTICLE 16 metadata = %q", art16.Metadata.Article)
	}
	if !strings.Contains(art16.Text, "pouvoirs exceptionnels") && !strings.Contains(art16.Text, "mesures exigées") {
		t.Fatalf("ARTICLE 16 text unexpected: %q", art16.Text[:min(120, len(art16.Text))])
	}
	if !strings.Contains(art7.Text, "élection du nouveau Président") {
		t.Fatalf("ARTICLE 7 text unexpected: %q", art7.Text[:min(120, len(art7.Text))])
	}
}

func TestWrapWithNeighborContext(t *testing.T) {
	sections := []legalSection{
		{title: "ARTICLE 5.", text: "ARTICLE 5.\n\nLe Président veille au respect de la Constitution. Il assure la continuité."},
		{title: "ARTICLE 16.", text: "ARTICLE 16.\n\nLorsque les institutions sont menacées, le Président prend les mesures exigées."},
	}
	wrapped := wrapWithNeighborContext(sections)
	if !strings.Contains(wrapped[1].text, "[Contexte précédent]") {
		t.Fatalf("expected previous context in art 16: %q", wrapped[1].text)
	}
}

func containsFold(items []string, want string) bool {
	want = strings.ToUpper(want)
	for _, item := range items {
		if strings.Contains(strings.ToUpper(item), want) {
			return true
		}
	}
	return false
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
