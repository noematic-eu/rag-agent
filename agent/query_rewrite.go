package main

import (
	"context"
	"log"
	"strings"
)

const abstractQueryMaxLen = 200

var legalInstitutionQueryTerms = []string{
	"president", "parlement", "gouvernement", "senat", "depute", "ministre",
	"conseil", "constitutionnel",
}

var legalRoleIntentTerms = []string{
	"obligations", "obligation", "devoirs", "devoir", "role", "fonction",
	"pouvoirs", "pouvoir", "competences", "competence", "mission",
	"responsabilites", "responsabilite", "garant",
}

func parseRewriteParam(raw string, corpus string) bool {
	raw = strings.TrimSpace(strings.ToLower(raw))
	switch raw {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	default:
		return corpus == "legal-demo" || strings.Contains(strings.ToLower(corpus), "legal")
	}
}

func isAbstractLegalQuery(query string) bool {
	query = strings.TrimSpace(query)
	if query == "" || len(query) >= abstractQueryMaxLen {
		return false
	}
	if len(articleRefsFromQuery(query)) > 0 {
		return false
	}
	terms := normalizeLegalExpandTerms(query)
	hasInstitution := hasAnyTerm(terms, legalInstitutionQueryTerms)
	hasRoleIntent := hasAnyTerm(terms, legalRoleIntentTerms)
	return hasInstitution || hasRoleIntent
}

func rewriteQueryPromptFR(generationQuery string) string {
	return "Tu reformules une question juridique pour la recherche dans la Constitution française.\n" +
		"Produis 2 requêtes courtes (mots-clés, pas de phrase complète) qui maximisent\n" +
		"la similarité avec les articles pertinents. Nomme les articles probables si tu les connais.\n" +
		"Ne réponds pas à la question.\n\n" +
		"Question : " + generationQuery + "\n" +
		"Requête 1 :\n" +
		"Requête 2 :"
}

func parseRewriteQueries(raw string) []string {
	lines := strings.Split(raw, "\n")
	var out []string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		lower := strings.ToLower(line)
		if strings.HasPrefix(lower, "requête") || strings.HasPrefix(lower, "requete") {
			if idx := strings.Index(line, ":"); idx >= 0 {
				line = strings.TrimSpace(line[idx+1:])
			}
		}
		if len(line) < 4 {
			continue
		}
		out = append(out, line)
	}
	return out
}

func buildRetrievalQueries(generationQuery, explicitRetrieval, corpus string, rewriteEnabled bool) []string {
	if explicitRetrieval != "" {
		return []string{explicitRetrieval}
	}

	queries := []string{generationQuery}
	if shouldExpandLegalQuery(corpus) {
		queries = expandLegalQuery(generationQuery)
	}

	if !rewriteEnabled || !isAbstractLegalQuery(generationQuery) {
		return dedupeQueries(queries)
	}

	ctx := context.Background()
	raw, err := completeLLM(ctx, "", rewriteQueryPromptFR(generationQuery))
	if err != nil {
		log.Printf("query rewrite: LLM failed: %v", err)
		return dedupeQueries(queries)
	}
	for _, q := range parseRewriteQueries(raw) {
		queries = append(queries, q)
	}
	return dedupeQueries(queries)
}

func dedupeQueries(queries []string) []string {
	seen := make(map[string]struct{}, len(queries))
	out := make([]string, 0, len(queries))
	for _, q := range queries {
		q = strings.TrimSpace(q)
		if q == "" {
			continue
		}
		key := strings.ToLower(q)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, q)
	}
	return out
}

func formatRetrievalQueriesDebug(queries []string) string {
	return strings.Join(queries, " | ")
}
