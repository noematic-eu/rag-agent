package main

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/noematic-eu/ai-rag-agent/internal/f4kvs"
	"github.com/noematic-eu/ai-rag-agent/model"
)

const defaultMinScore = 0.2

// searchDocuments handles GET /search (retrieve + LLM answer).
func searchDocuments(c *gin.Context) {
	queryText := strings.TrimSpace(c.Query("q"))
	if queryText == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Le paramètre 'q' est requis"})
		return
	}

	retrievalText, generationText, explicitRQ := parseSearchQueries(c, queryText)
	explicitRetrieval := ""
	if explicitRQ {
		explicitRetrieval = retrievalText
	}
	pipeline := retrievalPipelineFromContext(c, generationText, explicitRetrieval)
	p := pipeline.params
	lang := strings.TrimSpace(c.Query("lang"))
	mode := parseSearchMode(c)

	if mode.level == searchLevelRetrieve {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "level=0 (retrieve-only) is not supported on /search; use GET /retrieve instead",
		})
		return
	}

	var streamWriter StreamWriter
	if mode.autoEnabled || mode.cragEnabled || mode.agentEnabled {
		streamWriter = newGinStreamWriter(c)
	}

	outcome, docs, rewriteQueries, extraMeta, err := executeSearch(
		context.Background(), pipeline, generationText, lang, mode, streamWriter,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Erreur de recherche : " + err.Error()})
		return
	}

	if outcome.noResults || len(docs) == 0 {
		c.JSON(http.StatusOK, gin.H{"status": "no_results", "message": "Aucun résultat pertinent"})
		return
	}

	retrievalForPrompt := retrievalText
	if retrievalForPrompt == "" && len(rewriteQueries) > 0 {
		retrievalForPrompt = strings.Join(rewriteQueries, " ")
	}
	if streamWriter != nil {
		if err := generateResponseWithStream(docs, generationText, retrievalForPrompt, lang, p.topKFinal, rewriteQueries, extraMeta, streamWriter); err != nil {
			if c.Writer.Written() {
				writeSSEError(c, err)
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Erreur lors de la génération de la réponse : " + err.Error()})
		}
		return
	}
	if err := generateResponseWithLLM(docs, generationText, retrievalForPrompt, lang, p.topKFinal, rewriteQueries, extraMeta, c); err != nil {
		if c.Writer.Written() {
			writeSSEError(c, err)
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Erreur lors de la génération de la réponse : " + err.Error()})
		return
	}
}

func retrieveHitsToDocuments(hits []model.RetrieveHit, chunksByID map[string]model.Chunk) []model.LegalDocument {
	docs := make([]model.LegalDocument, 0, len(hits))
	for _, hit := range hits {
		chunk, ok := chunksByID[hit.ChunkID]
		if !ok {
			var loadErr error
			chunk, loadErr = loadChunkByID(hit.ChunkID)
			if loadErr != nil {
				continue
			}
		}

		sectionTitle := hit.Section
		if sectionTitle == "" {
			sectionTitle = chunk.Metadata.SectionPath
		}
		bookTitle := hit.DocTitle
		if bookTitle == "" {
			bookTitle = sectionTitle
		}
		article := hit.Article
		if article == "" {
			article = chunk.Metadata.Article
		}
		docs = append(docs, model.LegalDocument{
			ID:        chunk.Metadata.DocID + "::" + chunk.Metadata.ChunkID,
			Title:     sectionTitle,
			BookTitle: bookTitle,
			Content:   chunk.Text,
			Corpus:    chunk.Metadata.Corpus,
			Article:   article,
		})
	}
	return docs
}

func hydrateChunksByID(chunksByID map[string]model.Chunk, idLists ...[]string) {
	for _, ids := range idLists {
		for _, id := range ids {
			if _, ok := chunksByID[id]; ok {
				continue
			}
			chunk, err := loadChunkByID(id)
			if err != nil {
				continue
			}
			chunksByID[id] = chunk
		}
	}
}

func parseFusionParam(c *gin.Context) (mode string, weight float64) {
	raw := strings.TrimSpace(c.Query("fusion"))
	if strings.EqualFold(raw, "rrf") {
		return "rrf", 0.7
	}
	weight = floatQueryParam(c, "fusion", 0.6)
	if weight < 0 || weight > 1 {
		weight = 0.6
	}
	return "weighted", weight
}

func loadChunkByID(chunkID string) (model.Chunk, error) {
	var chunk model.Chunk
	data, err := chunkStore.Get("chunk:" + chunkID)
	if err != nil {
		if errors.Is(err, f4kvs.ErrNotFound) {
			return chunk, err
		}
		return chunk, err
	}
	if err := json.Unmarshal(data, &chunk); err != nil {
		return chunk, err
	}
	return chunk, nil
}

func embeddingCosineSimilarity(a, b []float64) float64 {
	if len(a) == 0 || len(b) == 0 {
		return 0
	}

	n := len(a)
	if len(b) < n {
		n = len(b)
	}

	var dot float64
	var normA float64
	var normB float64
	for i := 0; i < n; i++ {
		dot += a[i] * b[i]
		normA += a[i] * a[i]
		normB += b[i] * b[i]
	}
	if normA == 0 || normB == 0 {
		return 0
	}
	return dot / (math.Sqrt(normA) * math.Sqrt(normB))
}

func intQueryParam(c *gin.Context, key string, defaultValue int) int {
	raw := c.Query(key)
	if raw == "" {
		return defaultValue
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value <= 0 {
		return defaultValue
	}
	return value
}

func floatQueryParam(c *gin.Context, key string, defaultValue float64) float64 {
	raw := c.Query(key)
	if raw == "" {
		return defaultValue
	}
	value, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return defaultValue
	}
	return value
}
