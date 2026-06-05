package main

import (
	"encoding/json"
	"regexp"
	"strings"

	"github.com/noematic-eu/ai-rag-agent/model"
)

var articleRefInQueryRe = regexp.MustCompile(`(?i)article\s+(premier|\d+)`)

func articleRefsFromQuery(query string) []string {
	matches := articleRefInQueryRe.FindAllStringSubmatch(query, -1)
	if len(matches) == 0 {
		return nil
	}
	seen := make(map[string]struct{})
	var refs []string
	for _, m := range matches {
		ref := strings.ToUpper(strings.TrimSpace(m[1]))
		if ref == "PREMIER" {
			ref = "1"
		}
		if ref == "" {
			continue
		}
		if _, ok := seen[ref]; ok {
			continue
		}
		seen[ref] = struct{}{}
		refs = append(refs, ref)
	}
	return refs
}

func boostArticleChunks(sorted []chunkScore, chunksByID map[string]model.Chunk, articleRefs []string, corpus string) []chunkScore {
	if len(articleRefs) == 0 || chunkStore == nil {
		return sorted
	}
	existing := make(map[string]struct{}, len(sorted))
	for _, c := range sorted {
		existing[c.ID] = struct{}{}
	}

	pairs, err := chunkStore.ScanPrefix("chunk:")
	if err != nil {
		return sorted
	}

	boostScore := 0.0
	if len(sorted) > 0 {
		boostScore = sorted[0].Score
	}
	if boostScore <= 0 {
		boostScore = 1.0
	}

	for _, pair := range pairs {
		var chunk model.Chunk
		if err := json.Unmarshal(pair.Value, &chunk); err != nil {
			continue
		}
		if corpus != "" && chunk.Metadata.Corpus != corpus {
			continue
		}
		if chunk.Metadata.Article == "" {
			continue
		}
		for _, ref := range articleRefs {
			if !strings.EqualFold(chunk.Metadata.Article, ref) {
				continue
			}
			id := chunk.Metadata.ChunkID
			chunksByID[id] = chunk
			if _, ok := existing[id]; ok {
				break
			}
			sorted = append(sorted, chunkScore{ID: id, Score: boostScore * 0.95})
			existing[id] = struct{}{}
			break
		}
	}
	return sorted
}
