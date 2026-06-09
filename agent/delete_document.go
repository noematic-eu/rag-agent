package main

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/noematic-eu/ai-rag-agent/internal/f4kvs"
	"github.com/noematic-eu/ai-rag-agent/model"
)

// deleteDocument removes all chunks and metadata for doc_id (DELETE /documents/:doc_id).
func deleteDocument(c *gin.Context) {
	docID := strings.TrimSpace(c.Param("doc_id"))
	if docID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "doc_id is required"})
		return
	}

	result, err := ragAgent.DeleteDocument(docID)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			c.JSON(http.StatusNotFound, gin.H{"error": "document not found", "doc_id": docID})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "delete failed: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status":         result.Status,
		"doc_id":         result.DocID,
		"chunks_deleted": result.ChunksDeleted,
		"corpus":         result.Corpus,
	})
}

func deleteDocumentByID(docID string) (deleted int, corpus string, err error) {
	if chunkStore == nil {
		return 0, "", errors.New("chunk store not initialized")
	}

	pairs, err := chunkStore.ScanPrefix("chunk:")
	if err != nil {
		return 0, "", err
	}

	var chunkIDs []string
	for _, pair := range pairs {
		var chunk model.Chunk
		if err := json.Unmarshal(pair.Value, &chunk); err != nil {
			continue
		}
		if chunk.Metadata.DocID != docID {
			continue
		}
		if corpus == "" && chunk.Metadata.Corpus != "" {
			corpus = chunk.Metadata.Corpus
		}
		chunkIDs = append(chunkIDs, chunk.Metadata.ChunkID)
	}

	for _, chunkID := range chunkIDs {
		if lexicalBackend != nil {
			if err := lexicalBackend.DeleteChunk(chunkID); err != nil {
				log.Printf("lexical delete chunk %s: %v", chunkID, err)
			}
		}
		if err := chunkStore.Delete("chunk:" + chunkID); err != nil && !errors.Is(err, f4kvs.ErrNotFound) {
			return deleted, corpus, err
		}
		deleted++
	}

	if err := chunkStore.Delete("doc:" + docID); err != nil && !errors.Is(err, f4kvs.ErrNotFound) {
		return deleted, corpus, err
	}

	if deleted > 0 {
		recordDocumentDelete(docID, corpus, deleted)
	}
	return deleted, corpus, nil
}
