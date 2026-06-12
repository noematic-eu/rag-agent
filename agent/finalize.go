package main

import (
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

// finalize catches up lex:* for chunks not yet indexed (incremental by default).
// Use ?full=1 to wipe and rebuild all lex:* keys (slow on large corpora).
// Use ?stream=1 for SSE progress events (event: progress | complete | error).
// Rebuild runs in the background so /ingest and /search stay responsive.
func finalize(c *gin.Context) {
	if wantsFinalizeStream(c) {
		finalizeStream(c)
		return
	}
	if currentLexicalIndexStats().Rebuilding {
		c.JSON(http.StatusConflict, gin.H{
			"status":  "rebuilding",
			"message": "lexical rebuild already in progress; poll GET /stats",
		})
		return
	}
	startLexicalRebuildAsync(wantsFullLexicalRebuild(c))
	c.JSON(http.StatusAccepted, gin.H{
		"status":  "rebuilding",
		"message": "lexical rebuild started; poll GET /stats for lexical_index progress",
	})
}

func wantsFinalizeStream(c *gin.Context) bool {
	if strings.TrimSpace(c.Query("stream")) == "1" {
		return true
	}
	return strings.Contains(c.GetHeader("Accept"), "text/event-stream")
}

func wantsFullLexicalRebuild(c *gin.Context) bool {
	switch strings.ToLower(strings.TrimSpace(c.Query("full"))) {
	case "1", "true", "yes":
		return true
	default:
		return false
	}
}

func startLexicalRebuildAsync(fullRebuild bool) {
	go func() {
		_, err := rebuildLexicalFromChunkStore(lexicalRebuildProgressCallback(), fullRebuild)
		if err != nil {
			log.Printf("finalize: rebuild failed: %v", err)
		}
	}()
}

func finalizeStream(c *gin.Context) {
	w := newGinStreamWriter(c)
	_ = w.WriteAgentEvent("status", map[string]interface{}{"phase": "connected"})

	if stats := currentLexicalIndexStats(); stats.Rebuilding {
		streamLexicalRebuildAttach(w)
		return
	}

	startLexicalRebuildAsync(wantsFullLexicalRebuild(c))
	waitForLexicalRebuildStart()
	streamLexicalRebuildAttach(w)
}

func waitForLexicalRebuildStart() {
	for i := 0; i < 40; i++ {
		if currentLexicalIndexStats().Rebuilding {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
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
