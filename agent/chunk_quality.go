package main

import (
	"strings"

	"github.com/noematic-eu/ai-rag-agent/model"
)

const minChunkChars = 80

// isIndexableChunk returns false for chunks that are too short or title-only.
func isIndexableChunk(chunk model.Chunk) bool {
	text := strings.TrimSpace(chunk.Text)
	if len(text) < minChunkChars {
		return false
	}

	title := strings.TrimSpace(chunk.Metadata.Title)
	if title == "" {
		return true
	}

	body := strings.TrimSpace(strings.TrimPrefix(text, title))
	body = strings.TrimLeft(body, ":-— \n")
	if body == "" || body == title {
		return false
	}
	if strings.EqualFold(strings.TrimSpace(text), title) {
		return false
	}

	return true
}

func mergeSmallChunks(chunks []model.Chunk) []model.Chunk {
	if len(chunks) == 0 {
		return chunks
	}

	merged := make([]model.Chunk, 0, len(chunks))
	current := chunks[0]

	for i := 1; i < len(chunks); i++ {
		if len(strings.TrimSpace(current.Text)) < minChunkChars {
			current.Text = strings.TrimSpace(current.Text) + "\n\n" + strings.TrimSpace(chunks[i].Text)
			if chunks[i].Metadata.Title != "" && current.Metadata.Title == "" {
				current.Metadata.Title = chunks[i].Metadata.Title
			}
			continue
		}
		merged = append(merged, current)
		current = chunks[i]
	}
	merged = append(merged, current)
	return merged
}

func filterIndexableChunks(chunks []model.Chunk) []model.Chunk {
	chunks = mergeSmallChunks(chunks)
	filtered := make([]model.Chunk, 0, len(chunks))
	for _, chunk := range chunks {
		if isIndexableChunk(chunk) {
			filtered = append(filtered, chunk)
		}
	}
	return filtered
}
