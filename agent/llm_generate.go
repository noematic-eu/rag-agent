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

type ollamaGenerateRequest struct {
	Model  string `json:"model"`
	Prompt string `json:"prompt"`
	Stream bool   `json:"stream"`
}

type ollamaGenerateResponse struct {
	Response string `json:"response"`
	Done     bool   `json:"done"`
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

func ragSystemPrompt(queryLang string) string {
	if queryLang == "fr" {
		return "Tu es un assistant qui répond uniquement à partir des extraits fournis. " +
			"Avant de répondre, analyse chaque extrait et classe-le mentalement : directement pertinent, contexte général, ou hors sujet. " +
			"Ne traite pas un extrait de contexte général (ex. principes républicains, souveraineté) comme réponse directe à une question sur une institution précise (ex. Président, Parlement). " +
			"Si la question porte sur le Président mais que seuls les articles 1-3 sur la souveraineté sont fournis, ne présente pas « gouvernement du peuple » comme obligation présidentielle ; dis-le clairement. " +
			"Structure ta réponse en trois parties : (1) analyse brève des extraits pertinents vs hors sujet, (2) réponse structurée avec citations [n], (3) limites de ce que les extraits ne couvrent pas. " +
			"Chaque affirmation doit citer la source avec [n] (numéro d'extrait). " +
			"Quand un extrait indique un numéro d'article (champ section= ou article=), nomme cet article explicitement dans ta réponse (ex. « l'article 16 ») en plus de [n]. " +
			"Utilise le numéro d'article exact tel qu'il apparaît dans l'extrait, sans le modifier. " +
			"Quand plusieurs extraits concernent des articles différents, fais les liens logiques entre eux quand c'est pertinent (ex. mesures exceptionnelles et report du scrutin). " +
			"Ne fusionne pas des idées de plusieurs extraits en une seule attribution; une citation [n] = un extrait. " +
			"Ne nomme pas d'auteur ou de livre sauf s'ils apparaissent dans le texte de cet extrait. " +
			"Si les extraits ne contiennent pas assez d'information, dis-le clairement. " +
			"N'utilise aucune connaissance externe. Réponds dans la même langue que la question."
	}
	return "You are an assistant that answers only from the provided excerpts. " +
		"Before answering, analyze each excerpt and mentally classify it: directly relevant, general context, or off-topic. " +
		"Do not treat general-context excerpts (e.g. republican principles, sovereignty) as a direct answer to a question about a specific institution (e.g. President, Parliament). " +
		"If the question is about the President but only articles 1-3 on sovereignty are provided, do not present \"government of the people\" as a presidential obligation; say so clearly. " +
		"Structure your answer in three parts: (1) brief analysis of relevant vs off-topic excerpts, (2) structured answer with [n] citations, (3) limits of what the excerpts do not cover. " +
		"Every claim must cite its source with [n] (excerpt number). " +
		"When an excerpt names an article (section= or article= field), name that article explicitly in your answer (e.g. \"Article 16\") in addition to [n]. " +
		"Use the exact article number as it appears in the excerpt. " +
		"When multiple excerpts cover different articles, explain logical links between them when relevant (e.g. emergency powers and election postponement). " +
		"Do not merge ideas from different excerpts into one attribution; one [n] = one excerpt. " +
		"Do not name authors or books unless they appear in that excerpt's text. " +
		"If the excerpts do not contain enough information, say so clearly. " +
		"Do not use outside knowledge. Answer in the same language as the question."
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
	if topK <= 0 {
		topK = defaultGenerationTopK
	}
	var b strings.Builder
	b.WriteString("Question: ")
	b.WriteString(generationQuery)
	b.WriteString("\n\nExcerpts:\n")

	used := 0
	for i, doc := range docs {
		if i >= topK {
			break
		}
		body := excerptText(doc.Content, retrievalQuery, maxSnippetChars)
		book := strings.TrimSpace(doc.BookTitle)
		section := strings.TrimSpace(doc.Title)
		sectionPath := section
		if book != "" && book != section {
			sectionPath = book + " -> " + section
		}
		if book == "" {
			book = section
		}
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
	return b.String()
}

// generateResponseWithLLM streams an answer from the configured LLM backend over SSE.
func generateResponseWithLLM(docs []model.LegalDocument, generationQuery, retrievalQuery, langOverride string, topK int, rewriteQueries []string, c *gin.Context) error {
	return generateResponseWithStream(docs, generationQuery, retrievalQuery, langOverride, topK, rewriteQueries, newGinStreamWriter(c))
}

func generateResponseWithStream(docs []model.LegalDocument, generationQuery, retrievalQuery, langOverride string, topK int, rewriteQueries []string, w StreamWriter) error {
	queryLang := detectQueryLanguage(generationQuery, langOverride)
	systemPrompt := ragSystemPrompt(queryLang)
	userPrompt := buildRAGUserMessage(docs, generationQuery, retrievalQuery, topK)

	metadata := map[string]string{
		"prompt":   userPrompt,
		"system":   systemPrompt,
		"model":    llmConfig.GenerationModel,
		"provider": string(llmConfig.Provider),
		"base_url": llmConfig.BaseURL,
		"lang":     queryLang,
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
		combined := systemPrompt + "\n\n" + userPrompt
		return streamOllamaGenerate(ctx, combined, metadata, w)
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

func streamOllamaGenerate(ctx context.Context, prompt string, metadata map[string]string, w StreamWriter) error {
	reqBody, err := json.Marshal(ollamaGenerateRequest{
		Model:  llmConfig.GenerationModel,
		Prompt: prompt,
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
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("ollama generate (status %d): %s", resp.StatusCode, string(body))
	}

	decoder := json.NewDecoder(resp.Body)
	var buffer strings.Builder

	for {
		var chunk ollamaGenerateResponse
		if err := decoder.Decode(&chunk); err != nil {
			if err == io.EOF {
				break
			}
			return err
		}

		buffer.WriteString(chunk.Response)
		if chunk.Response != "" {
			if err := w.WriteToken(chunk.Response); err != nil {
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
	defer resp.Body.Close()

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
