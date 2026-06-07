package main

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/noematic-eu/ai-rag-agent/model"
)

// retrieveDocuments handles GET /retrieve (ranked chunks only, no LLM).
func retrieveDocuments(c *gin.Context) {
	retrievalText, generationText, err := retrieveQueriesFromContext(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	opts := rankParamsToOptions(rankParamsFromContext(c, retrievalText))
	opts.GenerationText = generationText
	resp, err := ragAgent.Retrieve(opts)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Erreur de recherche : " + err.Error()})
		return
	}
	if includeTextParam(c) {
		excerptQuery := effectiveRetrievalQueryForExcerpts(c, retrievalText, generationText)
		enrichRetrieveHitsWithExcerpts(&resp, excerptQuery)
	}
	c.JSON(http.StatusOK, resp)
}

func includeTextParam(c *gin.Context) bool {
	raw := strings.TrimSpace(c.Query("include_text"))
	if raw == "" {
		raw = strings.TrimSpace(c.Query("expand"))
	}
	if raw == "" {
		return false
	}
	v, err := strconv.ParseBool(raw)
	if err != nil {
		return raw == "1" || strings.EqualFold(raw, "text")
	}
	return v
}

func effectiveRetrievalQueryForExcerpts(c *gin.Context, retrievalText, generationText string) string {
	if strings.TrimSpace(retrievalText) != "" {
		return retrievalText
	}
	if strings.TrimSpace(generationText) == "" {
		return ""
	}
	p := rankParamsFromContext(c, generationText)
	rewriteEnabled := parseRewriteParam(c.Query("rewrite"), p.corpus)
	queries := buildRetrievalQueries(generationText, "", p.corpus, rewriteEnabled)
	return strings.Join(queries, " ")
}

func enrichRetrieveHitsWithExcerpts(resp *model.RetrieveResponse, retrievalQuery string) {
	for i := range resp.Hits {
		chunk, err := loadChunkByID(resp.Hits[i].ChunkID)
		if err != nil {
			continue
		}
		article := resp.Hits[i].Article
		if article == "" {
			article = chunk.Metadata.Article
		}
		resp.Hits[i].Excerpt = excerptTextForChunk(chunk.Text, retrievalQuery, article, maxSnippetChars)
	}
}

func retrieveQueriesFromContext(c *gin.Context) (retrievalText, generationText string, err error) {
	rq := strings.TrimSpace(c.Query("retrieval_q"))
	if rq == "" {
		rq = strings.TrimSpace(c.Query("rq"))
	}
	if rq != "" {
		return rq, "", nil
	}

	q := strings.TrimSpace(c.Query("q"))
	if q == "" {
		return "", "", errRetrievalQueryRequired
	}
	var explicitRQ bool
	retrievalText, generationText, explicitRQ = parseSearchQueries(c, q)
	if explicitRQ {
		return retrievalText, "", nil
	}
	return "", generationText, nil
}

var errRetrievalQueryRequired = &retrievalQueryError{}

type retrievalQueryError struct{}

func (e *retrievalQueryError) Error() string {
	return "Le paramètre 'q' ou 'rq' est requis"
}
