package main

import (
	"sort"

	"github.com/noematic-eu/ai-rag-agent/model"
)

const multiQueryTopKMultiplier = 12

// rankChunksMulti runs retrieval for each query and fuses results with RRF.
func rankChunksMulti(queries []string, p rankParams) (rankOutcome, error) {
	if len(queries) == 0 {
		return rankOutcome{noResults: true}, nil
	}
	if len(queries) == 1 {
		p.retrievalText = queries[0]
		return rankChunks(p)
	}

	perQueryTopK := p.topKFinal
	if perQueryTopK < multiQueryTopKMultiplier {
		perQueryTopK = multiQueryTopKMultiplier
	}

	rankLists := make([]map[string]int, 0, len(queries))
	mergedChunks := make(map[string]model.Chunk)
	var primaryTopScore float64
	var lexicalSource string

	for i, q := range queries {
		qp := p
		qp.retrievalText = q
		qp.topKFinal = perQueryTopK
		outcome, err := rankChunks(qp)
		if err != nil {
			return rankOutcome{}, err
		}
		if i == 0 {
			lexicalSource = outcome.lexicalSource
			if len(outcome.hits) > 0 {
				primaryTopScore = outcome.hits[0].Score
			}
		}
		for id, chunk := range outcome.chunksByID {
			mergedChunks[id] = chunk
		}
		ordered := make([]string, 0, len(outcome.hits))
		for _, hit := range outcome.hits {
			ordered = append(ordered, hit.ChunkID)
		}
		rankLists = append(rankLists, rankMap(nil, ordered))
	}

	if len(rankLists) == 0 {
		return rankOutcome{noResults: true, chunksByID: mergedChunks}, nil
	}

	fused := fuseMultiQueryRRF(rankLists)
	if len(fused) > p.topKFinal {
		fused = fused[:p.topKFinal]
	}

	if len(fused) == 0 {
		if p.docID != "" {
			if fallback, ok := fallbackDocChunks(p.docID, p.corpus, p.topKFinal); ok {
				return fallback, nil
			}
		}
		return rankOutcome{noResults: true, chunksByID: mergedChunks}, nil
	}

	hits := make([]model.RetrieveHit, 0, len(fused))
	for _, hit := range fused {
		chunk, ok := mergedChunks[hit.ID]
		if !ok {
			var loadErr error
			chunk, loadErr = loadChunkByID(hit.ID)
			if loadErr != nil {
				continue
			}
			mergedChunks[hit.ID] = chunk
		}
		section := chunk.Metadata.Title
		if section == "" {
			section = chunk.Metadata.SectionPath
		}
		hits = append(hits, model.RetrieveHit{
			ChunkID:     chunk.Metadata.ChunkID,
			DocID:       chunk.Metadata.DocID,
			Score:       hit.Score,
			Corpus:      chunk.Metadata.Corpus,
			Section:     section,
			DocTitle:    chunk.Metadata.DocTitle,
			SectionPath: chunk.Metadata.SectionPath,
			Article:     chunk.Metadata.Article,
		})
	}

	if len(hits) == 0 || primaryTopScore < p.minScore {
		if p.docID != "" {
			if fallback, ok := fallbackDocChunks(p.docID, p.corpus, p.topKFinal); ok {
				return fallback, nil
			}
		}
		return rankOutcome{noResults: true, chunksByID: mergedChunks}, nil
	}
	return rankOutcome{hits: hits, chunksByID: mergedChunks, lexicalSource: lexicalSource}, nil
}

func fuseMultiQueryRRF(rankLists []map[string]int) []chunkScore {
	scores := make(map[string]float64)
	for _, ranks := range rankLists {
		for id, rank := range ranks {
			scores[id] += 1.0 / (rrfK + float64(rank))
		}
	}
	out := make([]chunkScore, 0, len(scores))
	for id, score := range scores {
		out = append(out, chunkScore{ID: id, Score: score})
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Score > out[j].Score
	})
	return out
}
