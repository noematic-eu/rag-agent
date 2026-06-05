package main

import (
	"encoding/json"
	"sort"
	"strings"

	"github.com/noematic-eu/ai-rag-agent/model"
)

func filterChunksByArticle(chunks []chunkScore, chunksByID map[string]model.Chunk, article string) []chunkScore {
	article = strings.TrimSpace(article)
	if article == "" {
		return chunks
	}
	want := strings.ToUpper(article)
	out := make([]chunkScore, 0, len(chunks))
	for _, c := range chunks {
		chunk, ok := chunksByID[c.ID]
		if !ok {
			var err error
			chunk, err = loadChunkByID(c.ID)
			if err != nil {
				continue
			}
			chunksByID[c.ID] = chunk
		}
		if chunk.Metadata.Article != "" && strings.EqualFold(chunk.Metadata.Article, article) {
			out = append(out, c)
			continue
		}
		title := strings.ToUpper(chunk.Metadata.Title)
		if strings.Contains(title, "ARTICLE "+want) || strings.Contains(title, "ARTICLE PREMIER") && want == "1" {
			out = append(out, c)
		}
	}
	return out
}

func filterChunksByDocID(chunks []chunkScore, chunksByID map[string]model.Chunk, docID string) []chunkScore {
	if docID == "" {
		return chunks
	}
	out := make([]chunkScore, 0, len(chunks))
	for _, c := range chunks {
		chunk, ok := chunksByID[c.ID]
		if !ok {
			var err error
			chunk, err = loadChunkByID(c.ID)
			if err != nil {
				continue
			}
			chunksByID[c.ID] = chunk
		}
		if chunk.Metadata.DocID == docID {
			out = append(out, c)
		}
	}
	return out
}

func fallbackDocChunks(docID, corpus string, topK int) (rankOutcome, bool) {
	if chunkStore == nil || docID == "" {
		return rankOutcome{}, false
	}
	if topK <= 0 {
		topK = 6
	}

	pairs, err := chunkStore.ScanPrefix("chunk:")
	if err != nil {
		return rankOutcome{}, false
	}

	type positioned struct {
		chunk model.Chunk
		pos   int
	}
	var matches []positioned
	for _, pair := range pairs {
		var chunk model.Chunk
		if err := json.Unmarshal(pair.Value, &chunk); err != nil {
			continue
		}
		if chunk.Metadata.DocID != docID {
			continue
		}
		if corpus != "" && chunk.Metadata.Corpus != corpus {
			continue
		}
		matches = append(matches, positioned{chunk: chunk, pos: chunk.Metadata.Position})
	}
	if len(matches) == 0 {
		return rankOutcome{}, false
	}

	sort.Slice(matches, func(i, j int) bool {
		return matches[i].pos < matches[j].pos
	})
	if len(matches) > topK {
		matches = matches[:topK]
	}

	chunksByID := make(map[string]model.Chunk, len(matches))
	hits := make([]model.RetrieveHit, 0, len(matches))
	for i, m := range matches {
		chunksByID[m.chunk.Metadata.ChunkID] = m.chunk
		section := m.chunk.Metadata.Title
		if section == "" {
			section = m.chunk.Metadata.SectionPath
		}
		hits = append(hits, model.RetrieveHit{
			ChunkID:     m.chunk.Metadata.ChunkID,
			DocID:       m.chunk.Metadata.DocID,
			Score:       1.0 / float64(i+1),
			Corpus:      m.chunk.Metadata.Corpus,
			Section:     section,
			DocTitle:    m.chunk.Metadata.DocTitle,
			SectionPath: m.chunk.Metadata.SectionPath,
		})
	}
	return rankOutcome{hits: hits, chunksByID: chunksByID}, true
}
