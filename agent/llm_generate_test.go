package main

import (
	"os"
	"strings"
	"testing"

	"github.com/noematic-eu/ai-rag-agent/model"
)

func TestRagSystemPromptLegalOverlay(t *testing.T) {
	legalDocs := []model.LegalDocument{{Corpus: "legal-demo", Article: "16", Title: "ARTICLE 16."}}
	fr := ragSystemPrompt("fr", legalDocs)
	for _, want := range []string{"article", "liens logiques", "hors sujet", "dissoute", "raisonnement interne"} {
		if !strings.Contains(strings.ToLower(fr), strings.ToLower(want)) {
			t.Fatalf("French legal prompt missing %q: %s", want, fr)
		}
	}
	en := ragSystemPrompt("en", legalDocs)
	if !strings.Contains(en, "Article 16") {
		t.Fatalf("English legal prompt should mention Article 16: %s", en)
	}
}

func TestRagSystemPromptGeneralKBOverlay(t *testing.T) {
	kbDocs := []model.LegalDocument{{Corpus: "kb-business", Title: "Chapitre 4", Content: "vente"}}
	fr := ragSystemPrompt("fr", kbDocs)
	if strings.Contains(fr, "dissoute") || strings.Contains(fr, "Article 16") {
		t.Fatalf("KB prompt should not contain legal-only rules: %s", fr)
	}
	for _, want := range []string{"partiellement pertinent", "copyright", "Texte:"} {
		if !strings.Contains(fr, want) {
			t.Fatalf("French KB prompt missing %q: %s", want, fr)
		}
	}
}

func TestBuildRAGUserMessageOmitsCopyrightSection(t *testing.T) {
	docs := []model.LegalDocument{{
		BookTitle: "Office.2007.Reussir.Votre.Entreprise",
		Title:     "Toute représentation ou reproduction sans le consentement (article L122-4 du code de la propriété intellectuelle).",
		Content:   "Chapitre 4 Suivre vos propositions commerciales. Il est donc temps de mesurer vos performances commerciales.",
	}}
	msg := buildRAGUserMessage(docs, "réussir en affaires", "affaires", 8)
	if strings.Contains(strings.ToLower(msg), "propriété intellectuelle") {
		t.Fatalf("copyright leaked into prompt: %s", msg)
	}
	if !strings.Contains(strings.ToLower(msg), "chapitre 4") {
		t.Fatalf("expected sanitized chapter in section=: %s", msg)
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
