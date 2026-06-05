package main

import (
	"strings"
	"testing"

	"github.com/noematic-eu/ai-rag-agent/model"
)

func TestRagSystemPromptArticleCitations(t *testing.T) {
	fr := ragSystemPrompt("fr")
	for _, want := range []string{"article", "section=", "liens logiques", "hors sujet", "analyse"} {
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
