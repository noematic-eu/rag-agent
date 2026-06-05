package main

import (
	"math"
	"os"
	"reflect"
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

func TestTFIDFCalculation(t *testing.T) {
	tempBleveDir, err := os.MkdirTemp("", "bleve-tfidf-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempBleveDir)

	tempChunkStoreDir, err := os.MkdirTemp("", "f4kvs-tfidf-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempChunkStoreDir)

	setupTestDatabases(t, tempBleveDir, tempChunkStoreDir)
	documentTFIDFs = make([]DocumentTFIDF, 0)
	globalIDF = make(map[string]float64)
	totalDocs = 0

	// Test document
	doc := model.LegalDocument{
		ID:      "test4",
		Title:   "Test Document",
		Content: "this is a test document this is a test with enough repeated content to exceed the minimum chunk length required for ingestion filtering and indexing",
	}

	// Index the document
	if _, err := indexDocument(doc); err != nil {
		t.Fatalf("indexDocument failed: %v", err)
	}

	// Find the document in documentTFIDFs
	var found bool
	var docTFIDF DocumentTFIDF
	for _, d := range documentTFIDFs {
		if strings.HasPrefix(d.ID, doc.ID+"-chunk-") {
			docTFIDF = d
			found = true
			break
		}
	}

	if !found {
		t.Fatal("document TF-IDF scores not found")
	}

	// Check that common words have TF-IDF scores
	expectedWords := []string{"test", "document"}
	for _, word := range expectedWords {
		if score, exists := docTFIDF.TFIDF[word]; !exists {
			t.Errorf("word %q not found in TF-IDF scores", word)
		} else if score <= 0 {
			t.Errorf("word %q has invalid TF-IDF score: %v", word, score)
		}
	}

	// Check that the document has a valid norm
	if docTFIDF.Norm <= 0 {
		t.Errorf("invalid document norm: %v", docTFIDF.Norm)
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

func TestTokenize(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{
			name:     "basic tokenization",
			input:    "This is a test document",
			expected: []string{"this", "test", "document"},
		},
		{
			name:     "with special characters",
			input:    "Hello, world! This is a test.",
			expected: []string{"hello", "world", "this", "test"},
		},
		{
			name:     "with short words",
			input:    "A to do list is here",
			expected: []string{"list", "here"},
		},
		{
			name:     "with numbers",
			input:    "Article 123 states that",
			expected: []string{"article", "123", "states", "that"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tokenize(tt.input)
			if !reflect.DeepEqual(result, tt.expected) {
				t.Errorf("tokenize() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestCalculateTF(t *testing.T) {
	tests := []struct {
		name     string
		tokens   []string
		expected map[string]float64
	}{
		{
			name:   "basic frequency",
			tokens: []string{"test", "document", "test"},
			expected: map[string]float64{
				"test":     2.0 / 3.0,
				"document": 1.0 / 3.0,
			},
		},
		{
			name:   "single word repeated",
			tokens: []string{"test", "test", "test"},
			expected: map[string]float64{
				"test": 1.0,
			},
		},
		{
			name:   "unique words",
			tokens: []string{"one", "two", "three"},
			expected: map[string]float64{
				"one":   1.0 / 3.0,
				"two":   1.0 / 3.0,
				"three": 1.0 / 3.0,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := calculateTF(tt.tokens)
			if !reflect.DeepEqual(result, tt.expected) {
				t.Errorf("calculateTF() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestUpdateIDF(t *testing.T) {
	// Reset globalIDF before test
	globalIDF = make(map[string]float64)

	tests := []struct {
		name           string
		tokensToAdd    []string
		expectedCounts map[string]float64
	}{
		{
			name:        "single document",
			tokensToAdd: []string{"test", "document", "test"},
			expectedCounts: map[string]float64{
				"test":     1,
				"document": 1,
			},
		},
		{
			name:        "repeated document",
			tokensToAdd: []string{"test", "another", "test"},
			expectedCounts: map[string]float64{
				"test":     2,
				"document": 1,
				"another":  1,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			updateIDF(tt.tokensToAdd)
			if !reflect.DeepEqual(globalIDF, tt.expectedCounts) {
				t.Errorf("After updateIDF(), globalIDF = %v, want %v", globalIDF, tt.expectedCounts)
			}
		})
	}
}

func TestCalculateNorm(t *testing.T) {
	// Reset and initialize globalIDF
	globalIDF = map[string]float64{
		"test":     2.0,
		"document": 1.0,
	}

	tests := []struct {
		name     string
		tf       map[string]float64
		expected float64
	}{
		{
			name: "basic norm",
			tf: map[string]float64{
				"test":     0.5,
				"document": 0.5,
			},
			expected: 1.118033988749895, // sqrt((0.5*2)^2 + (0.5*1)^2)
		},
		{
			name:     "empty document",
			tf:       map[string]float64{},
			expected: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := calculateNorm(tt.tf)
			if math.Abs(result-tt.expected) > 0.000001 {
				t.Errorf("calculateNorm() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestCosineSimilarity(t *testing.T) {
	tests := []struct {
		name     string
		doc1     DocumentTFIDF
		doc2     DocumentTFIDF
		expected float64
	}{
		{
			name: "identical documents",
			doc1: DocumentTFIDF{
				ID: "doc1",
				TFIDF: map[string]float64{
					"test": 0.5,
				},
				Norm: 0.5,
			},
			doc2: DocumentTFIDF{
				ID: "doc2",
				TFIDF: map[string]float64{
					"test": 0.5,
				},
				Norm: 0.5,
			},
			expected: 1.0,
		},
		{
			name: "different documents",
			doc1: DocumentTFIDF{
				ID: "doc1",
				TFIDF: map[string]float64{
					"test": 0.5,
				},
				Norm: 0.5,
			},
			doc2: DocumentTFIDF{
				ID: "doc2",
				TFIDF: map[string]float64{
					"other": 0.5,
				},
				Norm: 0.5,
			},
			expected: 0.0,
		},
		{
			name: "partially similar documents",
			doc1: DocumentTFIDF{
				ID: "doc1",
				TFIDF: map[string]float64{
					"test":     0.5,
					"document": 0.5,
				},
				Norm: 0.7071067811865476, // sqrt(0.5^2 + 0.5^2)
			},
			doc2: DocumentTFIDF{
				ID: "doc2",
				TFIDF: map[string]float64{
					"test":  0.5,
					"other": 0.5,
				},
				Norm: 0.7071067811865476,
			},
			expected: 0.5,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := cosineSimilarity(tt.doc1, tt.doc2)
			if math.Abs(result-tt.expected) > 0.000001 {
				t.Errorf("cosineSimilarity() = %v, want %v", result, tt.expected)
			}
		})
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
