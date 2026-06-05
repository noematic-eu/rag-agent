package p9fs

import (
	"bufio"
	"strings"
)

func parseParamsText(text string) map[string]string {
	out := make(map[string]string)
	scanner := bufio.NewScanner(strings.NewReader(text))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, val, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		out[strings.TrimSpace(key)] = strings.TrimSpace(val)
	}
	return out
}

func formatParamsText(params map[string]string) string {
	if len(params) == 0 {
		return ""
	}
	keys := []string{"rq", "retrieval_q", "corpus", "doc_id", "top_k", "bm25_k", "vector_k", "fusion", "min_score", "lang"}
	var b strings.Builder
	for _, k := range keys {
		if v, ok := params[k]; ok && v != "" {
			b.WriteString(k)
			b.WriteByte('=')
			b.WriteString(v)
			b.WriteByte('\n')
		}
	}
	return b.String()
}
