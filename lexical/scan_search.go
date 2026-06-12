package lexical

import (
	"sort"

	"github.com/noematic-eu/ai-rag-agent/model"
)

// SearchBM25Scan scores chunks from scan with in-memory BM25 (O(n), no lex:* index).
func SearchBM25Scan(scan func(yield func(model.Chunk) error) error, text, corpus string, k int, maxChunks int) ([]Hit, error) {
	query := Tokenize(text)
	if len(query) == 0 {
		return nil, nil
	}

	g := BM25Global{DF: make(map[string]int)}
	var chunks []BM25Chunk
	n := 0
	err := scan(func(chunk model.Chunk) error {
		f := FieldsFromChunk(chunk)
		if f.ChunkID == "" || f.Text == "" {
			return nil
		}
		if corpus != "" && f.Corpus != corpus {
			return nil
		}
		if maxChunks > 0 && n >= maxChunks {
			return nil
		}
		registerChunkFields(&g, &chunks, f)
		n++
		return nil
	})
	if err != nil {
		return nil, err
	}
	if len(chunks) == 0 {
		return nil, nil
	}

	type scored struct {
		id    string
		score float64
	}
	scores := make([]scored, 0, len(chunks))
	for _, c := range chunks {
		s := ScoreChunkBM25(c, query, &g)
		if s > 0 {
			scores = append(scores, scored{id: c.Fields.ChunkID, score: s})
		}
	}
	sort.Slice(scores, func(i, j int) bool { return scores[i].score > scores[j].score })
	if k > 0 && len(scores) > k {
		scores = scores[:k]
	}
	hits := make([]Hit, len(scores))
	for i, s := range scores {
		hits[i] = Hit{ChunkID: s.id, Score: s.score}
	}
	return hits, nil
}
