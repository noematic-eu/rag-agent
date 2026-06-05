package main

import (
	"sort"
	"strings"

	"github.com/noematic-eu/ai-rag-agent/model"
)

const rrfK = 60.0

type chunkScore struct {
	ID    string
	Score float64
}

func fuseWeighted(bm25Scores, vectorScores map[string]float64, maxBM25, maxVector, weight float64) []chunkScore {
	ids := make(map[string]struct{})
	for id := range bm25Scores {
		ids[id] = struct{}{}
	}
	for id := range vectorScores {
		ids[id] = struct{}{}
	}

	out := make([]chunkScore, 0, len(ids))
	for id := range ids {
		bm25 := 0.0
		vector := 0.0
		if score, ok := bm25Scores[id]; ok && maxBM25 > 0 {
			bm25 = score / maxBM25
		}
		if score, ok := vectorScores[id]; ok && maxVector > 0 {
			vector = score / maxVector
		}
		hybrid := weight*bm25 + (1-weight)*vector
		out = append(out, chunkScore{ID: id, Score: hybrid})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Score > out[j].Score })
	return out
}

func fuseRRF(bm25Rank, vectorRank map[string]int) []chunkScore {
	ids := make(map[string]struct{})
	for id := range bm25Rank {
		ids[id] = struct{}{}
	}
	for id := range vectorRank {
		ids[id] = struct{}{}
	}

	out := make([]chunkScore, 0, len(ids))
	for id := range ids {
		score := 0.0
		if rank, ok := bm25Rank[id]; ok {
			score += 1.0 / (rrfK + float64(rank))
		}
		if rank, ok := vectorRank[id]; ok {
			score += 1.0 / (rrfK + float64(rank))
		}
		out = append(out, chunkScore{ID: id, Score: score})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Score > out[j].Score })
	return out
}

func rankMap(scores map[string]float64, orderedIDs []string) map[string]int {
	ranks := make(map[string]int, len(orderedIDs))
	for i, id := range orderedIDs {
		ranks[id] = i + 1
	}
	for id := range scores {
		if _, ok := ranks[id]; !ok {
			ranks[id] = len(orderedIDs) + 1
		}
	}
	return ranks
}

func dedupeByDocID(hits []chunkScore, chunksByID map[string]model.Chunk, maxPerDoc int) []chunkScore {
	if maxPerDoc <= 0 {
		return hits
	}
	perDoc := make(map[string]int)
	out := make([]chunkScore, 0, len(hits))
	for _, hit := range hits {
		docID := hit.ID
		if chunk, ok := chunksByID[hit.ID]; ok {
			docID = chunk.Metadata.DocID
		}
		if perDoc[docID] >= maxPerDoc {
			continue
		}
		perDoc[docID]++
		out = append(out, hit)
	}
	return out
}

// lengthScorePenalty down-weights very short chunks in weighted fusion.
func lengthScorePenalty(chunkID string, chunksByID map[string]model.Chunk) float64 {
	chunk, ok := chunksByID[chunkID]
	if !ok {
		return 1.0
	}
	n := len([]rune(strings.TrimSpace(chunk.Text)))
	if n >= minChunkChars {
		return 1.0
	}
	return float64(n) / float64(minChunkChars)
}
