package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

type openAICompleteResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}

// completeLLM sends a non-streaming completion request to the configured LLM backend.
func completeLLM(ctx context.Context, systemPrompt, userPrompt string) (string, error) {
	switch llmConfig.Provider {
	case ProviderOpenAI:
		return completeOpenAIChat(ctx, systemPrompt, userPrompt)
	default:
		return completeOllamaChat(ctx, systemPrompt, userPrompt)
	}
}

type ollamaChatCompleteResponse struct {
	Message struct {
		Content string `json:"content"`
	} `json:"message"`
}

func completeOllamaChat(ctx context.Context, systemPrompt, userPrompt string) (string, error) {
	messages := []openAIChatMessage{{Role: "user", Content: userPrompt}}
	if systemPrompt != "" {
		messages = []openAIChatMessage{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: userPrompt},
		}
	}

	reqBody, err := json.Marshal(ollamaChatRequest{
		Model:    llmConfig.GenerationModel,
		Messages: messages,
		Stream:   false,
	})
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, llmConfig.generationEndpoint(), bytes.NewBuffer(reqBody))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json; charset=utf-8")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("ollama chat complete (status %d): %s", resp.StatusCode, string(body))
	}

	var parsed ollamaChatCompleteResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return "", err
	}
	return strings.TrimSpace(parsed.Message.Content), nil
}

func completeOpenAIChat(ctx context.Context, systemPrompt, userPrompt string) (string, error) {
	messages := []openAIChatMessage{{Role: "user", Content: userPrompt}}
	if systemPrompt != "" {
		messages = []openAIChatMessage{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: userPrompt},
		}
	}

	reqBody, err := json.Marshal(openAIChatRequest{
		Model:    llmConfig.GenerationModel,
		Messages: messages,
		Stream:   false,
	})
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, llmConfig.generationEndpoint(), bytes.NewBuffer(reqBody))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json; charset=utf-8")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("chat complete (status %d): %s", resp.StatusCode, string(body))
	}

	var parsed openAICompleteResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return "", err
	}
	if len(parsed.Choices) == 0 {
		return "", fmt.Errorf("chat complete: empty choices")
	}
	return strings.TrimSpace(parsed.Choices[0].Message.Content), nil
}
