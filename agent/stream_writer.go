package main

import (
	"encoding/json"

	"github.com/gin-gonic/gin"
)

// StreamWriter receives streamed LLM output events.
type StreamWriter interface {
	WriteMetadata(metadata map[string]string) error
	WriteToken(text string) error
	WriteComplete(response string, metadata map[string]string) error
	WriteError(err error) error
}

type ginStreamWriter struct {
	c *gin.Context
}

func newGinStreamWriter(c *gin.Context) *ginStreamWriter {
	c.Header("Content-Type", "text/event-stream; charset=utf-8")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("Transfer-Encoding", "chunked")
	return &ginStreamWriter{c: c}
}

func (w *ginStreamWriter) WriteMetadata(metadata map[string]string) error {
	metadataJSON, _ := json.Marshal(metadata)
	w.c.SSEvent("metadata", string(metadataJSON))
	w.c.Writer.Flush()
	return nil
}

func (w *ginStreamWriter) WriteToken(text string) error {
	sseMessage(w.c, text)
	return nil
}

func (w *ginStreamWriter) WriteComplete(response string, metadata map[string]string) error {
	completeResponse := map[string]interface{}{
		"response": response,
		"metadata": metadata,
	}
	completeJSON, _ := json.Marshal(completeResponse)
	w.c.SSEvent("complete", string(completeJSON))
	w.c.Writer.Flush()
	return nil
}

func (w *ginStreamWriter) WriteError(err error) error {
	writeSSEError(w.c, err)
	return nil
}

type bufferStreamWriter struct {
	metadata map[string]string
	answer   string
}

func (w *bufferStreamWriter) WriteMetadata(metadata map[string]string) error {
	w.metadata = metadata
	return nil
}

func (w *bufferStreamWriter) WriteToken(text string) error {
	w.answer += text
	return nil
}

func (w *bufferStreamWriter) WriteComplete(response string, metadata map[string]string) error {
	w.answer = response
	if metadata != nil {
		w.metadata = metadata
	}
	return nil
}

func (w *bufferStreamWriter) WriteError(err error) error {
	return err
}
