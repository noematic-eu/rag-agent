package main

import (
	"log"
	"sort"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/noematic-eu/ai-rag-agent/model"
)

type rankParams struct {
	retrievalText string
	topKBM25      int
	topKVector    int
	topKFinal     int
	minScore      float64
	corpus        string
	docID         string
	article       string
	legalRerank   *bool
	fusionMode    string
	fusionWeight  float64
	maxPerDoc     int
}

type rankOutcome struct {
	hits       []model.RetrieveHit
	chunksByID map[string]model.Chunk
	noResults  bool
}

func rankParamsFromContext(c *gin.Context, retrievalText string) rankParams {
	corpus := strings.TrimSpace(c.Query("corpus"))
	docID := strings.TrimSpace(c.Query("doc_id"))
	maxPerDoc := defaultMaxPerDoc(corpus, docID, intQueryParam(c, "top_k", 8))
	if raw := strings.TrimSpace(c.Query("max_per_doc")); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 {
			maxPerDoc = n
		}
	}
	fusionMode, fusionWeight := parseFusionParam(c)
	return rankParams{
		retrievalText: retrievalText,
		topKBM25:      intQueryParam(c, "bm25_k", 20),
		topKVector:    intQueryParam(c, "vector_k", 20),
		topKFinal:     intQueryParam(c, "top_k", 8),
		minScore:      floatQueryParam(c, "min_score", defaultMinScore),
		corpus:        corpus,
		docID:         docID,
		article:       strings.TrimSpace(c.Query("article")),
		legalRerank:   parseLegalRerankParam(c.Query("legal_rerank")),
		fusionMode:    fusionMode,
		fusionWeight:  fusionWeight,
		maxPerDoc:     maxPerDoc,
	}
}

func defaultMaxPerDoc(corpus, docID string, topK int) int {
	if docID != "" {
		return 8
	}
	if corpus != "" {
		if topK < 6 {
			return topK
		}
		return 6
	}
	return 1
}

func rankChunks(p rankParams) (rankOutcome, error) {
	if p.corpus != "" && p.maxPerDoc < p.topKFinal {
		log.Printf("rank: max_per_doc=%d < top_k=%d for corpus=%q; some chunks from the same document may be dropped", p.maxPerDoc, p.topKFinal, p.corpus)
	}
	lexHits, err := lexicalBackend.Search(p.retrievalText, p.corpus, p.topKBM25)
	if err != nil {
		return rankOutcome{}, err
	}

	bm25Scores := make(map[string]float64)
	bm25Order := make([]string, 0, len(lexHits))
	maxBM25 := 0.0
	for _, hit := range lexHits {
		bm25Scores[hit.ChunkID] = hit.Score
		bm25Order = append(bm25Order, hit.ChunkID)
		if hit.Score > maxBM25 {
			maxBM25 = hit.Score
		}
	}

	vectorScores := make(map[string]float64)
	chunksByID := make(map[string]model.Chunk)
	vectorOrder := make([]string, 0)
	maxVector := 0.0

	if llmConfig.EmbeddingsEnabled {
		queryEmbedding := []float64{}
		queryEmbeddings, embErr := EmbedTextBatch([]string{p.retrievalText})
		if embErr == nil && len(queryEmbeddings) > 0 {
			queryEmbedding = queryEmbeddings[0]
		}

		vectorHits, vecErr := topKVectorHits(queryEmbedding, p.topKVector, p.corpus)
		if vecErr != nil {
			return rankOutcome{}, vecErr
		}
		for _, hit := range vectorHits {
			vectorScores[hit.ChunkID] = hit.Score
			chunksByID[hit.ChunkID] = hit.Chunk
			vectorOrder = append(vectorOrder, hit.ChunkID)
			if hit.Score > maxVector {
				maxVector = hit.Score
			}
		}
	}

	hydrateChunksByID(chunksByID, bm25Order, vectorOrder)

	var sortedChunks []chunkScore
	switch p.fusionMode {
	case "rrf":
		sortedChunks = fuseRRF(rankMap(bm25Scores, bm25Order), rankMap(vectorScores, vectorOrder))
	default:
		sortedChunks = fuseWeighted(bm25Scores, vectorScores, maxBM25, maxVector, p.fusionWeight)
		for i := range sortedChunks {
			sortedChunks[i].Score *= lengthScorePenalty(sortedChunks[i].ID, chunksByID)
		}
		sort.Slice(sortedChunks, func(i, j int) bool {
			return sortedChunks[i].Score > sortedChunks[j].Score
		})
	}

	if refs := articleRefsFromQuery(p.retrievalText); len(refs) > 1 {
		sortedChunks = boostArticleChunks(sortedChunks, chunksByID, refs, p.corpus)
		sort.Slice(sortedChunks, func(i, j int) bool {
			return sortedChunks[i].Score > sortedChunks[j].Score
		})
	}

	sortedChunks = rerankLegalArticles(sortedChunks, chunksByID, p.retrievalText, p.corpus, p.legalRerank)

	sortedChunks = dedupeByDocID(sortedChunks, chunksByID, p.maxPerDoc)
	if p.docID != "" {
		sortedChunks = filterChunksByDocID(sortedChunks, chunksByID, p.docID)
	}
	if p.article != "" {
		sortedChunks = filterChunksByArticle(sortedChunks, chunksByID, p.article)
	}
	if len(sortedChunks) > p.topKFinal {
		sortedChunks = sortedChunks[:p.topKFinal]
	}

	if len(sortedChunks) == 0 || sortedChunks[0].Score < p.minScore {
		if p.docID != "" {
			if fallback, ok := fallbackDocChunks(p.docID, p.corpus, p.topKFinal); ok {
				return fallback, nil
			}
		}
		return rankOutcome{noResults: true, chunksByID: chunksByID}, nil
	}

	hits := make([]model.RetrieveHit, 0, len(sortedChunks))
	for _, hit := range sortedChunks {
		chunk, ok := chunksByID[hit.ID]
		if !ok {
			var loadErr error
			chunk, loadErr = loadChunkByID(hit.ID)
			if loadErr != nil {
				continue
			}
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
		return rankOutcome{noResults: true, chunksByID: chunksByID}, nil
	}
	return rankOutcome{hits: hits, chunksByID: chunksByID}, nil
}
