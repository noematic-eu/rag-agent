package main

import (
	"strings"
	"testing"
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
