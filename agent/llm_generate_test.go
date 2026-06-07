package main

import (
	"os"
	"strings"
	"testing"

	"github.com/noematic-eu/ai-rag-agent/model"
)

func TestRagSystemPromptArticleCitations(t *testing.T) {
	fr := ragSystemPrompt("fr")
	for _, want := range []string{"article", "section=", "liens logiques", "hors sujet", "analyse", "dissoute", "empêchement", "raisonnement interne"} {
		if !strings.Contains(strings.ToLower(fr), strings.ToLower(want)) {
			t.Fatalf("French prompt missing %q: %s", want, fr)
		}
	}
	en := ragSystemPrompt("en")
	if !strings.Contains(en, "Article 16") && !strings.Contains(en, "article") {
		t.Fatalf("English prompt should mention article citations: %s", en)
	}
}

func TestBuildRAGUserMessageRespectsTopK(t *testing.T) {
	docs := make([]model.LegalDocument, 0, 10)
	for i := 0; i < 10; i++ {
		docs = append(docs, model.LegalDocument{
			Title:   "ARTICLE 1.",
			Article: "1",
			Content: "contenu test",
		})
	}
	msg := buildRAGUserMessage(docs, "question test", "test", 4)
	count := strings.Count(msg, "[")
	if count != 4 {
		t.Fatalf("expected 4 excerpt headers, got %d in:\n%s", count, msg)
	}
}

func TestEffectiveGenerationTopKLegal(t *testing.T) {
	docs := []model.LegalDocument{{
		Corpus:  "legal-demo",
		Article: "16",
		Content: "text",
		Title:   "ARTICLE 16.",
	}}
	if got := effectiveGenerationTopK(docs, 8); got != legalGenerationTopK {
		t.Fatalf("expected legal top_k=%d, got %d", legalGenerationTopK, got)
	}
}

func TestBuildRAGUserMessageIncludesArticleField(t *testing.T) {
	docs := []model.LegalDocument{{
		Title:     "ARTICLE 16.",
		BookTitle: "Titre III",
		Article:   "16",
		Content:   "mesures exigées",
	}}
	msg := buildRAGUserMessage(docs, "q", "rq", 8)
	if !strings.Contains(msg, "article=16") {
		t.Fatalf("expected article=16 in message: %s", msg)
	}
	if !strings.Contains(msg, "section=Titre III -> ARTICLE 16.") {
		t.Fatalf("expected section path in message: %s", msg)
	}
}

func TestBuildRAGUserMessageArticle16ConstitutionExcerpt(t *testing.T) {
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
	docs := make([]model.LegalDocument, 0, 2)
	retrievalQ := "article 16 pleins pouvoirs election presidentielle article 7 organisation scrutin"
	for _, ch := range chunks {
		if ch.Metadata.Article != "16" && ch.Metadata.Article != "7" {
			continue
		}
		docs = append(docs, model.LegalDocument{
			Title:   ch.Metadata.Title,
			Article: ch.Metadata.Article,
			Content: ch.Text,
		})
	}
	if len(docs) < 2 {
		t.Fatal("expected Article 16 and 7 chunks")
	}
	msg := buildRAGUserMessage(docs, "emergency powers", retrievalQ, 8)
	if !strings.Contains(msg, "pouvoirs exceptionnels") {
		t.Fatalf("Article 16 body missing from RAG prompt: %s", msg[:min(500, len(msg))])
	}
	if !strings.Contains(msg, "dissoute") {
		t.Fatalf("Article 16 dissolution ban missing from RAG prompt: %s", msg[:min(800, len(msg))])
	}
	if !strings.Contains(msg, "élection du nouveau Président") && !strings.Contains(strings.ToLower(msg), "report") {
		t.Fatalf("Article 7 body missing from RAG prompt: %s", msg[:min(500, len(msg))])
	}
	if !strings.Contains(msg, "ne peut être dissoute") {
		t.Fatalf("legal checklist missing Art 16 dissoute reminder: %s", msg[len(msg)-min(400, len(msg)):])
	}
}
