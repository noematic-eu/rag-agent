package lexical

import (
	"regexp"
	"strings"
)

var tokenCleaner = regexp.MustCompile(`[^\p{L}\p{N}]+`)

// Tokenize splits text into normalized terms for BM25 (same rules as agent/index.go tokenize).
func Tokenize(text string) []string {
	words := strings.Fields(strings.ToLower(text))
	out := make([]string, 0, len(words))
	for i := 0; i < len(words); i++ {
		cleaned := tokenCleaner.ReplaceAllString(words[i], "")
		if cleaned == "" {
			continue
		}
		if len(cleaned) > 2 {
			next := ""
			if i+1 < len(words) {
				next = tokenCleaner.ReplaceAllString(words[i+1], "")
			}
			if next != "" && isLegalPhrase(cleaned, next) {
				out = append(out, cleaned+" "+next)
				i++
				continue
			}
		}
		out = append(out, cleaned)
	}
	return out
}

func isLegalPhrase(a, b string) bool {
	phrases := map[string]bool{
		"article premier": true, "article 1er": true,
	}
	return phrases[a+" "+b]
}
