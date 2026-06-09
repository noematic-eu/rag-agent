package main

import (
	"strings"
	"testing"

	"github.com/noematic-eu/ai-rag-agent/model"
)

func TestBleveSetup(t *testing.T) {
	t.Skip("Legacy setup test is not stable with shared globals")
}

func TestIndexDocument(t *testing.T) {
	tempBleveDir := t.TempDir()
	tempChunkStoreDir := t.TempDir()
	setupTestDatabases(t, tempBleveDir, tempChunkStoreDir)

	tests := []struct {
		name    string
		doc     model.LegalDocument
		query   string
		wantHit bool
	}{
		{
			name: "basic document indexing",
			doc: model.LegalDocument{
				ID:      "test1",
				Title:   "Test Document",
				Content: "# Legal matters\n\nThis is a test document about legal matters and contractual obligations with enough text to pass chunk quality filters for search indexing in the test harness.",
			},
			query:   "legal",
			wantHit: true,
		},
		{
			name: "document with special characters",
			doc: model.LegalDocument{
				ID:      "test2",
				Title:   "Legal Document 你好",
				Content: "# Special characters\n\nThis is a legal document with special characters: 你好, ñ, é. It includes additional prose so the chunking pipeline produces at least one indexable segment for Bleve search validation.",
			},
			query:   "special characters",
			wantHit: true,
		},
		{
			name: "no match query",
			doc: model.LegalDocument{
				ID:      "test3",
				Title:   "Test Document",
				Content: "This is a test document",
			},
			query:   "nonexistent",
			wantHit: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Index the document
			if _, err := indexDocument(tt.doc); err != nil {
				t.Fatalf("indexDocument failed: %v", err)
			}

			hits, err := lexicalBackend.Search(tt.query, "", 10)
			if err != nil {
				t.Fatalf("search failed: %v", err)
			}

			gotHit := len(hits) > 0
			if gotHit != tt.wantHit {
				t.Errorf("search for query %q: got hit = %v, want hit = %v", tt.query, gotHit, tt.wantHit)
			}

			if tt.wantHit && len(hits) > 0 {
				if !strings.HasPrefix(hits[0].ChunkID, tt.doc.ID+"-chunk-") {
					t.Errorf("got chunk ID %v, want prefix %v", hits[0].ChunkID, tt.doc.ID+"-chunk-")
				}
			}
		})
	}
}

func TestBleveInitialization(t *testing.T) {
	setupTestDatabases(t, t.TempDir(), t.TempDir())
	if lexicalBackend == nil {
		t.Fatal("lexical backend not initialized")
	}
	if lexicalBackend.Engine() != "bleve" {
		t.Fatalf("engine: %s", lexicalBackend.Engine())
	}
}

func TestParseMarkdown(t *testing.T) {
	tests := []struct {
		name     string
		markdown string
		expected string
	}{
		{
			name:     "basic text",
			markdown: "This is a test",
			expected: "This is a test",
		},
		{
			name:     "with formatting",
			markdown: "**Bold** and *italic*",
			expected: "Bold and italic",
		},
		{
			name:     "with headers",
			markdown: "# Header\n## Subheader\nContent",
			expected: "Header Subheader Content",
		},
		{
			name:     "with links",
			markdown: "[Link text](http://example.com)",
			expected: "Link text",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseMarkdown(tt.markdown)
			// Normalize spaces in result and expected
			result = strings.Join(strings.Fields(result), " ")
			expected := strings.Join(strings.Fields(tt.expected), " ")
			if result != expected {
				t.Errorf("parseMarkdown() = %q, want %q", result, expected)
			}
		})
	}
}
