package main

import (
	"encoding/json"
	"fmt"

	"github.com/gin-gonic/gin"
)

// sseMessage emits a streamed text token. Payload is JSON-encoded on the wire as
// data:<json> (no space after "data:") so leading spaces are never stripped.
func sseMessage(c *gin.Context, text string) {
	if text == "" {
		return
	}
	encoded, err := json.Marshal(text)
	if err != nil {
		encoded = []byte(text)
	}
	_, _ = fmt.Fprintf(c.Writer, "event:message\ndata:%s\n\n", encoded)
	c.Writer.Flush()
}
