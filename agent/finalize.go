package main

import (
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

// finalize to be called once all documents have been ingested.
// Use ?stream=1 for SSE progress events (event: progress | complete | error).
func finalize(c *gin.Context) {
	if wantsFinalizeStream(c) {
		finalizeStream(c)
		return
	}
	result, err := ragAgent.Finalize()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "finalize failed: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, result)
}

func wantsFinalizeStream(c *gin.Context) bool {
	if strings.TrimSpace(c.Query("stream")) == "1" {
		return true
	}
	return strings.Contains(c.GetHeader("Accept"), "text/event-stream")
}

func finalizeStream(c *gin.Context) {
	w := newGinStreamWriter(c)
	_ = w.WriteAgentEvent("status", map[string]interface{}{"phase": "connected"})

	if stats := currentLexicalIndexStats(); stats.Rebuilding {
		streamLexicalRebuildAttach(w)
		return
	}

	// Another request may hold lexicalRebuildMu during pre-count; poll until free or rebuilding.
	for !lexicalRebuildMu.TryLock() {
		_ = w.WriteAgentEvent("status", map[string]interface{}{"phase": "waiting"})
		if stats := currentLexicalIndexStats(); stats.Rebuilding {
			streamLexicalRebuildAttach(w)
			return
		}
		time.Sleep(500 * time.Millisecond)
	}
	lexicalRebuildMu.Unlock()

	result, err := ragAgent.FinalizeWithProgress(func(indexed, total int) {
		_ = w.WriteAgentEvent("progress", lexicalProgressPayload(indexed, total))
	})
	if err != nil {
		_ = w.WriteError(err)
		return
	}
	_ = w.WriteAgentEvent("complete", map[string]interface{}{
		"status":         result.Status,
		"chunks_indexed": result.ChunksIndexed,
		"chunks_total":   result.ChunksTotal,
		"duration_s":     result.DurationSec,
	})
}

func lexicalProgressPayload(indexed, total int) map[string]interface{} {
	payload := map[string]interface{}{
		"chunks_indexed": indexed,
		"chunks_total":   total,
	}
	if total > 0 {
		payload["percent"] = float64(indexed) * 100 / float64(total)
	}
	return payload
}

// streamLexicalRebuildAttach relays progress from GET /stats while another handler rebuilds.
func streamLexicalRebuildAttach(w StreamWriter) {
	_ = w.WriteAgentEvent("status", map[string]interface{}{"phase": "attached"})
	lastIndexed := -1
	for {
		stats := currentLexicalIndexStats()
		if stats.ChunksIndexed != lastIndexed {
			_ = w.WriteAgentEvent("progress", lexicalProgressPayload(stats.ChunksIndexed, stats.ChunksTotal))
			lastIndexed = stats.ChunksIndexed
		}
		if !stats.Rebuilding {
			_ = w.WriteAgentEvent("complete", map[string]interface{}{
				"status":         "finalized",
				"chunks_indexed": stats.ChunksIndexed,
				"chunks_total":   stats.ChunksTotal,
				"duration_s":     stats.DurationSec,
			})
			return
		}
		time.Sleep(500 * time.Millisecond)
	}
}
