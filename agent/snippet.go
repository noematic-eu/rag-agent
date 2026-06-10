package main

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

var legalArticleHeadingSentenceRe = regexp.MustCompile(`(?i)^ARTICLE\s+(?:PREMIER|\d+)\.?\s*$`)

const maxLegalSnippetChars = 2200
const legalExcerptHeadChars = 1200

var snippetPriorityLegalTerms = []string{
	"pouvoirs", "exceptionnels", "exceptionnel", "mesures", "exigees", "dissoute",
	"dissolution", "report", "reporter", "prorog", "election", "president", "presidentielle",
	"scrutin", "conseil", "constitutionnel", "urgence", "emergency",
}

var snippetGenericLegalTokens = map[string]bool{
	"article": true,
}

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
		rest := content[idx+len(neighborContextPrev):]
		if artIdx := strings.Index(strings.ToUpper(rest), "ARTICLE "); artIdx >= 0 {
			content = strings.TrimSpace(rest[artIdx:])
		} else if end := strings.Index(rest, "\n\n"); end >= 0 {
			content = strings.TrimSpace(rest[end+2:])
		}
	}
	if idx := strings.Index(content, neighborContextNext); idx >= 0 {
		content = strings.TrimSpace(content[:idx])
	}
	return content
}

func queryTokenSet(query string) map[string]bool {
	queryTokens := tokenizeV2(query)
	querySet := make(map[string]bool, len(queryTokens))
	for _, token := range queryTokens {
		if snippetGenericLegalTokens[token] {
			continue
		}
		querySet[token] = true
	}
	return querySet
}

func isLegalArticleContent(content string) bool {
	return legalArticleDetectRe.MatchString(content)
}

func anchorLegalArticleContent(content, article string) string {
	content = strings.TrimSpace(content)
	if content == "" || article == "" {
		return content
	}
	upper := strings.ToUpper(content)
	if article == "1" {
		if idx := strings.Index(upper, "ARTICLE PREMIER"); idx >= 0 {
			return strings.TrimSpace(content[idx:])
		}
	}
	marker := strings.ToUpper(fmt.Sprintf("ARTICLE %s", article))
	if idx := strings.Index(upper, marker); idx >= 0 {
		return strings.TrimSpace(content[idx:])
	}
	return content
}

// excerptTextForChunk builds an excerpt for generation or /retrieve display.
func excerptTextForChunk(content, retrievalQuery, article string, maxLength int) string {
	limit := maxLength
	if strings.TrimSpace(article) != "" {
		limit = maxLegalSnippetChars
	}
	return excerptTextAnchored(content, retrievalQuery, limit, article)
}

func excerptText(content, retrievalQuery string, maxLength int) string {
	return excerptTextAnchored(content, retrievalQuery, maxLength, "")
}

func excerptTextAnchored(content, retrievalQuery string, maxLength int, article string) string {
	content = stripNeighborContext(strings.TrimSpace(content))
	content = stripCopyrightLines(content)
	if content == "" {
		return ""
	}
	content = anchorLegalArticleContent(content, article)
	if len(content) <= maxLength {
		return content
	}
	if isLegalArticleContent(content) || article != "" {
		return legalArticleExcerpt(content, retrievalQuery, maxLength)
	}
	return extractSnippet(content, retrievalQuery, maxLength)
}

func isLegalHeadingOnlySentence(sentence string) bool {
	return legalArticleHeadingSentenceRe.MatchString(strings.TrimSpace(sentence))
}

func trimToMaxLength(s string, maxLength int) string {
	if len(s) <= maxLength {
		return s
	}
	return s[:maxLength] + "..."
}

func sentenceMatchesPriorityTerms(sentence string) bool {
	lower := strings.ToLower(sentence)
	for _, term := range snippetPriorityLegalTerms {
		if strings.Contains(lower, term) {
			return true
		}
	}
	return false
}

func prioritySentencesExcerpt(content string, budget int, exclude string) string {
	if budget <= 0 {
		return ""
	}
	excludeLower := strings.ToLower(exclude)
	var picked []string
	used := 0
	for _, sentence := range strings.Split(content, ". ") {
		sentence = strings.TrimSpace(sentence)
		if sentence == "" || isLegalHeadingOnlySentence(sentence) {
			continue
		}
		if strings.Contains(excludeLower, strings.ToLower(sentence)) {
			continue
		}
		if !sentenceMatchesPriorityTerms(sentence) {
			continue
		}
		part := sentence
		sep := 0
		if len(picked) > 0 {
			sep = 2
		}
		if used+sep+len(part) > budget {
			break
		}
		picked = append(picked, part)
		used += sep + len(part)
		if len(picked) >= maxLegalSnippetSentences {
			break
		}
	}
	return strings.Join(picked, ". ")
}

// legalArticleExcerpt returns anchored legal text, merging head and priority tail when needed.
func legalArticleExcerpt(content, query string, maxLength int) string {
	if len(content) <= maxLength {
		return content
	}
	head := content
	if len(head) > legalExcerptHeadChars {
		head = head[:legalExcerptHeadChars]
	}
	tailBudget := maxLength - len(head) - 2
	if tailBudget > 80 {
		tail := prioritySentencesExcerpt(content, tailBudget, head)
		if tail != "" {
			merged := head + "\n\n" + tail
			if len(merged) <= maxLength {
				return merged
			}
			return trimToMaxLength(merged, maxLength)
		}
	}
	return trimToMaxLength(content, maxLength)
}

type scoredSentence struct {
	text  string
	score int
}

const maxLegalSnippetSentences = 3

func sentenceScore(sentence string, querySet map[string]bool) int {
	score := 0
	for _, token := range tokenizeV2(sentence) {
		if querySet[token] {
			score++
		}
	}
	lower := strings.ToLower(sentence)
	for _, term := range snippetPriorityLegalTerms {
		if strings.Contains(lower, term) {
			score++
		}
	}
	return score
}

// extractSnippet extracts a snippet from content based on query, up to maxLength.
func extractSnippet(content, query string, maxLength int) string {
	querySet := queryTokenSet(query)
	if len(querySet) == 0 {
		return trimToMaxLength(content, maxLength)
	}

	sentences := strings.Split(content, ". ")
	scored := make([]scoredSentence, 0, len(sentences))
	for _, sentence := range sentences {
		sentence = strings.TrimSpace(sentence)
		if sentence == "" || isLegalHeadingOnlySentence(sentence) {
			continue
		}
		scored = append(scored, scoredSentence{text: sentence, score: sentenceScore(sentence, querySet)})
	}

	if len(scored) == 0 {
		return legalArticleExcerpt(content, query, maxLength)
	}

	sort.Slice(scored, func(i, j int) bool {
		if scored[i].score != scored[j].score {
			return scored[i].score > scored[j].score
		}
		return len(scored[i].text) > len(scored[j].text)
	})

	if scored[0].score == 0 {
		return legalArticleExcerpt(content, query, maxLength)
	}

	var parts []string
	used := 0
	for _, s := range scored {
		if s.score <= 0 {
			break
		}
		sepLen := 0
		if len(parts) > 0 {
			sepLen = 2
		}
		if used+sepLen+len(s.text) > maxLength {
			if len(parts) == 0 {
				return trimToMaxLength(s.text, maxLength)
			}
			break
		}
		parts = append(parts, s.text)
		used += sepLen + len(s.text)
		if len(parts) >= maxLegalSnippetSentences {
			break
		}
	}
	if len(parts) == 0 {
		return legalArticleExcerpt(content, query, maxLength)
	}
	return strings.Join(parts, ". ")
}
