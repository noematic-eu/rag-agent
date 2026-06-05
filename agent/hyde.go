package main

import (
	"context"
	"log"
	"strings"

	"github.com/noematic-eu/ai-rag-agent/model"
)

const hydeConfidenceThreshold = 0.65

func parseHydeParam(raw string) (enabled bool, auto bool) {
	raw = strings.TrimSpace(strings.ToLower(raw))
	switch raw {
	case "1", "true", "yes", "on":
		return true, false
	case "auto":
		return false, true
	default:
		return false, false
	}
}

func hydePromptFR(generationQuery string) string {
	return "Rédige un court extrait fictif (3-4 phrases) tel qu'il pourrait apparaître\n" +
		"dans la Constitution française pour répondre à cette question.\n" +
		"Cite un numéro d'article plausible. Style juridique, pas de commentaire.\n\n" +
		"Question : " + generationQuery
}

func generateHydeText(generationQuery string) (string, error) {
	ctx := context.Background()
	return completeLLM(ctx, "", hydePromptFR(generationQuery))
}

func shouldApplyHyde(forced bool, auto bool, primaryTopScore float64) bool {
	if forced {
		return true
	}
	if auto && primaryTopScore > 0 && primaryTopScore < hydeConfidenceThreshold {
		return true
	}
	return false
}

// applyHydeBoost fuses vector hits from a HyDE embedding into an existing ranked outcome.
func applyHydeBoost(outcome rankOutcome, generationQuery string, p rankParams) rankOutcome {
	if !llmConfig.EmbeddingsEnabled || chunkStore == nil {
		return outcome
	}

	hydeText, err := generateHydeText(generationQuery)
	if err != nil {
		log.Printf("hyde: generation failed: %v", err)
		return outcome
	}
	hydeText = strings.TrimSpace(hydeText)
	if hydeText == "" {
		return outcome
	}

	embeddings, err := EmbedTextBatch([]string{hydeText})
	if err != nil || len(embeddings) == 0 {
		log.Printf("hyde: embedding failed: %v", err)
		return outcome
	}

	vectorHits, err := topKVectorHits(embeddings[0], p.topKVector, p.corpus)
	if err != nil {
		log.Printf("hyde: vector search failed: %v", err)
		return outcome
	}

	existingRanks := make(map[string]int)
	for i, hit := range outcome.hits {
		existingRanks[hit.ChunkID] = i + 1
	}
	hydeRanks := make(map[string]int)
	for i, hit := range vectorHits {
		hydeRanks[hit.ChunkID] = i + 1
		outcome.chunksByID[hit.ChunkID] = hit.Chunk
	}

	fused := fuseRRF(existingRanks, hydeRanks)
	if len(fused) > p.topKFinal {
		fused = fused[:p.topKFinal]
	}

	hits := make([]model.RetrieveHit, 0, len(fused))
	for _, hit := range fused {
		chunk, ok := outcome.chunksByID[hit.ID]
		if !ok {
			var loadErr error
			chunk, loadErr = loadChunkByID(hit.ID)
			if loadErr != nil {
				continue
			}
			outcome.chunksByID[hit.ID] = chunk
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
	if len(hits) == 0 {
		return outcome
	}
	outcome.hits = hits
	return outcome
}
