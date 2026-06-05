package main

import (
	"fmt"
	"log"
	"os"
	"strings"
)

// LLMProvider selects the HTTP API shape for generation and embeddings.
type LLMProvider string

const (
	ProviderOllama LLMProvider = "ollama"
	ProviderOpenAI LLMProvider = "openai" // LM Studio, vLLM, OpenAI, etc.
)

// LLMConfig holds runtime settings for the upstream LLM service.
type LLMConfig struct {
	Provider          LLMProvider
	BaseURL           string
	GenerationModel   string
	EmbeddingModel    string
	EmbeddingsEnabled bool
}

var llmConfig LLMConfig

func defaultLLMConfig() LLMConfig {
	return LLMConfig{
		Provider:          ProviderOllama,
		BaseURL:           "http://localhost:11434",
		GenerationModel:   "qwq",
		EmbeddingModel:    "nomic-embed-text",
		EmbeddingsEnabled: true,
	}
}

func envOr(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}

func parseProvider(raw string) (LLMProvider, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", "ollama":
		return ProviderOllama, nil
	case "openai", "lmstudio", "lm-studio":
		return ProviderOpenAI, nil
	default:
		return "", fmt.Errorf("unsupported llm provider %q (use ollama or openai)", raw)
	}
}

func parseBool(raw string) bool {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func applyLLMConfig(provider, baseURL, generationModel, embeddingModel string, disableEmbeddings bool) error {
	cfg := defaultLLMConfig()

	p, err := parseProvider(provider)
	if err != nil {
		return err
	}
	cfg.Provider = p

	if baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/"); baseURL != "" {
		cfg.BaseURL = baseURL
	} else if cfg.Provider == ProviderOpenAI {
		cfg.BaseURL = "http://localhost:1234/v1"
	}

	if generationModel = strings.TrimSpace(generationModel); generationModel != "" {
		cfg.GenerationModel = generationModel
	}
	if embeddingModel = strings.TrimSpace(embeddingModel); embeddingModel != "" {
		cfg.EmbeddingModel = embeddingModel
	}
	cfg.EmbeddingsEnabled = !disableEmbeddings

	if cfg.GenerationModel == "" {
		return fmt.Errorf("generation model must not be empty")
	}
	if cfg.EmbeddingsEnabled && cfg.EmbeddingModel == "" {
		return fmt.Errorf("embedding model must not be empty")
	}

	llmConfig = cfg
	return nil
}

func logLLMConfig() {
	log.Printf(
		"LLM config: provider=%s base_url=%s generation_model=%s embedding_model=%s embeddings_enabled=%t",
		llmConfig.Provider,
		llmConfig.BaseURL,
		llmConfig.GenerationModel,
		llmConfig.EmbeddingModel,
		llmConfig.EmbeddingsEnabled,
	)
}

func (c LLMConfig) generationEndpoint() string {
	switch c.Provider {
	case ProviderOpenAI:
		return c.BaseURL + "/chat/completions"
	default:
		return c.BaseURL + "/api/generate"
	}
}

func (c LLMConfig) embeddingsEndpoint() string {
	switch c.Provider {
	case ProviderOpenAI:
		return c.BaseURL + "/embeddings"
	default:
		return c.BaseURL + "/api/embeddings"
	}
}
