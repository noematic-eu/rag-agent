package main

import (
	"os"
	"strings"
	"testing"

	"github.com/noematic-eu/ai-rag-agent/model"
)

func TestStripNeighborContext(t *testing.T) {
	content := "[Contexte précédent] Son principe est : gouvernement du peuple.\n\nARTICLE 2. La langue de la République est le français.\n\n[Contexte suivant] ARTICLE 3. La souveraineté"
	stripped := stripNeighborContext(content)
	if strings.Contains(stripped, "[Contexte précédent]") || strings.Contains(stripped, "[Contexte suivant]") {
		t.Fatalf("neighbor context not stripped: %s", stripped)
	}
	if !strings.Contains(stripped, "ARTICLE 2") {
		t.Fatalf("main text missing: %s", stripped)
	}

	compact := "[Contexte précédent] Il préside les conseils de la défense nationale.. ARTICLE 16. Lorsque les institutions sont menacées."
	stripped = stripNeighborContext(compact)
	if strings.Contains(stripped, "[Contexte précédent]") {
		t.Fatalf("compact neighbor context not stripped: %s", stripped)
	}
	if !strings.Contains(stripped, "Lorsque les institutions") {
		t.Fatalf("article 16 body missing: %s", stripped)
	}
}

func TestExcerptTextStripsNeighborContext(t *testing.T) {
	content := "[Contexte précédent] gouvernement du peuple.\n\nARTICLE 5. Le Président veille au respect de la Constitution."
	out := excerptText(content, "president obligations", 900)
	if strings.Contains(out, "gouvernement du peuple") {
		t.Fatalf("neighbor context leaked into excerpt: %s", out)
	}
	if !strings.Contains(out, "ARTICLE 5") {
		t.Fatalf("expected article 5 text: %s", out)
	}
}

func TestExcerptTextArticle16WithArticleQuery(t *testing.T) {
	raw, err := os.ReadFile("../texts/constitution.md")
	if err != nil {
		t.Skipf("constitution fixture unavailable: %v", err)
	}
	doc := model.LegalDocument{
		ID:      "doc-constitution-test",
		Title:   "constitution",
		Content: string(raw),
	}
	chunks, err := ChunkDocument(doc, DefaultChunkConfig())
	if err != nil {
		t.Fatalf("ChunkDocument: %v", err)
	}
	var art16Text string
	for _, ch := range chunks {
		if ch.Metadata.Article == "16" {
			art16Text = ch.Text
			break
		}
	}
	if art16Text == "" {
		t.Fatal("ARTICLE 16 chunk not found")
	}

	query := "article 16 pleins pouvoirs election presidentielle article 7 organisation scrutin"
	out := excerptTextForChunk(art16Text, query, "16", maxSnippetChars)
	if isLegalHeadingOnlySentence(strings.TrimSpace(strings.Split(out, ". ")[0])) && !strings.Contains(out, "pouvoirs exceptionnels") {
		t.Fatalf("excerpt collapsed to heading only: %q", out)
	}
	if !strings.Contains(out, "pouvoirs exceptionnels") && !strings.Contains(out, "mesures exigées") {
		t.Fatalf("expected substantive Article 16 body, got: %q", out[:min(200, len(out))])
	}
	if !strings.Contains(out, "dissoute") {
		t.Fatalf("expected dissolution ban in Article 16 excerpt, got: %q", out[:min(300, len(out))])
	}
}

func TestExcerptTextArticle7WithArticleQuery(t *testing.T) {
	raw, err := os.ReadFile("../texts/constitution.md")
	if err != nil {
		t.Skipf("constitution fixture unavailable: %v", err)
	}
	doc := model.LegalDocument{
		ID:      "doc-constitution-test",
		Title:   "constitution",
		Content: string(raw),
	}
	chunks, err := ChunkDocument(doc, DefaultChunkConfig())
	if err != nil {
		t.Fatalf("ChunkDocument: %v", err)
	}
	var art7Text string
	for _, ch := range chunks {
		if ch.Metadata.Article == "7" {
			art7Text = ch.Text
			break
		}
	}
	if art7Text == "" {
		t.Fatal("ARTICLE 7 chunk not found")
	}

	query := "article 16 pleins pouvoirs election presidentielle article 7 organisation scrutin"
	out := excerptTextForChunk(art7Text, query, "7", maxSnippetChars)
	if isLegalHeadingOnlySentence(strings.TrimSpace(strings.Split(out, ". ")[0])) && !strings.Contains(out, "Président") {
		t.Fatalf("excerpt collapsed to heading only: %q", out)
	}
	if !strings.Contains(out, "élection du nouveau Président") && !strings.Contains(out, "report") {
		t.Fatalf("expected substantive Article 7 body, got: %q", out[:min(200, len(out))])
	}
	if strings.Contains(strings.ToUpper(out[:min(80, len(out))]), "ARTICLE 6") {
		t.Fatalf("Article 7 excerpt should not start with Article 6 bleed: %q", out[:min(120, len(out))])
	}
	if !strings.Contains(strings.ToLower(out), "reporter") {
		t.Fatalf("expected report/reporter in Article 7 excerpt, got: %q", out[:min(400, len(out))])
	}
}

func TestExcerptTextForChunkUsesLegalLimit(t *testing.T) {
	raw, err := os.ReadFile("../texts/constitution.md")
	if err != nil {
		t.Skipf("constitution fixture unavailable: %v", err)
	}
	doc := model.LegalDocument{ID: "doc-test", Title: "constitution", Content: string(raw)}
	chunks, err := ChunkDocument(doc, DefaultChunkConfig())
	if err != nil {
		t.Fatalf("ChunkDocument: %v", err)
	}
	query := "article 16 pleins pouvoirs election presidentielle article 7 organisation scrutin"
	for _, ch := range chunks {
		if ch.Metadata.Article != "16" {
			continue
		}
		out := excerptTextForChunk(ch.Text, query, "16", maxSnippetChars)
		if !strings.Contains(out, "pouvoirs exceptionnels") {
			t.Fatalf("expected full legal excerpt, got: %q", out[:min(200, len(out))])
		}
		return
	}
	t.Fatal("ARTICLE 16 chunk not found")
}
