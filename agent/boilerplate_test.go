package main

import (
	"strings"
	"testing"

	"github.com/noematic-eu/ai-rag-agent/model"
)

func TestIsCopyrightBoilerplate(t *testing.T) {
	line := "Toute représentation ou reproduction, intégrale ou partielle, faite sans le consentement de MICRO APPLICATION est illicite (article L122-4 du code de la propriété intellectuelle)."
	if !isCopyrightBoilerplate(line) {
		t.Fatal("expected copyright line to match")
	}
	if isCopyrightBoilerplate("Il est donc temps de mesurer vos performances commerciales !") {
		t.Fatal("expected business line not to match copyright")
	}
}

func TestStripCopyrightLines(t *testing.T) {
	input := "Toute représentation ou reproduction sans le consentement est illicite (article L122-4 du code de la propriété intellectuelle).\n\nIl est donc temps de mesurer vos performances commerciales !"
	got := stripCopyrightLines(input)
	if strings.Contains(strings.ToLower(got), "propriété intellectuelle") {
		t.Fatalf("copyright not stripped: %q", got)
	}
	if !strings.Contains(got, "performances commerciales") {
		t.Fatalf("content lost: %q", got)
	}
}

func TestIsBoilerplateTitle(t *testing.T) {
	long := "Toute représentation ou reproduction, intégrale ou partielle, faite sans leconsentement de MICRO APPLICATION est illicite (article L122-4 du codede la propriété intellectuelle). Cette représentation ou reproduction illicite constituerait une contrefaçon sanctionnée par les articles L335-2 et suivants."
	if !isBoilerplateTitle(long) {
		t.Fatal("expected long copyright title to be boilerplate")
	}
	if isBoilerplateTitle("Chapitre 4 Suivre vos propositions commerciales") {
		t.Fatal("chapter title should not be boilerplate")
	}
}

func TestDeriveChunkTitleFromBody(t *testing.T) {
	body := "131 Chapitre 4 Suivre vos propositions commerciales nombre de propositions signées"
	got := deriveChunkTitle("", body, 2)
	if !strings.Contains(strings.ToLower(got), "chapitre 4") {
		t.Fatalf("expected chapitre in title, got %q", got)
	}
}

func TestDisplaySectionPathSanitizesCopyright(t *testing.T) {
	doc := model.LegalDocument{
		BookTitle: "Office.2007.Reussir.Votre.Entreprise",
		Title:     "Toute représentation ou reproduction sans le consentement (article L122-4 du code de la propriété intellectuelle).",
		Content:   "Chapitre 4 Suivre vos propositions commerciales. Il est donc temps de mesurer vos performances commerciales.",
	}
	path := displaySectionPath(doc)
	if strings.Contains(strings.ToLower(path), "propriété intellectuelle") {
		t.Fatalf("section path still contains copyright: %q", path)
	}
	if !strings.Contains(strings.ToLower(path), "chapitre 4") {
		t.Fatalf("expected chapter in path: %q", path)
	}
}
