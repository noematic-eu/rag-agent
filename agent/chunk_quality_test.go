package main

import (
	"strings"
	"testing"

	"github.com/noematic-eu/ai-rag-agent/model"
)

func TestIsIndexableChunkRejectsShortAndTitleOnly(t *testing.T) {
	short := model.Chunk{Text: "too short", Metadata: model.ChunkMetadata{Title: "T"}}
	if isIndexableChunk(short) {
		t.Fatal("expected short chunk to be rejected")
	}

	titleOnly := model.Chunk{
		Text:     "Build and Test",
		Metadata: model.ChunkMetadata{Title: "Build and Test"},
	}
	if isIndexableChunk(titleOnly) {
		t.Fatal("expected title-only chunk to be rejected")
	}

	long := model.Chunk{
		Text:     "Build and Test\n\nThis section explains how to build and test a retrieval pipeline with enough content to be useful for search indexing.",
		Metadata: model.ChunkMetadata{Title: "Build and Test"},
	}
	if !isIndexableChunk(long) {
		t.Fatal("expected substantive chunk to be accepted")
	}
}

func TestNormalizeChunkTextStripsHTML(t *testing.T) {
	in := "<p>Hello &amp; world</p>\n•\n<p>RAG steps</p>"
	out := normalizeChunkText(in)
	if out == "" {
		t.Fatal("expected non-empty output")
	}
	if strings.ContainsAny(out, "<>") || strings.Contains(out, "&amp;") {
		t.Fatalf("expected cleaned text, got %q", out)
	}
}
