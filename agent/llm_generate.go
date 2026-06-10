package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/noematic-eu/ai-rag-agent/model"
)

type ollamaChatRequest struct {
	Model    string              `json:"model"`
	Messages []openAIChatMessage `json:"messages"`
	Stream   bool                `json:"stream"`
}

type ollamaChatStreamChunk struct {
	Message struct {
		Content string `json:"content"`
	} `json:"message"`
	Done bool `json:"done"`
}

type openAIChatRequest struct {
	Model    string              `json:"model"`
	Messages []openAIChatMessage `json:"messages"`
	Stream   bool                `json:"stream"`
}

type openAIChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type openAIChatStreamChunk struct {
	Choices []struct {
		Delta struct {
			Content string `json:"content"`
		} `json:"delta"`
	} `json:"choices"`
}

const maxPromptContextChars = 6000
const maxSnippetChars = 900
const defaultGenerationTopK = 8
const legalGenerationTopK = 4

func ragBasePrompt(queryLang string) string {
	if queryLang == "fr" {
		return "Tu es un assistant qui répond uniquement à partir des extraits fournis. " +
			"Ne montre jamais ton raisonnement interne, tes hésitations ni une analyse étape par étape avant de répondre ; produis directement la réponse structurée demandée. " +
			"Structure ta réponse en trois parties : (1) analyse brève des extraits pertinents vs hors sujet, (2) réponse structurée avec citations [n], (3) limites de ce que les extraits ne couvrent pas. " +
			"Chaque affirmation doit citer la source avec [n] (numéro d'extrait). Le numéro [n] doit correspondre exactement à la liste d'extraits. " +
			"Ne fusionne pas des idées de plusieurs extraits en une seule attribution; une citation [n] = un extrait. " +
			"Ne nomme pas d'auteur ou de livre sauf s'ils apparaissent dans le texte de cet extrait. " +
			"Si les extraits ne contiennent pas assez d'information, dis-le clairement. " +
			"N'utilise aucune connaissance externe. Réponds dans la même langue que la question."
	}
	return "You are an assistant that answers only from the provided excerpts. " +
		"Never show internal reasoning, deliberation, or step-by-step analysis before answering; output the structured response directly. " +
		"Structure your answer in three parts: (1) brief analysis of relevant vs off-topic excerpts, (2) structured answer with [n] citations, (3) limits of what the excerpts do not cover. " +
		"Every claim must cite its source with [n] (excerpt number). The [n] index must match the excerpt list exactly. " +
		"Do not merge ideas from different excerpts into one attribution; one [n] = one excerpt. " +
		"Do not name authors or books unless they appear in that excerpt's text. " +
		"If the excerpts do not contain enough information, say so clearly. " +
		"Do not use outside knowledge. Answer in the same language as the question."
}

func ragLegalOverlay(queryLang string) string {
	if queryLang == "fr" {
		return " Avant de répondre, analyse chaque extrait et classe-le mentalement : directement pertinent, contexte général, ou hors sujet. " +
			"Ne traite pas un extrait de contexte général (ex. principes républicains, souveraineté) comme réponse directe à une question sur une institution précise (ex. Président, Parlement). " +
			"Si la question porte sur le Président mais que seuls les articles 1-3 sur la souveraineté sont fournis, ne présente pas « gouvernement du peuple » comme obligation présidentielle ; dis-le clairement. " +
			"Quand les articles 16 et 7 (ou d'autres articles distincts) sont pertinents, réponds en sous-sections séparées intitulées « Article 16 — pouvoirs exceptionnels » et « Article 7 — élection présidentielle » sans fusionner leurs régimes. " +
			"Dans la sous-section article 16, si un extrait contient « dissoute » ou l'interdiction de dissolution de l'Assemblée nationale, tu dois inclure une phrase explicite reprenant cette interdiction avec citation [n] avant de passer à l'article 7. " +
			"Ne confonds pas les pouvoirs exceptionnels (article 16) avec l'empêchement du Président (article 7) sauf si l'extrait établit ce lien. " +
			"Pour l'article 16, après 30 et 60 jours le Conseil constitutionnel contrôle si les conditions demeurent réunies ; ce n'est pas une expiration automatique des pouvoirs à 30 jours. " +
			"Quand un extrait indique un numéro d'article (champ section= ou article=), nomme cet article explicitement dans ta réponse (ex. « l'article 16 ») en plus de [n]. " +
			"Utilise le numéro d'article exact tel qu'il apparaît dans l'extrait, sans le modifier. " +
			"Quand plusieurs extraits concernent des articles différents, fais les liens logiques entre eux quand c'est pertinent (ex. mesures exceptionnelles et report du scrutin)."
	}
	return " Before answering, analyze each excerpt and mentally classify it: directly relevant, general context, or off-topic. " +
		"Do not treat general-context excerpts (e.g. republican principles, sovereignty) as a direct answer to a question about a specific institution (e.g. President, Parliament). " +
		"If the question is about the President but only articles 1-3 on sovereignty are provided, do not present \"government of the people\" as a presidential obligation; say so clearly. " +
		"When Articles 16 and 7 (or other distinct articles) are relevant, answer in separate subsections titled \"Article 16 — emergency powers\" and \"Article 7 — presidential election\" without merging their regimes. " +
		"In the Article 16 subsection, if an excerpt contains \"dissoute\" or a ban on dissolving the Assemblée nationale, you must include an explicit sentence stating that ban with an [n] citation before moving to Article 7. " +
		"Do not equate Article 16 exceptional powers with Article 7 presidential empêchement unless an excerpt establishes that link. " +
		"For Article 16, after 30 and 60 days the Conseil constitutionnel reviews whether conditions still hold; this is oversight, not an automatic 30-day expiry of powers. " +
		"When an excerpt names an article (section= or article= field), name that article explicitly in your answer (e.g. \"Article 16\") in addition to [n]. " +
		"Use the exact article number as it appears in the excerpt. " +
		"When multiple excerpts cover different articles, explain logical links between them when relevant (e.g. emergency powers and election postponement)."
}

func ragGeneralKBOverlay(queryLang string) string {
	if queryLang == "fr" {
		return " Avant de répondre, analyse chaque extrait et classe-le : directement pertinent, partiellement pertinent, contexte général, ou hors sujet. " +
			"Ignore les mentions légales, copyright ou avertissements d'éditeur en en-tête d'extrait ; base ton analyse sur le champ Texte: (contenu utile). " +
			"Le champ section= peut contenir du bruit PDF : ne le traite pas comme le sujet de l'extrait si le Texte: traite d'un autre thème. " +
			"Pour une question comportant plusieurs parties (ex. développement logiciel et affaires), réponds séparément pour chaque partie : exploite les extraits pertinents pour les parties couvertes et indique clairement dans les limites ce qui manque. " +
			"Un extrait partiellement pertinent apporte des éléments de réponse même s'il ne couvre pas toute la question."
	}
	return " Before answering, classify each excerpt: directly relevant, partially relevant, general context, or off-topic. " +
		"Ignore legal notices, copyright, or publisher warnings in excerpt headers; base your analysis on the Texte: field (useful content). " +
		"The section= field may contain PDF noise: do not treat it as the excerpt topic when Texte: covers a different subject. " +
		"For multi-part questions, answer each part separately: use relevant excerpts for covered parts and state clearly in limits what is missing. " +
		"A partially relevant excerpt still provides useful answer elements even if it does not cover the whole question."
}

func ragSystemPrompt(queryLang string, docs []model.LegalDocument) string {
	base := ragBasePrompt(queryLang)
	if isLegalGenerationDocs(docs) {
		return base + ragLegalOverlay(queryLang)
	}
	return base + ragGeneralKBOverlay(queryLang)
}

func isLegalGenerationDocs(docs []model.LegalDocument) bool {
	for _, doc := range docs {
		if doc.Corpus == "legal-demo" || strings.Contains(strings.ToLower(doc.Corpus), "legal") {
			return true
		}
		if strings.TrimSpace(doc.Article) != "" {
			return true
		}
	}
	return false
}

func effectiveGenerationTopK(docs []model.LegalDocument, requestedTopK int) int {
	if requestedTopK <= 0 {
		requestedTopK = defaultGenerationTopK
	}
	if isLegalGenerationDocs(docs) && requestedTopK > legalGenerationTopK {
		return legalGenerationTopK
	}
	return requestedTopK
}

func detectQueryLanguage(query, langOverride string) string {
	if langOverride == "fr" || langOverride == "en" {
		return langOverride
	}
	lower := strings.ToLower(query)
	if strings.ContainsAny(query, "àâäéèêëïîôùûüçœ") {
		return "fr"
	}
	for _, word := range []string{"comment", "quoi", "pourquoi", "comment", "une", "les", "des", "est"} {
		if strings.Contains(lower, " "+word+" ") || strings.HasPrefix(lower, word+" ") {
			return "fr"
		}
	}
	return "en"
}

func buildRAGUserMessage(docs []model.LegalDocument, generationQuery, retrievalQuery string, topK int) string {
	topK = effectiveGenerationTopK(docs, topK)
	var b strings.Builder
	b.WriteString("Question: ")
	b.WriteString(generationQuery)
	b.WriteString("\n\nExcerpts:\n")

	used := 0
	for i, doc := range docs {
		if i >= topK {
			break
		}
		body := excerptTextForChunk(doc.Content, retrievalQuery, doc.Article, maxSnippetChars)
		sectionPath := displaySectionPath(doc)
		articleField := ""
		if art := strings.TrimSpace(doc.Article); art != "" {
			articleField = fmt.Sprintf(" article=%s", art)
		}
		excerpt := fmt.Sprintf("[%d] section=%s%s\nTexte:\n%s\n\n", i+1, sectionPath, articleField, body)
		if used+len(excerpt) > maxPromptContextChars {
			break
		}
		b.WriteString(excerpt)
		used += len(excerpt)
	}
	if checklist := legalGenerationChecklist(docs, retrievalQuery, topK); checklist != "" {
		if used+len(checklist) <= maxPromptContextChars {
			b.WriteString(checklist)
		}
	}
	return b.String()
}

func legalGenerationChecklist(docs []model.LegalDocument, retrievalQuery string, topK int) string {
	topK = effectiveGenerationTopK(docs, topK)
	var art16Dissoute, art7 bool
	for i, doc := range docs {
		if i >= topK {
			break
		}
		body := excerptTextForChunk(doc.Content, retrievalQuery, doc.Article, maxSnippetChars)
		lower := strings.ToLower(body)
		if strings.TrimSpace(doc.Article) == "16" && strings.Contains(lower, "dissoute") {
			art16Dissoute = true
		}
		if strings.TrimSpace(doc.Article) == "7" {
			art7 = true
		}
	}
	if !art16Dissoute && !art7 {
		return ""
	}
	var b strings.Builder
	b.WriteString("\nRappel pour la partie (2) :\n")
	if art16Dissoute {
		b.WriteString("- Article 16 : énoncer explicitement que l'Assemblée nationale ne peut être dissoute pendant les pouvoirs exceptionnels, avec citation [n].\n")
	}
	if art7 {
		b.WriteString("- Article 7 : énoncer les règles d'organisation ou de report du scrutin présidentiel pertinentes, avec citation [n].\n")
	}
	return b.String()
}

// generateResponseWithLLM streams an answer from the configured LLM backend over SSE.
func generateResponseWithLLM(docs []model.LegalDocument, generationQuery, retrievalQuery, langOverride string, topK int, rewriteQueries []string, extraMeta map[string]string, c *gin.Context) error {
	return generateResponseWithStream(docs, generationQuery, retrievalQuery, langOverride, topK, rewriteQueries, extraMeta, newGinStreamWriter(c))
}

func generateResponseWithStream(docs []model.LegalDocument, generationQuery, retrievalQuery, langOverride string, topK int, rewriteQueries []string, extraMeta map[string]string, w StreamWriter) error {
	queryLang := detectQueryLanguage(generationQuery, langOverride)
	systemPrompt := ragSystemPrompt(queryLang, docs)
	userPrompt := buildRAGUserMessage(docs, generationQuery, retrievalQuery, topK)

	metadata := map[string]string{
		"prompt":   userPrompt,
		"system":   systemPrompt,
		"model":    llmConfig.GenerationModel,
		"provider": string(llmConfig.Provider),
		"base_url": llmConfig.BaseURL,
		"lang":     queryLang,
	}
	for k, v := range extraMeta {
		metadata[k] = v
	}
	if len(rewriteQueries) > 0 {
		metadata["rewrite_queries"] = formatRetrievalQueriesDebug(rewriteQueries)
	}
	if err := w.WriteMetadata(metadata); err != nil {
		return err
	}

	ctx := context.Background()
	switch llmConfig.Provider {
	case ProviderOpenAI:
		return streamOpenAIChatCompletion(ctx, systemPrompt, userPrompt, metadata, w)
	default:
		return streamOllamaChat(ctx, systemPrompt, userPrompt, metadata, w)
	}
}

func writeSSEError(c *gin.Context, err error) {
	if err == nil {
		return
	}
	payload, _ := json.Marshal(gin.H{"error": err.Error()})
	c.SSEvent("error", string(payload))
	c.Writer.Flush()
}

func streamOllamaChat(ctx context.Context, systemPrompt, userPrompt string, metadata map[string]string, w StreamWriter) error {
	reqBody, err := json.Marshal(ollamaChatRequest{
		Model: llmConfig.GenerationModel,
		Messages: []openAIChatMessage{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: userPrompt},
		},
		Stream: true,
	})
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		llmConfig.generationEndpoint(),
		bytes.NewBuffer(reqBody),
	)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json; charset=utf-8")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("ollama chat (status %d): %s", resp.StatusCode, string(body))
	}

	decoder := json.NewDecoder(resp.Body)
	var buffer strings.Builder

	for {
		var chunk ollamaChatStreamChunk
		if err := decoder.Decode(&chunk); err != nil {
			if err == io.EOF {
				break
			}
			return err
		}

		text := chunk.Message.Content
		buffer.WriteString(text)
		if text != "" {
			if err := w.WriteToken(text); err != nil {
				return err
			}
		}

		if chunk.Done {
			return w.WriteComplete(buffer.String(), metadata)
		}
	}

	return nil
}

func streamOpenAIChatCompletion(ctx context.Context, systemPrompt, userPrompt string, metadata map[string]string, w StreamWriter) error {
	reqBody, err := json.Marshal(openAIChatRequest{
		Model: llmConfig.GenerationModel,
		Messages: []openAIChatMessage{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: userPrompt},
		},
		Stream: true,
	})
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		llmConfig.generationEndpoint(),
		bytes.NewBuffer(reqBody),
	)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json; charset=utf-8")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("chat completion (status %d): %s", resp.StatusCode, string(body))
	}

	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	var buffer strings.Builder
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		if !strings.HasPrefix(line, "data:") {
			continue
		}

		payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if payload == "[DONE]" {
			break
		}

		var chunk openAIChatStreamChunk
		if err := json.Unmarshal([]byte(payload), &chunk); err != nil {
			continue
		}
		if len(chunk.Choices) == 0 {
			continue
		}

		text := chunk.Choices[0].Delta.Content
		if text == "" {
			continue
		}

		buffer.WriteString(text)
		if err := w.WriteToken(text); err != nil {
			return err
		}
	}
	if err := scanner.Err(); err != nil {
		return err
	}

	return w.WriteComplete(buffer.String(), metadata)
}
