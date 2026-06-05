package main

import (
	"strings"
	"unicode"
)

var legalInstitutionTerms = []string{
	"president", "parlement", "gouvernement", "conseil", "constitutionnel",
	"senat", "depute", "ministre", "premier",
}

var legalAbstractIntentTerms = []string{
	"obligations", "obligation", "devoirs", "devoir", "role", "fonction",
	"pouvoirs", "pouvoir", "garant", "competences", "competence", "mission",
	"responsabilites", "responsabilite",
}

var legalElectionTerms = []string{
	"election", "scrutin", "suffrage", "candidat", "mandat", "report", "prorogation",
}

func foldLegalAccent(r rune) rune {
	switch r {
	case 'à', 'â', 'ä', 'á', 'ã', 'å':
		return 'a'
	case 'ç':
		return 'c'
	case 'é', 'è', 'ê', 'ë':
		return 'e'
	case 'î', 'ï', 'í':
		return 'i'
	case 'ô', 'ö', 'ó', 'õ':
		return 'o'
	case 'ù', 'û', 'ü', 'ú':
		return 'u'
	case 'œ':
		return 'o'
	default:
		return r
	}
}

func normalizeLegalExpandTerms(query string) map[string]bool {
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

func hasAnyTerm(terms map[string]bool, candidates []string) bool {
	for _, c := range candidates {
		if terms[c] {
			return true
		}
	}
	return false
}

// expandLegalQuery returns retrieval query variants for legal corpora.
// The original query is always first; expansions are appended when patterns match.
func expandLegalQuery(query string) []string {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil
	}
	out := []string{query}
	terms := normalizeLegalExpandTerms(query)

	hasPresident := terms["president"] || terms["presidente"] || terms["presidentielle"]
	hasAbstract := hasAnyTerm(terms, legalAbstractIntentTerms)
	hasElection := hasAnyTerm(terms, legalElectionTerms)

	if hasPresident && hasAbstract && !hasElection {
		out = append(out, query+" article 5 veille constitution arbitrage continuite garant independance traites")
	}
	if hasPresident && hasElection {
		out = append(out, query+" article 7 election presidentielle scrutin organisation")
	}
	if terms["parlement"] && hasAbstract {
		out = append(out, query+" article 24 parlement loi controle gouvernement")
	}
	if terms["gouvernement"] && hasAbstract {
		out = append(out, query+" article 20 gouvernement responsable devant parlement")
	}

	seen := make(map[string]struct{}, len(out))
	unique := make([]string, 0, len(out))
	for _, q := range out {
		q = strings.TrimSpace(q)
		if q == "" {
			continue
		}
		key := strings.ToLower(q)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		unique = append(unique, q)
	}
	return unique
}

func shouldExpandLegalQuery(corpus string) bool {
	return corpus == "legal-demo" || strings.Contains(strings.ToLower(corpus), "legal")
}
