package main

import (
	"log"
	"regexp"
	"strings"

	"github.com/gin-gonic/gin"
)

// parseSearchQueries returns the text used for retrieval (Bleve/embeddings) and the full user question for the LLM.
// explicitRQ is true when the client passed retrieval_q/rq (skips auto rewrite/expansion).
func parseSearchQueries(c *gin.Context, queryText string) (retrievalText, generationText string, explicitRQ bool) {
	generationText = queryText
	retrievalText = strings.TrimSpace(c.Query("retrieval_q"))
	if retrievalText == "" {
		retrievalText = strings.TrimSpace(c.Query("rq"))
	}
	if retrievalText != "" {
		return retrievalText, generationText, true
	}

	retrievalText = stripInstructionPrefixForRetrieval(queryText)
	if retrievalText != queryText {
		log.Printf("search: using stripped retrieval query (%d chars) for BM25", len(retrievalText))
	}
	return "", generationText, false
}

const retrievalQueryHeuristicMinLen = 120

var instructionPrefixPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)^from the excerpts only[,:]?\s*`),
	regexp.MustCompile(`(?i)^extrais uniquement (?:des |d')?extraits[,:]?\s*`),
	regexp.MustCompile(`(?i)^list \d+ `),
}

func stripInstructionPrefixForRetrieval(query string) string {
	q := strings.TrimSpace(query)
	if len(q) < retrievalQueryHeuristicMinLen {
		return q
	}
	lower := strings.ToLower(q)
	for _, phrase := range []string{
		"from the excerpts only",
		"from excerpts only",
		"extrais uniquement des extraits",
		"extrais uniquement",
	} {
		if idx := strings.Index(lower, phrase); idx >= 0 && idx < 80 {
			rest := strings.TrimSpace(q[idx+len(phrase):])
			rest = strings.TrimLeft(rest, ":,. ")
			if len(rest) >= 8 {
				return rest
			}
		}
	}
	for _, re := range instructionPrefixPatterns {
		if loc := re.FindStringIndex(q); loc != nil && loc[0] == 0 {
			rest := strings.TrimSpace(q[loc[1]:])
			if len(rest) >= 8 {
				return rest
			}
		}
	}
	return q
}
