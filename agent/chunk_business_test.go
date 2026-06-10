package main

import (
	"strings"
	"testing"

	"github.com/noematic-eu/ai-rag-agent/model"
)

func TestChunkDocumentBusinessBookSkipsCopyrightTitle(t *testing.T) {
	content := `Toute représentation ou reproduction, intégrale ou partielle, faite sans le consentement de MICRO APPLICATION est illicite (article L122-4 du code de la propriété intellectuelle).

Il est donc temps de mesurer vos performances commerciales ! Cette mesure se fait sur deux axes : un axe quantitatif et un axe qualitatif avec les taux de transformation tout au long du cycle de vente.

224 LE GUIDE DES EXPERTS Construire son argumentaire de vente Chapitre 10 Lister les avantages génériques de l'offre. L'objectif est de formaliser le savoir-faire commercial de votre entreprise pour développer le chiffre d'affaires.`
	doc := model.LegalDocument{
		ID:      "doc-business-test",
		Title:   "Office.2007.Reussir.Votre.Entreprise",
		Content: content,
	}
	chunks, err := ChunkDocument(doc, DefaultChunkConfig())
	if err != nil {
		t.Fatalf("ChunkDocument: %v", err)
	}
	if len(chunks) == 0 {
		t.Fatal("expected chunks")
	}
	for _, ch := range chunks {
		if isBoilerplateTitle(ch.Metadata.Title) {
			t.Fatalf("chunk title is copyright boilerplate: %q", ch.Metadata.Title)
		}
	}
	combined := strings.ToLower(chunks[0].Text + chunks[len(chunks)-1].Text)
	if !strings.Contains(combined, "performances commerciales") && !strings.Contains(combined, "argumentaire de vente") {
		t.Fatalf("expected business content in chunks: %v", chunks[0].Text[:min(200, len(chunks[0].Text))])
	}
}
