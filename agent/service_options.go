package main

import (
	"strconv"
	"strings"
)

// RankOptions holds hybrid retrieval parameters shared by search and retrieve.
type RankOptions struct {
	RetrievalText  string
	GenerationText string
	Corpus         string
	DocID          string
	TopKBM25       int
	TopKVector     int
	TopKFinal      int
	MinScore       float64
	FusionMode     string
	FusionWeight   float64
	MaxPerDoc      int
	Article        string
	LegalRerank    *bool
	Lang           string
}

func defaultRankOptions(retrievalText string) RankOptions {
	return RankOptions{
		RetrievalText: retrievalText,
		TopKBM25:      20,
		TopKVector:    20,
		TopKFinal:     8,
		MinScore:      defaultMinScore,
		FusionMode:    "weighted",
		FusionWeight:  0.6,
		MaxPerDoc:     1,
	}
}

func rankParamsToOptions(p rankParams) RankOptions {
	return RankOptions{
		RetrievalText: p.retrievalText,
		Corpus:        p.corpus,
		DocID:         p.docID,
		TopKBM25:      p.topKBM25,
		TopKVector:    p.topKVector,
		TopKFinal:     p.topKFinal,
		MinScore:      p.minScore,
		FusionMode:    p.fusionMode,
		FusionWeight:  p.fusionWeight,
		MaxPerDoc:     p.maxPerDoc,
		Article:       p.article,
		LegalRerank:   p.legalRerank,
	}
}

func rankParamsFromOptions(opts RankOptions) rankParams {
	maxPerDoc := opts.MaxPerDoc
	if maxPerDoc == 0 {
		maxPerDoc = defaultMaxPerDoc(opts.Corpus, opts.DocID, opts.TopKFinal)
	}
	fusionMode := opts.FusionMode
	fusionWeight := opts.FusionWeight
	if fusionMode == "" {
		fusionMode = "weighted"
	}
	if fusionWeight == 0 && fusionMode == "weighted" {
		fusionWeight = 0.6
	}
	topKBM25 := opts.TopKBM25
	if topKBM25 <= 0 {
		topKBM25 = 20
	}
	topKVector := opts.TopKVector
	if topKVector <= 0 {
		topKVector = 20
	}
	topKFinal := opts.TopKFinal
	if topKFinal <= 0 {
		topKFinal = 8
	}
	minScore := opts.MinScore
	if minScore == 0 {
		minScore = defaultMinScore
	}
	return rankParams{
		retrievalText: opts.RetrievalText,
		topKBM25:      topKBM25,
		topKVector:    topKVector,
		topKFinal:     topKFinal,
		minScore:      minScore,
		corpus:        strings.TrimSpace(opts.Corpus),
		docID:         strings.TrimSpace(opts.DocID),
		fusionMode:    fusionMode,
		fusionWeight:  fusionWeight,
		maxPerDoc:     maxPerDoc,
		article:       strings.TrimSpace(opts.Article),
		legalRerank:   opts.LegalRerank,
	}
}

func parseRankOptionsFromParams(retrievalQuery, generationQuery string, params map[string]string) RankOptions {
	baseQuery := retrievalQuery
	if baseQuery == "" {
		baseQuery = generationQuery
	}
	opts := defaultRankOptions(baseQuery)
	if generationQuery != "" {
		opts.GenerationText = generationQuery
	}
	if retrievalQuery != "" {
		opts.RetrievalText = retrievalQuery
	}
	if v := strings.TrimSpace(params["rq"]); v != "" {
		opts.RetrievalText = v
	}
	if v := strings.TrimSpace(params["retrieval_q"]); v != "" {
		opts.RetrievalText = v
	}
	if v := strings.TrimSpace(params["corpus"]); v != "" {
		opts.Corpus = v
	}
	if v := strings.TrimSpace(params["doc_id"]); v != "" {
		opts.DocID = v
	}
	if v := strings.TrimSpace(params["lang"]); v != "" {
		opts.Lang = v
	}
	if v := strings.TrimSpace(params["fusion"]); v != "" {
		if strings.EqualFold(v, "rrf") {
			opts.FusionMode = "rrf"
			opts.FusionWeight = 0.7
		} else if f, err := strconv.ParseFloat(v, 64); err == nil {
			opts.FusionMode = "weighted"
			if f >= 0 && f <= 1 {
				opts.FusionWeight = f
			}
		}
	}
	if v := strings.TrimSpace(params["top_k"]); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			opts.TopKFinal = n
		}
	}
	if v := strings.TrimSpace(params["bm25_k"]); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			opts.TopKBM25 = n
		}
	}
	if v := strings.TrimSpace(params["vector_k"]); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			opts.TopKVector = n
		}
	}
	if v := strings.TrimSpace(params["min_score"]); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			opts.MinScore = f
		}
	}
	if v := strings.TrimSpace(params["max_per_doc"]); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			opts.MaxPerDoc = n
		}
	}
	if v := strings.TrimSpace(params["article"]); v != "" {
		opts.Article = v
	}
	if v := strings.TrimSpace(params["legal_rerank"]); v != "" {
		opts.LegalRerank = parseLegalRerankParam(v)
	}
	return opts
}
