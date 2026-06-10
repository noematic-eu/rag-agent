package main

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/noematic-eu/ai-rag-agent/model"
)

type agentAction struct {
	Name    string
	Query   string
	Corpus  string
	ChunkID string
	Reason  string
}

type agentToolContext struct {
	collectedDocs  []model.LegalDocument
	seenChunks     map[string]struct{}
	retrievalCount int
}

func newAgentToolContext(docs []model.LegalDocument) *agentToolContext {
	ctx := &agentToolContext{
		collectedDocs: make([]model.LegalDocument, 0, len(docs)),
		seenChunks:    make(map[string]struct{}),
	}
	ctx.mergeDocuments(docs)
	return ctx
}

func (ctx *agentToolContext) mergeDocuments(docs []model.LegalDocument) {
	for _, doc := range docs {
		chunkID := docChunkID(doc)
		if chunkID == "" {
			ctx.collectedDocs = append(ctx.collectedDocs, doc)
			continue
		}
		if _, ok := ctx.seenChunks[chunkID]; ok {
			continue
		}
		ctx.seenChunks[chunkID] = struct{}{}
		ctx.collectedDocs = append(ctx.collectedDocs, doc)
	}
}

func docChunkID(doc model.LegalDocument) string {
	parts := strings.SplitN(doc.ID, "::", 2)
	if len(parts) == 2 {
		return parts[1]
	}
	return ""
}

func agentSystemPrompt(lang string) string {
	if lang == "fr" {
		return "Tu es un agent de recherche documentaire. " +
			"Choisis UNE action par tour pour compléter le contexte avant de répondre. " +
			"Utilise search_kb pour chercher, get_chunk pour développer un extrait, finish quand le contexte suffit."
	}
	return "You are a document research agent. " +
		"Choose ONE action per turn to gather context before answering. " +
		"Use search_kb to search, get_chunk to expand an excerpt, finish when context is sufficient."
}

func agentTurnUserPrompt(lang, question string, docs []model.LegalDocument, topK int) string {
	topK = effectiveGenerationTopK(docs, topK)
	var b strings.Builder
	if lang == "fr" {
		b.WriteString("Question : ")
	} else {
		b.WriteString("Question: ")
	}
	b.WriteString(question)
	b.WriteString("\n\n")
	if len(docs) == 0 {
		if lang == "fr" {
			b.WriteString("Aucun extrait pour l'instant.\n\n")
		} else {
			b.WriteString("No excerpts yet.\n\n")
		}
	} else {
		if lang == "fr" {
			b.WriteString("Extraits actuels :\n")
		} else {
			b.WriteString("Current excerpts:\n")
		}
		for i, doc := range docs {
			if i >= topK {
				break
			}
			chunkID := docChunkID(doc)
			body := excerptTextForChunk(doc.Content, question, doc.Article, maxSnippetChars/2)
			fmt.Fprintf(&b, "[%d] chunk_id=%s section=%s\n%s\n\n", i+1, chunkID, strings.TrimSpace(doc.Title), body)
		}
	}
	if lang == "fr" {
		b.WriteString(`Réponds avec exactement un bloc :

ACTION: search_kb
QUERY: <mots-clés>
CORPUS: <optionnel>

ACTION: get_chunk
CHUNK_ID: <identifiant>

ACTION: finish
REASON: <pourquoi le contexte suffit>`)
	} else {
		b.WriteString(`Reply with exactly one block:

ACTION: search_kb
QUERY: <keywords>
CORPUS: <optional>

ACTION: get_chunk
CHUNK_ID: <id>

ACTION: finish
REASON: <why context is sufficient>`)
	}
	return b.String()
}

func parseAgentAction(raw string) agentAction {
	raw = strings.TrimSpace(raw)
	lines := strings.Split(raw, "\n")
	action := agentAction{}
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		upper := strings.ToUpper(line)
		switch {
		case strings.HasPrefix(upper, "ACTION:"):
			action.Name = strings.ToLower(strings.TrimSpace(line[len("ACTION:"):]))
		case strings.HasPrefix(upper, "QUERY:"):
			action.Query = strings.TrimSpace(line[len("QUERY:"):])
		case strings.HasPrefix(upper, "CORPUS:"):
			action.Corpus = strings.TrimSpace(line[len("CORPUS:"):])
		case strings.HasPrefix(upper, "CHUNK_ID:"):
			action.ChunkID = strings.TrimSpace(line[len("CHUNK_ID:"):])
		case strings.HasPrefix(upper, "REASON:"):
			action.Reason = strings.TrimSpace(line[len("REASON:"):])
		}
	}
	return action
}

func executeSearchKB(ctx *agentToolContext, pipeline retrievalPipelineInput, query, corpus string) (string, error) {
	if ctx.retrievalCount >= maxAgentRetrievals {
		return "retrieval budget exhausted", nil
	}
	query = strings.TrimSpace(query)
	if query == "" {
		return "empty query", nil
	}
	ctx.retrievalCount++

	p := pipeline
	p.explicitRetrieval = query
	if corpus != "" {
		p.params.corpus = corpus
	}
	outcome, _, err := runRetrievalPipeline(p)
	if err != nil {
		return "", err
	}
	if outcome.noResults {
		return "no results", nil
	}
	docs := retrieveHitsToDocuments(outcome.hits, outcome.chunksByID)
	ctx.mergeDocuments(docs)

	summary := make([]map[string]string, 0, len(outcome.hits))
	for i, hit := range outcome.hits {
		if i >= 5 {
			break
		}
		summary = append(summary, map[string]string{
			"chunk_id": hit.ChunkID,
			"section":  hit.Section,
			"score":    fmt.Sprintf("%.3f", hit.Score),
		})
	}
	data, _ := json.Marshal(summary)
	return string(data), nil
}

func executeGetChunk(ctx *agentToolContext, chunkID string) (string, error) {
	chunkID = strings.TrimSpace(chunkID)
	if chunkID == "" {
		return "empty chunk_id", nil
	}
	chunk, err := loadChunkByID(chunkID)
	if err != nil {
		return "chunk not found", nil
	}
	section := chunk.Metadata.Title
	if section == "" {
		section = chunk.Metadata.SectionPath
	}
	doc := model.LegalDocument{
		ID:        chunk.Metadata.DocID + "::" + chunk.Metadata.ChunkID,
		Title:     section,
		BookTitle: chunk.Metadata.DocTitle,
		Content:   chunk.Text,
		Corpus:    chunk.Metadata.Corpus,
		Article:   chunk.Metadata.Article,
	}
	ctx.mergeDocuments([]model.LegalDocument{doc})
	body := excerptTextForChunk(chunk.Text, "", chunk.Metadata.Article, maxSnippetChars)
	return body, nil
}
