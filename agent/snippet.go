package main

import (
	"strings"
)

var snippetStopwords = map[string]bool{
	"a": true, "an": true, "the": true, "and": true, "or": true, "but": true,
	"in": true, "on": true, "at": true, "to": true, "for": true, "of": true,
	"with": true, "from": true, "by": true, "as": true, "is": true, "are": true,
	"was": true, "were": true, "be": true, "been": true, "being": true,
	"that": true, "this": true, "these": true, "those": true, "it": true,
	"its": true, "you": true, "your": true, "do": true, "not": true, "only": true,
	"list": true, "cite": true, "citations": true, "excerpts": true, "excerpt": true,
	"quote": true, "paraphrase": true, "bullet": true, "bullets": true,
	"extraits": true, "extrait": true, "uniquement": true, "des": true, "les": true,
}

// tokenize splits text into lowercase words, removing basic punctuation
func tokenizeV2(text string) []string {
	text = strings.ToLower(text)
	for _, punct := range []string{".", ",", ";", "!", "?"} {
		text = strings.ReplaceAll(text, punct, " ")
	}
	words := strings.Fields(text)
	out := make([]string, 0, len(words))
	for _, w := range words {
		if snippetStopwords[w] {
			continue
		}
		out = append(out, w)
	}
	return out
}

const neighborContextPrev = "[Contexte précédent]"
const neighborContextNext = "[Contexte suivant]"

// stripNeighborContext removes injected neighbor-article context from legal chunks.
func stripNeighborContext(content string) string {
	content = strings.TrimSpace(content)
	if content == "" {
		return ""
	}
	if idx := strings.Index(content, neighborContextPrev); idx >= 0 {
		if end := strings.Index(content[idx:], "\n\n"); end >= 0 {
			content = strings.TrimSpace(content[idx+end+2:])
		}
	}
	if idx := strings.Index(content, neighborContextNext); idx >= 0 {
		content = strings.TrimSpace(content[:idx])
	}
	return content
}

// excerptText returns full content when short enough, otherwise a query-focused snippet.
func excerptText(content, retrievalQuery string, maxLength int) string {
	content = stripNeighborContext(strings.TrimSpace(content))
	if content == "" {
		return ""
	}
	if len(content) <= maxLength {
		return content
	}
	return extractSnippet(content, retrievalQuery, maxLength)
}

// extractSnippet extracts a snippet from content based on query, up to maxLength
func extractSnippet(content, query string, maxLength int) string {
	queryTokens := tokenizeV2(query)
	if len(queryTokens) == 0 {
		if len(content) <= maxLength {
			return content
		}
		return content[:maxLength] + "..."
	}
	querySet := make(map[string]bool, len(queryTokens))
	for _, token := range queryTokens {
		querySet[token] = true
	}

	sentences := strings.Split(content, ". ")
	var bestSentence string
	maxScore := 0

	for _, sentence := range sentences {
		sentenceTokens := tokenizeV2(sentence)
		score := 0
		for _, token := range sentenceTokens {
			if querySet[token] {
				score++
			}
		}
		if score > maxScore {
			maxScore = score
			bestSentence = sentence
		}
	}

	if maxScore == 0 && len(content) > 0 {
		if len(content) <= maxLength {
			return content
		}
		return content[:maxLength] + "..."
	}

	if len(bestSentence) <= maxLength {
		return bestSentence
	}
	return bestSentence[:maxLength] + "..."
}
