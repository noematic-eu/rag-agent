package main

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/noematic-eu/ai-rag-agent/model"
)

// ingestDocument gère l'ingestion des documents via l'endpoint /ingest
func ingestDocument(c *gin.Context) {
	var doc model.LegalDocument
	if err := c.BindJSON(&doc); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Requête invalide : " + err.Error()})
		return
	}

	result, err := ragAgent.Ingest(doc)
	if err != nil {
		if strings.Contains(err.Error(), "normalize content") {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Erreur normalisation du contenu : " + err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Erreur lors de l'indexation : " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": result.Status, "chunks": result.Chunks})
}
