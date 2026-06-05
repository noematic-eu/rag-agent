package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/noematic-eu/ai-rag-agent/model"
)

type ollamaEmbeddingRequest struct {
	Model string   `json:"model"`
	Input []string `json:"input"`
}

type ollamaEmbeddingResponse struct {
	Embeddings [][]float64 `json:"embeddings"`
}

type openAIEmbeddingRequest struct {
	Model string   `json:"model"`
	Input []string `json:"input"`
}

type openAIEmbeddingResponse struct {
	Data []struct {
		Embedding []float64 `json:"embedding"`
		Index     int       `json:"index"`
	} `json:"data"`
}

// EmbedTextBatch generates embeddings for a batch of texts.
func EmbedTextBatch(texts []string) ([][]float64, error) {
	if len(texts) == 0 {
		return [][]float64{}, nil
	}

	switch llmConfig.Provider {
	case ProviderOpenAI:
		return embedTextBatchOpenAI(texts)
	default:
		return embedTextBatchOllama(texts)
	}
}

func embedTextBatchOllama(texts []string) ([][]float64, error) {
	reqBody, err := json.Marshal(ollamaEmbeddingRequest{
		Model: llmConfig.EmbeddingModel,
		Input: texts,
	})
	if err != nil {
		return nil, fmt.Errorf("erreur lors de la création de la requête d'embedding: %v", err)
	}

	body, status, err := postLLM(llmConfig.embeddingsEndpoint(), reqBody)
	if err != nil {
		return nil, err
	}
	if status < 200 || status >= 300 {
		return nil, fmt.Errorf("erreur embedding (status %d): %s", status, string(body))
	}

	var embeddingResp ollamaEmbeddingResponse
	if err := json.Unmarshal(body, &embeddingResp); err != nil {
		return nil, fmt.Errorf("erreur lors du décodage JSON de la réponse d'embedding: %v", err)
	}

	return validateEmbeddings(texts, embeddingResp.Embeddings)
}

func embedTextBatchOpenAI(texts []string) ([][]float64, error) {
	reqBody, err := json.Marshal(openAIEmbeddingRequest{
		Model: llmConfig.EmbeddingModel,
		Input: texts,
	})
	if err != nil {
		return nil, fmt.Errorf("erreur lors de la création de la requête d'embedding: %v", err)
	}

	body, status, err := postLLM(llmConfig.embeddingsEndpoint(), reqBody)
	if err != nil {
		return nil, err
	}
	if status < 200 || status >= 300 {
		return nil, fmt.Errorf("erreur embedding (status %d): %s", status, string(body))
	}

	var embeddingResp openAIEmbeddingResponse
	if err := json.Unmarshal(body, &embeddingResp); err != nil {
		return nil, fmt.Errorf("erreur lors du décodage JSON de la réponse d'embedding: %v", err)
	}

	embeddings := make([][]float64, len(texts))
	for _, item := range embeddingResp.Data {
		if item.Index < 0 || item.Index >= len(embeddings) {
			return nil, fmt.Errorf("index d'embedding invalide: %d", item.Index)
		}
		embeddings[item.Index] = item.Embedding
	}

	return validateEmbeddings(texts, embeddings)
}

func validateEmbeddings(texts []string, embeddings [][]float64) ([][]float64, error) {
	if len(embeddings) != len(texts) {
		return nil, fmt.Errorf("nombre d'embeddings invalide: attendu=%d, reçu=%d", len(texts), len(embeddings))
	}
	for i, emb := range embeddings {
		if len(emb) == 0 {
			return nil, fmt.Errorf("embedding vide pour l'input %d", i)
		}
	}
	return embeddings, nil
}

func postLLM(endpoint string, reqBody []byte) ([]byte, int, error) {
	req, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewBuffer(reqBody))
	if err != nil {
		return nil, 0, fmt.Errorf("erreur lors de la création de la requête HTTP: %v", err)
	}
	req.Header.Set("Content-Type", "application/json; charset=utf-8")

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("erreur lors de l'appel au service LLM: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, fmt.Errorf("erreur lors de la lecture de la réponse: %v", err)
	}
	return body, resp.StatusCode, nil
}

// EmbedChunk generates an embedding for a single chunk.
func EmbedChunk(chunk model.Chunk) (model.Chunk, error) {
	embeddings, err := EmbedTextBatch([]string{chunk.Text})
	if err != nil {
		return chunk, fmt.Errorf("erreur lors de l'embedding du chunk %s: %v", chunk.Metadata.ChunkID, err)
	}
	if len(embeddings) > 0 {
		chunk.Embedding = embeddings[0]
	}
	return chunk, nil
}
