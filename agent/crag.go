package main

import (
	"context"
	"encoding/json"
	"log"
	"strconv"
	"strings"

	"github.com/noematic-eu/ai-rag-agent/model"
)

const maxCRAGFollowUpQueries = 2

type cragExcerptGrade struct {
	Index   int    `json:"n"`
	Verdict string `json:"verdict"`
}

type cragGradeResult struct {
	Sufficient      bool               `json:"sufficient"`
	FollowUpQueries []string           `json:"follow_up_queries"`
	Grades          []cragExcerptGrade `json:"grades"`
}

type cragTrace struct {
	Rounds          int      `json:"rounds"`
	FollowUpQueries []string `json:"follow_up_queries,omitempty"`
	Sufficient      bool     `json:"sufficient"`
}

func cragGradeSystemPrompt(lang string) string {
	if lang == "fr" {
		return "Tu évalues si des extraits documentaires suffisent pour répondre à une question. " +
			"Réponds uniquement en JSON valide, sans markdown ni commentaire."
	}
	return "You grade whether document excerpts are enough to answer a question. " +
		"Reply only with valid JSON, no markdown or commentary."
}

func cragGradeUserPrompt(lang, question string, docs []model.LegalDocument, topK int) string {
	topK = effectiveGenerationTopK(docs, topK)
	var b strings.Builder
	if lang == "fr" {
		b.WriteString("Question : ")
		b.WriteString(question)
		b.WriteString("\n\nExtraits :\n")
	} else {
		b.WriteString("Question: ")
		b.WriteString(question)
		b.WriteString("\n\nExcerpts:\n")
	}
	for i, doc := range docs {
		if i >= topK {
			break
		}
		body := excerptTextForChunk(doc.Content, question, doc.Article, maxSnippetChars)
		b.WriteString("[")
		b.WriteString(strconv.Itoa(i + 1))
		b.WriteString("] section=")
		b.WriteString(displaySectionPath(doc))
		b.WriteString("\nTexte:\n")
		b.WriteString(body)
		b.WriteString("\n\n")
	}
	if lang == "fr" {
		b.WriteString(`Analyse chaque extrait et produis ce JSON :
{"sufficient": true|false, "follow_up_queries": ["requête courte 1", "requête 2"], "grades": [{"n": 1, "verdict": "relevant|off_topic|ambiguous"}]}

Règles :
- sufficient=true seulement si tous les aspects de la question sont couverts par des extraits pertinents.
- follow_up_queries : 0 à 2 requêtes mots-clés pour combler les lacunes (vide si sufficient=true).
- verdict par extrait numéroté [n].
- ignore le copyright ou les mentions légales en en-tête ; juge la pertinence d'après le Texte:.
- verdict=ambiguous si l'en-tête est du bruit mais le Texte: est pertinent ; ne classe pas off_topic sur le seul copyright.`)
	} else {
		b.WriteString(`Grade each excerpt and output this JSON:
{"sufficient": true|false, "follow_up_queries": ["short query 1", "query 2"], "grades": [{"n": 1, "verdict": "relevant|off_topic|ambiguous"}]}

Rules:
- sufficient=true only when every aspect of the question is covered by relevant excerpts.
- follow_up_queries: 0-2 keyword queries to fill gaps (empty if sufficient=true).
- verdict per numbered excerpt [n].
- ignore copyright or legal notices in headers; grade relevance from Texte: body.
- verdict=ambiguous when the header is noise but Texte: is relevant; do not mark off_topic based on copyright alone.`)
	}
	return b.String()
}

func parseCragGradeResponse(raw string) cragGradeResult {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return cragGradeResult{}
	}
	start := strings.Index(raw, "{")
	end := strings.LastIndex(raw, "}")
	if start >= 0 && end > start {
		raw = raw[start : end+1]
	}
	var result cragGradeResult
	if err := json.Unmarshal([]byte(raw), &result); err != nil {
		log.Printf("crag: parse grade JSON failed: %v", err)
		return cragGradeResult{}
	}
	result.FollowUpQueries = dedupeQueries(result.FollowUpQueries)
	if len(result.FollowUpQueries) > maxCRAGFollowUpQueries {
		result.FollowUpQueries = result.FollowUpQueries[:maxCRAGFollowUpQueries]
	}
	return result
}

func gradeRetrievalContext(ctx context.Context, question, lang string, docs []model.LegalDocument, topK int) cragGradeResult {
	if len(docs) == 0 {
		return cragGradeResult{Sufficient: false}
	}
	raw, err := completeLLM(ctx, cragGradeSystemPrompt(lang), cragGradeUserPrompt(lang, question, docs, topK))
	if err != nil {
		log.Printf("crag: grade LLM failed: %v", err)
		return cragGradeResult{}
	}
	return parseCragGradeResponse(raw)
}

func mergeRetrievalOutcomes(primary, secondary rankOutcome, topK int) rankOutcome {
	if secondary.noResults || len(secondary.hits) == 0 {
		return primary
	}
	if primary.noResults || len(primary.hits) == 0 {
		return secondary
	}

	mergedChunks := make(map[string]model.Chunk, len(primary.chunksByID)+len(secondary.chunksByID))
	for id, chunk := range primary.chunksByID {
		mergedChunks[id] = chunk
	}
	for id, chunk := range secondary.chunksByID {
		mergedChunks[id] = chunk
	}

	rankLists := make([]map[string]int, 0, 2)
	for _, outcome := range []rankOutcome{primary, secondary} {
		ordered := make([]string, 0, len(outcome.hits))
		for _, hit := range outcome.hits {
			ordered = append(ordered, hit.ChunkID)
		}
		rankLists = append(rankLists, rankMap(nil, ordered))
	}

	fused := fuseMultiQueryRRF(rankLists)
	if topK > 0 && len(fused) > topK {
		fused = fused[:topK]
	}

	hits := make([]model.RetrieveHit, 0, len(fused))
	for _, hit := range fused {
		chunk, ok := mergedChunks[hit.ID]
		if !ok {
			var loadErr error
			chunk, loadErr = loadChunkByID(hit.ID)
			if loadErr != nil {
				continue
			}
			mergedChunks[hit.ID] = chunk
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

	return rankOutcome{hits: hits, chunksByID: mergedChunks}
}

func runCRAGFollowUp(pipeline retrievalPipelineInput, queries []string) (rankOutcome, error) {
	if len(queries) == 0 {
		return rankOutcome{noResults: true}, nil
	}
	p := pipeline.params
	return rankChunksMulti(queries, p)
}

func applyCRAGLoop(
	ctx context.Context,
	pipeline retrievalPipelineInput,
	initial rankOutcome,
	generationQuery, lang string,
	maxRounds int,
	emit func(round int, eventType string, payload map[string]interface{}),
) (rankOutcome, cragTrace, error) {
	trace := cragTrace{Rounds: 1, Sufficient: true}
	if maxRounds <= 0 || initial.noResults {
		return initial, trace, nil
	}

	outcome := initial
	topK := pipeline.params.topKFinal

	for round := 1; round <= maxRounds; round++ {
		docs := retrieveHitsToDocuments(outcome.hits, outcome.chunksByID)
		grade := gradeRetrievalContext(ctx, generationQuery, lang, docs, topK)
		trace.Sufficient = grade.Sufficient
		if emit != nil {
			emit(round, "grade", map[string]interface{}{
				"sufficient": grade.Sufficient,
				"grades":     grade.Grades,
			})
		}
		if grade.Sufficient || len(grade.FollowUpQueries) == 0 {
			break
		}
		if round >= maxRounds {
			break
		}

		trace.FollowUpQueries = append(trace.FollowUpQueries, grade.FollowUpQueries...)
		if emit != nil {
			emit(round+1, "retrieval_round", map[string]interface{}{
				"queries": grade.FollowUpQueries,
			})
		}

		followUp, err := runCRAGFollowUp(pipeline, grade.FollowUpQueries)
		if err != nil {
			return outcome, trace, err
		}
		outcome = mergeRetrievalOutcomes(outcome, followUp, topK)
		trace.Rounds++
	}

	return outcome, trace, nil
}
