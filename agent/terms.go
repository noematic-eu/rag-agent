package main

import (
	"regexp"
	"strings"
)

// Predefined set of common legal terms
var legalTerms = map[string]bool{
	"court":     true,
	"law":       true,
	"statute":   true,
	"plaintiff": true,
	"defendant": true,
	"contract":  true,
	// Add more terms as needed
}

// Regular expressions for legal term patterns
var legalPatterns = []*regexp.Regexp{
	regexp.MustCompile(`^article \d+$`),
	regexp.MustCompile(`^section \w+$`),
	// Add more patterns as needed
}

// isLegalTerm checks if a word is a legal term
func isLegalTerm(word string) bool {
	word = strings.ToLower(word)
	if legalTerms[word] {
		return true
	}
	for _, pattern := range legalPatterns {
		if pattern.MatchString(word) {
			return true
		}
	}
	return false
}

// Predefined set of common legal phrases
var legalPhrases = map[string]bool{
	"supreme court":      true,
	"due process":        true,
	"civil rights":       true,
	"power of attorney":  true,
	"breach of contract": true,
	// Add more phrases as needed
}

// isLegalPhrase checks if two words form a legal phrase
func isLegalPhrase(word1, word2 string) bool {
	phrase := strings.ToLower(word1 + " " + word2)
	return legalPhrases[phrase]
}
