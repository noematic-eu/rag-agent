package main

import (
	"context"
	"errors"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/noematic-eu/ai-rag-agent/agent/p9fs"
)

func main() {
	provider := flag.String("llm-provider", "", "LLM API provider: ollama or openai (LM Studio). Env: RAG_LLM_PROVIDER")
	baseURL := flag.String("llm-base-url", "", "LLM base URL (e.g. http://localhost:1234/v1). Env: RAG_LLM_BASE_URL")
	generationModel := flag.String("llm-model", "", "Model used for /search answers. Env: RAG_LLM_MODEL")
	embeddingModel := flag.String("embedding-model", "", "Model used for embeddings. Env: RAG_EMBEDDING_MODEL")
	disableEmbeddings := flag.Bool("disable-embeddings", false, "Disable embedding generation and vector search. Env: RAG_DISABLE_EMBEDDINGS")
	addr := flag.String("addr", "", "HTTP listen address (e.g. :8080, 127.0.0.1:8081). Env: RAG_LISTEN")
	p9Addr := flag.String("9p-addr", "", "9P listen address (e.g. unix!/tmp/rag9p, tcp!127.0.0.1!5640). Env: RAG_9P_ADDR")
	dataDir := flag.String("data-dir", "", "Directory for indexes and legal.f4kvs. Env: RAG_DATA_DIR")
	lexicalEngine := flag.String("lexical-engine", "", "Lexical index: bleve, tantivy, or f4kvs. Env: RAG_LEXICAL_ENGINE")
	flag.Parse()

	cfgProvider := envOr("RAG_LLM_PROVIDER", "ollama")
	cfgBaseURL := envOr("RAG_LLM_BASE_URL", "")
	cfgGenerationModel := envOr("RAG_LLM_MODEL", "")
	cfgEmbeddingModel := envOr("RAG_EMBEDDING_MODEL", "")
	cfgDisableEmbeddings := parseBool(envOr("RAG_DISABLE_EMBEDDINGS", "false"))

	if *provider != "" {
		cfgProvider = *provider
	}
	if *baseURL != "" {
		cfgBaseURL = *baseURL
	}
	if *generationModel != "" {
		cfgGenerationModel = *generationModel
	}
	if *embeddingModel != "" {
		cfgEmbeddingModel = *embeddingModel
	}
	if *disableEmbeddings {
		cfgDisableEmbeddings = true
	}

	if err := applyLLMConfig(cfgProvider, cfgBaseURL, cfgGenerationModel, cfgEmbeddingModel, cfgDisableEmbeddings); err != nil {
		log.Fatal(err)
	}
	logLLMConfig()

	agentCfg, err := resolveAgentConfig(*addr, *p9Addr, *dataDir, *lexicalEngine)
	if err != nil {
		log.Fatal(err)
	}
	if err := openStores(agentCfg); err != nil {
		log.Fatal("Erreur lors de l'initialisation des stores :", err)
	}
	if err := maybeCompactChunkStore(); err != nil {
		log.Printf("f4kvs compaction warning: %v", err)
	}
	log.Printf(
		"data-dir=%s, lexical_engine=%s, f4kvs=%s, http=%s, 9p=%s",
		agentCfg.DataDir,
		agentCfg.LexicalEngine,
		agentCfg.chunkStorePath(),
		agentCfg.Listen,
		agentCfg.P9Addr,
	)

	var srv *http.Server
	if agentCfg.P9Addr != "" {
		if err := p9fs.Serve(agentCfg.P9Addr, newP9Service(ragAgent)); err != nil {
			log.Fatal("Erreur lors du démarrage du serveur 9P :", err)
		}
	}

	if agentCfg.Listen != "" && agentCfg.Listen != "off" && agentCfg.Listen != "none" {
		initCORS()

		r := gin.Default()
		r.Use(corsMiddleware())
		r.POST("/ingest", ingestDocument)
		r.DELETE("/documents/:doc_id", deleteDocument)
		r.POST("/finalize", finalize)
		r.POST("/reset", resetIndex)
		r.GET("/search", searchDocuments)
		r.GET("/retrieve", retrieveDocuments)
		r.GET("/stats", statsHandler)

		srv = &http.Server{Addr: agentCfg.Listen, Handler: r}
		go func() {
			log.Printf("listening on %s", agentCfg.Listen)
			if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
				log.Fatal("Erreur lors du démarrage du serveur :", err)
			}
		}()
	}

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("Arrêt en cours…")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if srv != nil {
		if err := srv.Shutdown(ctx); err != nil {
			log.Printf("Arrêt du serveur HTTP : %v", err)
		}
	}
	if err := closeStores(); err != nil {
		log.Printf("Fermeture des bases : %v", err)
	}
	log.Println("Arrêt terminé")
}
