package main

import (
	"container/heap"
	"encoding/json"

	"github.com/noematic-eu/ai-rag-agent/model"
)

type vectorHit struct {
	ChunkID string
	Score   float64
	Chunk   model.Chunk
}

type vectorHitHeap []vectorHit

func (h vectorHitHeap) Len() int           { return len(h) }
func (h vectorHitHeap) Less(i, j int) bool { return h[i].Score < h[j].Score }
func (h vectorHitHeap) Swap(i, j int)      { h[i], h[j] = h[j], h[i] }

func (h *vectorHitHeap) Push(x interface{}) {
	*h = append(*h, x.(vectorHit))
}

func (h *vectorHitHeap) Pop() interface{} {
	old := *h
	n := len(old)
	item := old[n-1]
	*h = old[:n-1]
	return item
}

func topKVectorHits(queryEmbedding []float64, k int, corpus string) ([]vectorHit, error) {
	if k <= 0 || len(queryEmbedding) == 0 {
		return nil, nil
	}

	h := &vectorHitHeap{}
	heap.Init(h)

	pairs, err := chunkStore.ScanPrefix("chunk:")
	if err != nil {
		return nil, err
	}

	for _, pair := range pairs {
		var chunk model.Chunk
		if err := json.Unmarshal(pair.Value, &chunk); err != nil {
			continue
		}
		if len(chunk.Embedding) == 0 {
			continue
		}
		if corpus != "" && chunk.Metadata.Corpus != corpus {
			continue
		}

		score := embeddingCosineSimilarity(queryEmbedding, chunk.Embedding)
		hit := vectorHit{
			ChunkID: chunk.Metadata.ChunkID,
			Score:   score,
			Chunk:   chunk,
		}

		if h.Len() < k {
			heap.Push(h, hit)
			continue
		}
		if score > (*h)[0].Score {
			(*h)[0] = hit
			heap.Fix(h, 0)
		}
	}

	results := make([]vectorHit, h.Len())
	for i := len(results) - 1; i >= 0; i-- {
		results[i] = heap.Pop(h).(vectorHit)
	}
	return results, nil
}
