package main

import (
	"sort"
	"strings"
	"unicode"

	"github.com/noematic-eu/ai-rag-agent/model"
)

type legalArticleAffinity struct {
	terms    []string
	articles []string
	bonus    float64
}

var legalArticleAffinities = []legalArticleAffinity{
	{terms: []string{"pleins", "pouvoirs", "exceptionnelles", "exceptionnel", "urgence", "mesures"}, articles: []string{"16"}, bonus: 0.35},
	{terms: []string{"election", "presidentielle", "scrutin", "report", "prorogation", "candidat"}, articles: []string{"7"}, bonus: 0.35},
	{terms: []string{"dissolution", "elections", "generales"}, articles: []string{"12"}, bonus: 0.35},
	{terms: []string{"veille", "continuite", "respect", "constitution", "arbitrage"}, articles: []string{"5"}, bonus: 0.35},
	{terms: []string{"referendum", "organisation", "pouvoirs", "publics"}, articles: []string{"11"}, bonus: 0.35},
	{terms: []string{"republique", "indivisible", "democratique"}, articles: []string{"1", "2"}, bonus: 0.20},
	{terms: []string{"obligations", "obligation", "devoirs", "devoir", "role", "fonction", "garant", "mission"}, articles: []string{"5"}, bonus: 0.45},
}

var legalRerankElectionTerms = map[string]bool{
	"election": true, "presidentielle": true, "scrutin": true,
	"report": true, "prorogation": true, "candidat": true, "suffrage": true,
}

func shouldLegalRerank(corpus string, enabled *bool, chunksByID map[string]model.Chunk) bool {
	if enabled != nil {
		return *enabled
	}
	if corpus == "legal-demo" {
		return true
	}
	for _, chunk := range chunksByID {
		if chunk.Metadata.Article != "" {
			return true
		}
	}
	return false
}

func parseLegalRerankParam(raw string) *bool {
	raw = strings.TrimSpace(strings.ToLower(raw))
	if raw == "" {
		return nil
	}
	switch raw {
	case "1", "true", "yes", "on":
		v := true
		return &v
	case "0", "false", "no", "off":
		v := false
		return &v
	default:
		return nil
	}
}

func normalizeLegalQueryTerms(query string) map[string]bool {
	query = strings.ToLower(query)
	var b strings.Builder
	for _, r := range query {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(foldLegalAccent(r))
		} else {
			b.WriteByte(' ')
		}
	}
	terms := make(map[string]bool)
	for _, w := range strings.Fields(b.String()) {
		if len(w) > 2 {
			terms[w] = true
		}
	}
	return terms
}

func hasPresidentTerm(terms map[string]bool) bool {
	return terms["president"] || terms["presidente"] || terms["presidentielle"]
}

func hasElectionTerm(terms map[string]bool) bool {
	for t := range terms {
		if legalRerankElectionTerms[t] {
			return true
		}
	}
	return false
}

func hasAbstractIntentTerm(terms map[string]bool) bool {
	for _, aff := range legalArticleAffinities {
		for _, term := range aff.terms {
			if term == "obligations" || term == "obligation" || term == "devoirs" || term == "devoir" ||
				term == "role" || term == "fonction" || term == "garant" || term == "mission" {
				if terms[term] {
					return true
				}
			}
		}
	}
	return false
}

func legalAffinityBonus(queryTerms map[string]bool, article string) float64 {
	if article == "" {
		return 0
	}
	var bonus float64
	for _, aff := range legalArticleAffinities {
		matched := false
		for _, term := range aff.terms {
			if queryTerms[term] {
				matched = true
				break
			}
		}
		if !matched {
			continue
		}
		for _, want := range aff.articles {
			if want == article {
				bonus += aff.bonus
				break
			}
		}
	}

	hasPresident := hasPresidentTerm(queryTerms)
	hasElection := hasElectionTerm(queryTerms)
	hasAbstract := hasAbstractIntentTerm(queryTerms)

	if hasPresident && hasAbstract && !hasElection && article == "5" {
		bonus += 0.45
	}
	if hasPresident && !hasElection && (article == "5" || article == "6") {
		bonus += 0.25
	}
	if queryTerms["peuple"] && !hasPresident && (article == "1" || article == "2" || article == "3") {
		bonus += 0.20
	}

	return bonus
}

func legalAffinityPenalty(queryTerms map[string]bool, article string) float64 {
	if queryTerms["peuple"] && hasPresidentTerm(queryTerms) {
		if article == "1" || article == "2" || article == "3" {
			return 0.15
		}
	}
	return 0
}

func sectionPathBonus(queryTerms map[string]bool, chunk model.Chunk) float64 {
	if !hasPresidentTerm(queryTerms) {
		return 0
	}
	path := strings.ToUpper(chunk.Metadata.SectionPath)
	if strings.Contains(path, "PRESIDENT DE LA REPUBLIQUE") || strings.Contains(path, "PRÉSIDENT DE LA RÉPUBLIQUE") {
		return 0.30
	}
	return 0
}

func queryChunkOverlapBonus(queryTerms map[string]bool, chunk model.Chunk) float64 {
	if len(queryTerms) == 0 {
		return 0
	}
	text := strings.ToLower(chunk.Metadata.Title + " " + stripNeighborContext(chunk.Text))
	if len(text) > 400 {
		text = text[:400]
	}
	var b strings.Builder
	for _, r := range text {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		} else {
			b.WriteByte(' ')
		}
	}
	chunkTerms := strings.Fields(b.String())
	if len(chunkTerms) == 0 {
		return 0
	}
	matches := 0
	for _, w := range chunkTerms {
		if queryTerms[w] {
			matches++
		}
	}
	return float64(matches) * 0.04
}

func rerankLegalArticles(sorted []chunkScore, chunksByID map[string]model.Chunk, query, corpus string, enabled *bool) []chunkScore {
	if !shouldLegalRerank(corpus, enabled, chunksByID) || len(sorted) == 0 {
		return sorted
	}
	queryTerms := normalizeLegalQueryTerms(query)
	out := make([]chunkScore, len(sorted))
	for i, hit := range sorted {
		chunk, ok := chunksByID[hit.ID]
		if !ok {
			out[i] = hit
			continue
		}
		bonus := legalAffinityBonus(queryTerms, chunk.Metadata.Article)
		bonus += queryChunkOverlapBonus(queryTerms, chunk)
		bonus += sectionPathBonus(queryTerms, chunk)
		penalty := legalAffinityPenalty(queryTerms, chunk.Metadata.Article)
		if bonus > 0 {
			hit.Score *= 1 + bonus
		}
		if penalty > 0 {
			hit.Score *= 1 - penalty
		}
		out[i] = hit
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Score > out[j].Score
	})
	return out
}
