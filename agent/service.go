package main

import (
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/noematic-eu/ai-rag-agent/model"
)

// Agent exposes the RAG pipeline without HTTP or 9P transport details.
type Agent struct {
	mu sync.RWMutex
}

var ragAgent = &Agent{}

type IngestResult struct {
	Status string `json:"status"`
	Chunks int    `json:"chunks"`
}

type DeleteResult struct {
	Status        string `json:"status"`
	DocID         string `json:"doc_id"`
	ChunksDeleted int    `json:"chunks_deleted"`
	Corpus        string `json:"corpus,omitempty"`
}

type SearchResult struct {
	Status   string            `json:"status,omitempty"`
	Message  string            `json:"message,omitempty"`
	Answer   string            `json:"response,omitempty"`
	Metadata map[string]string `json:"metadata,omitempty"`
}

func (a *Agent) Ingest(doc model.LegalDocument) (IngestResult, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	normalizedDoc, err := normalizeDocumentContent(doc)
	if err != nil {
		return IngestResult{}, fmt.Errorf("normalize content: %w", err)
	}
	chunks, err := indexDocument(normalizedDoc)
	if err != nil {
		return IngestResult{}, fmt.Errorf("index document: %w", err)
	}
	return IngestResult{
		Status: "Document ingéré avec succès",
		Chunks: chunks,
	}, nil
}

func (a *Agent) Retrieve(opts RankOptions) (model.RetrieveResponse, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()

	retrievalText := strings.TrimSpace(opts.RetrievalText)
	generationText := strings.TrimSpace(opts.GenerationText)
	if retrievalText == "" && generationText == "" {
		return model.RetrieveResponse{}, errors.New("retrieval query is required")
	}

	var outcome rankOutcome
	var err error
	explicitRetrieval := retrievalText
	usePipeline := generationText != "" && (explicitRetrieval == "" || explicitRetrieval == generationText)
	if !usePipeline && explicitRetrieval != "" {
		outcome, err = rankChunks(rankParamsFromOptions(opts))
	} else if generationText != "" {
		pipeline := retrievalPipelineFromOptions(opts, generationText, "")
		outcome, _, err = runRetrievalPipeline(pipeline)
		if retrievalText == "" {
			retrievalText = generationText
		}
	} else {
		return model.RetrieveResponse{}, errors.New("retrieval query is required")
	}
	if err != nil {
		return model.RetrieveResponse{}, err
	}
	if outcome.noResults {
		return model.RetrieveResponse{
			Status: "no_results",
			Query:  retrievalText,
			Hits:   []model.RetrieveHit{},
		}, nil
	}
	return model.RetrieveResponse{
		Status: "ok",
		Query:  retrievalText,
		Hits:   outcome.hits,
	}, nil
}

func (a *Agent) Search(opts RankOptions, generationQuery string) (SearchResult, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()

	generationQuery = strings.TrimSpace(generationQuery)
	if generationQuery == "" {
		return SearchResult{}, errors.New("search query is required")
	}

	generationText := generationQuery
	explicitRetrieval := ""
	if strings.TrimSpace(opts.RetrievalText) != "" && opts.RetrievalText != generationQuery {
		explicitRetrieval = strings.TrimSpace(opts.RetrievalText)
	}
	pipeline := retrievalPipelineFromOptions(opts, generationText, explicitRetrieval)
	outcome, rewriteQueries, err := runRetrievalPipeline(pipeline)
	if err != nil {
		return SearchResult{}, err
	}
	retrievalText := explicitRetrieval
	if retrievalText == "" && len(rewriteQueries) > 0 {
		retrievalText = rewriteQueries[0]
	}
	if outcome.noResults {
		return SearchResult{
			Status:  "no_results",
			Message: "Aucun résultat pertinent",
		}, nil
	}

	docs := retrieveHitsToDocuments(outcome.hits, outcome.chunksByID)
	if len(docs) == 0 {
		return SearchResult{
			Status:  "no_results",
			Message: "Aucun résultat pertinent",
		}, nil
	}

	buf := &bufferStreamWriter{}
	if err := generateResponseWithStream(docs, generationText, retrievalText, opts.Lang, opts.TopKFinal, rewriteQueries, buf); err != nil {
		return SearchResult{}, err
	}
	if buf.metadata == nil {
		buf.metadata = map[string]string{}
	}
	if len(rewriteQueries) > 0 {
		buf.metadata["rewrite_queries"] = formatRetrievalQueriesDebug(rewriteQueries)
	}
	return SearchResult{
		Status:   "ok",
		Answer:   buf.answer,
		Metadata: buf.metadata,
	}, nil
}

func (a *Agent) Stats() statsSnapshot {
	return currentStatsSnapshot()
}

func (a *Agent) Reset() error {
	a.mu.Lock()
	defer a.mu.Unlock()
	return resetStores()
}

func (a *Agent) Finalize() error {
	a.mu.Lock()
	defer a.mu.Unlock()
	finalizeIDF()
	return nil
}

func (a *Agent) DeleteDocument(docID string) (DeleteResult, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	docID = strings.TrimSpace(docID)
	if docID == "" {
		return DeleteResult{}, errors.New("doc_id is required")
	}
	deleted, corpus, err := deleteDocumentByID(docID)
	if err != nil {
		return DeleteResult{}, err
	}
	if deleted == 0 {
		return DeleteResult{}, fmt.Errorf("document not found: %s", docID)
	}
	return DeleteResult{
		Status:        "deleted",
		DocID:         docID,
		ChunksDeleted: deleted,
		Corpus:        corpus,
	}, nil
}

func (a *Agent) RunCtl(command string) (string, error) {
	command = strings.TrimSpace(strings.ToLower(command))
	switch command {
	case "reset":
		if err := a.Reset(); err != nil {
			return "", err
		}
		return "index reset", nil
	case "finalize":
		if err := a.Finalize(); err != nil {
			return "", err
		}
		return "finalize ok", nil
	case "", "status":
		return "rag agent ready", nil
	default:
		return "", fmt.Errorf("unknown ctl command: %q (use reset, finalize, or status)", command)
	}
}
