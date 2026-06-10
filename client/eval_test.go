package main

import "testing"

func TestScoreRetrievalDocHit(t *testing.T) {
	gc := GoldCase{
		ExpectedDocIDs: []string{"doc-a"},
	}
	hits := []retrieveHit{
		{ChunkID: "doc-b-chunk-0", DocID: "doc-b"},
		{ChunkID: "doc-a-chunk-0", DocID: "doc-a", Score: 0.9},
	}
	hit, mrr := scoreRetrieval(gc, hits, 8)
	if !hit || mrr != 0.5 {
		t.Fatalf("got hit=%v mrr=%v", hit, mrr)
	}
}

func TestScoreRetrievalMiss(t *testing.T) {
	gc := GoldCase{ExpectedDocIDs: []string{"doc-x"}}
	hits := []retrieveHit{{DocID: "doc-y", ChunkID: "doc-y-chunk-0"}}
	hit, mrr := scoreRetrieval(gc, hits, 8)
	if hit || mrr != 0 {
		t.Fatalf("got hit=%v mrr=%v", hit, mrr)
	}
}

func TestScoreRetrievalMatchAllDocIDs(t *testing.T) {
	gc := GoldCase{
		ExpectedDocIDs: []string{"doc-a", "doc-b"},
		MatchAllDocIDs: true,
	}
	hits := []retrieveHit{
		{DocID: "doc-a", ChunkID: "doc-a-chunk-0"},
		{DocID: "doc-b", ChunkID: "doc-b-chunk-0"},
	}
	hit, mrr := scoreRetrieval(gc, hits, 8)
	if !hit || mrr != 1.0 {
		t.Fatalf("got hit=%v mrr=%v", hit, mrr)
	}

	partial := []retrieveHit{{DocID: "doc-a", ChunkID: "doc-a-chunk-0"}}
	hit, mrr = scoreRetrieval(gc, partial, 8)
	if hit || mrr != 0 {
		t.Fatalf("expected miss for partial docs, got hit=%v mrr=%v", hit, mrr)
	}
}

func TestScoreExcerptTermsPass(t *testing.T) {
	gc := GoldCase{
		ExcerptTermsBySection: map[string][]string{
			"ARTICLE 16": {"pouvoirs exceptionnels"},
			"ARTICLE 7":  {"élection du nouveau Président"},
		},
	}
	hits := []retrieveHit{
		{Section: "ARTICLE 16.", Article: "16", Excerpt: "ARTICLE 16. L'Assemblée nationale ne peut être dissoute pendant l'exercice des pouvoirs exceptionnels."},
		{Section: "ARTICLE 7.", Article: "7", Excerpt: "L'élection du nouveau Président a lieu vingt jours au moins et trente-cinq jours au plus avant l'expiration des pouvoirs du président en exercice."},
	}
	pass, missed := scoreExcerptTerms(gc, hits, 8)
	if !pass || len(missed) != 0 {
		t.Fatalf("expected pass, got pass=%v missed=%v", pass, missed)
	}
}

func TestScoreExcerptTermsHeadingOnlyFail(t *testing.T) {
	gc := GoldCase{
		ExcerptTermsBySection: map[string][]string{
			"ARTICLE 16": {"pouvoirs exceptionnels"},
		},
	}
	hits := []retrieveHit{
		{Section: "ARTICLE 16.", Article: "16", Excerpt: "ARTICLE 16"},
	}
	pass, missed := scoreExcerptTerms(gc, hits, 8)
	if pass || len(missed) == 0 {
		t.Fatalf("expected fail for heading-only excerpt, got pass=%v missed=%v", pass, missed)
	}
}

func TestScoreGenerationPhrases(t *testing.T) {
	gc := GoldCase{
		GenerationPhrases: []string{"dissoute", "article 16", "article 7"},
	}
	pass, missed := scoreGenerationPhrases(gc, "L'article 16 interdit que l'Assemblée nationale soit dissoute; l'article 7 prévoit le report.")
	if !pass || len(missed) != 0 {
		t.Fatalf("expected pass, got pass=%v missed=%v", pass, missed)
	}
	pass, missed = scoreGenerationPhrases(gc, "réponse sans mots-clés")
	if pass || len(missed) == 0 {
		t.Fatalf("expected fail, got pass=%v missed=%v", pass, missed)
	}
}
