package lexical

import (
	"strings"
	"unicode/utf8"
)

const (
	diskPrefixMeta    = "lex:meta"
	diskPrefixDF      = "lex:df:"
	diskPrefixPost    = "lex:post:"
	diskPrefixChunk   = "lex:chunk:"
	diskPrefixTerms   = "lex:terms:"
	diskPrefixAll     = "lex:"
	diskMetaVersion   = 1
	diskMaxTermLen    = 64
	diskMaxCandidates = 10000
)

func diskDFKey(term string) string   { return diskPrefixDF + term }
func diskPostKey(term string) string { return diskPrefixPost + term }
func diskChunkKey(id string) string  { return diskPrefixChunk + id }
func diskTermsKey(id string) string  { return diskPrefixTerms + id }

func diskNormalizeTerm(term string) string {
	term = strings.ToValidUTF8(strings.ToLower(strings.TrimSpace(term)), "")
	if term == "" {
		return ""
	}
	// f4kvs keys must be valid UTF-8; byte truncation can split multibyte runes.
	runes := []rune(term)
	if len(runes) > diskMaxTermLen {
		term = string(runes[:diskMaxTermLen])
	}
	if !utf8.ValidString(term) {
		return ""
	}
	return term
}
