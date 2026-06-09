package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/noematic-eu/ai-rag-agent/model"
)

func TestIngestSetup(t *testing.T) {
	// Switch to test mode
	gin.SetMode(gin.TestMode)

	// Create temporary directories for test databases
	tempBleveDir, err := os.MkdirTemp("", "bleve-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.RemoveAll(tempBleveDir) }()

	tempChunkStoreDir, err := os.MkdirTemp("", "f4kvs-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.RemoveAll(tempChunkStoreDir) }()

	// Initialize test databases
	setupTestDatabases(t, tempBleveDir, tempChunkStoreDir)
	defer cleanupTestDatabases()
}

func cleanupTestDatabases() {
	// Cleanup is handled by defer statements in setupTestDatabases
}

func newGinContext(method, target string, body *bytes.Buffer) (*gin.Context, *httptest.ResponseRecorder, error) {
	rr := httptest.NewRecorder()
	var req *http.Request
	var err error
	if body != nil {
		req, err = http.NewRequest(method, target, body)
	} else {
		req, err = http.NewRequest(method, target, nil)
	}
	if err != nil {
		return nil, nil, err
	}
	c, _ := gin.CreateTestContext(rr)
	c.Request = req
	return c, rr, nil
}

func TestIngestDocument(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupTestDatabases(t, t.TempDir(), t.TempDir())

	doc := model.LegalDocument{
		ID:      "test-1",
		Title:   "Test Document",
		Content: "# Test\n\nThis is a test document with enough content to pass the minimum chunk length filter used during ingestion and indexing in the retrieval pipeline.",
	}

	// Create a test HTTP request
	jsonDoc, _ := json.Marshal(doc)
	body := bytes.NewBuffer(jsonDoc)
	c, rr, err := newGinContext("POST", "/ingest", body)
	if err != nil {
		t.Fatal(err)
	}

	// Call the handler
	ingestDocument(c)

	// Check the status code
	if rr.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, rr.Code)
	}
}

func TestNormalizeDocumentContentHTML(t *testing.T) {
	doc := model.LegalDocument{
		ID:          "html-normalize-1",
		Title:       "HTML normalize",
		ContentType: "html",
		Content:     "<html><body><h1>Title</h1><p>Hello <strong>world</strong></p></body></html>",
	}

	normalized, err := normalizeDocumentContent(doc)
	if err != nil {
		t.Fatalf("Failed to normalize HTML content: %v", err)
	}

	if normalized.ContentType != contentTypeHTML {
		t.Fatalf("expected content type %q, got %q", contentTypeHTML, normalized.ContentType)
	}
	if normalized.OriginalContent == "" {
		t.Fatal("expected original HTML content to be preserved")
	}
	if strings.Contains(normalized.Content, "<h1>") {
		t.Fatalf("expected markdown output, got html-like content: %q", normalized.Content)
	}
	if !strings.Contains(normalized.Content, "Title") {
		t.Fatalf("expected converted markdown to contain heading text, got: %q", normalized.Content)
	}
}

func TestIngestHTMLDocument(t *testing.T) {
	tempBleveDir, err := os.MkdirTemp("", "bleve-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.RemoveAll(tempBleveDir) }()

	tempChunkStoreDir, err := os.MkdirTemp("", "f4kvs-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.RemoveAll(tempChunkStoreDir) }()

	setupTestDatabases(t, tempBleveDir, tempChunkStoreDir)
	defer cleanupTestDatabases()

	doc := model.LegalDocument{
		ID:          "html-ingest-1",
		Title:       "HTML ingestion",
		ContentType: "html",
		Content:     `<html><body><h1>Main title</h1><p>First paragraph with enough text to produce a useful chunk after HTML normalization and chunk quality filtering during ingestion.</p><ul><li>Item A with additional detail about retrieval pipelines</li></ul></body></html>`,
	}

	jsonDoc, _ := json.Marshal(doc)
	body := bytes.NewBuffer(jsonDoc)
	c, rr, err := newGinContext("POST", "/ingest", body)
	if err != nil {
		t.Fatal(err)
	}

	ingestDocument(c)
	if rr.Code != http.StatusOK {
		t.Fatalf("Expected status %d, got %d", http.StatusOK, rr.Code)
	}

	stored := firstStoredChunk(t)
	if stored.Metadata.Source != contentTypeHTML {
		t.Fatalf("expected source %q, got %q", contentTypeHTML, stored.Metadata.Source)
	}
	if stored.Original != "" {
		t.Fatalf("expected chunk to omit duplicated original HTML, got: %q", stored.Original)
	}
	docStored := loadStoredDocument(t, doc.ID)
	if !strings.Contains(docStored.OriginalContent, "<h1>Main title</h1>") {
		t.Fatalf("expected original HTML on doc record, got OriginalContent: %q", docStored.OriginalContent)
	}
	if strings.Contains(stored.Text, "<p>") || strings.Contains(stored.Text, "<h1>") {
		t.Fatalf("expected plain text in stored chunk, got: %q", stored.Text)
	}
}

func TestStoredChunkPlainText(t *testing.T) {
	tempBleveDir, err := os.MkdirTemp("", "bleve-plain-*")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.RemoveAll(tempBleveDir) }()

	tempChunkStoreDir, err := os.MkdirTemp("", "f4kvs-plain-*")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.RemoveAll(tempChunkStoreDir) }()

	setupTestDatabases(t, tempBleveDir, tempChunkStoreDir)

	doc := model.LegalDocument{
		ID:      "plain-text-1",
		Title:   "Plain",
		Content: "# Section\n\n<p>Hello <strong>world</strong></p>\n\nThis paragraph adds enough characters to satisfy the minimum chunk length requirement for indexing.",
	}

	if _, err := indexDocument(doc); err != nil {
		t.Fatalf("indexDocument failed: %v", err)
	}

	stored := firstStoredChunk(t)
	if strings.Contains(stored.Text, "<") {
		t.Fatalf("expected no HTML in stored chunk text, got: %q", stored.Text)
	}
	if !strings.Contains(stored.Text, "paragraph") {
		t.Fatalf("expected readable text in chunk, got: %q", stored.Text)
	}
	docStored := loadStoredDocument(t, doc.ID)
	if docStored.Content == "" {
		t.Fatalf("expected document record to be stored at ingest")
	}
}

func TestIngestInvalidDocument(t *testing.T) {
	// Create an invalid HTTP request
	c, rr, err := newGinContext("POST", "/ingest", nil)
	if err != nil {
		t.Fatal(err)
	}

	// Call the handler
	ingestDocument(c)

	// Check the status code
	if rr.Code != http.StatusBadRequest {
		t.Errorf("Expected status %d, got %d", http.StatusBadRequest, rr.Code)
	}
}

func TestIngestEmptyContent(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupTestDatabases(t, t.TempDir(), t.TempDir())

	doc := model.LegalDocument{
		ID:      "test-2",
		Title:   "Empty Document",
		Content: "",
	}

	// Create a test HTTP request
	jsonDoc, _ := json.Marshal(doc)
	body := bytes.NewBuffer(jsonDoc)
	c, rr, err := newGinContext("POST", "/ingest", body)
	if err != nil {
		t.Fatal(err)
	}

	// Call the handler
	ingestDocument(c)

	// Check the status code (should still be OK, just with empty chunks)
	if rr.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, rr.Code)
	}
}

func TestIngestCorpusField(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupTestDatabases(t, t.TempDir(), t.TempDir())

	doc := model.LegalDocument{
		ID:      "corpus-doc-1",
		Title:   "Memoir",
		Corpus:  "memoirs",
		Content: "# Jean\n\n## Early Life\n\n### Where were you born?\n\nParis in 1948 with enough detail to pass the minimum chunk length filter used during ingestion.",
	}
	if _, err := indexDocument(doc); err != nil {
		t.Fatalf("indexDocument failed: %v", err)
	}

	stored := firstStoredChunk(t)
	if stored.Metadata.Corpus != "memoirs" {
		t.Fatalf("expected corpus memoirs on chunk, got %q", stored.Metadata.Corpus)
	}
}

func TestDeleteDocument(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupTestDatabases(t, t.TempDir(), t.TempDir())

	doc := model.LegalDocument{
		ID:      "delete-doc-1",
		Title:   "To Delete",
		Corpus:  "memoirs",
		Content: "# Delete me\n\nParagraph with enough content to produce at least one indexable chunk after chunking and quality filtering in the test harness.",
	}
	if _, err := indexDocument(doc); err != nil {
		t.Fatalf("indexDocument failed: %v", err)
	}

	c, rr, err := newGinContext("DELETE", "/documents/delete-doc-1", nil)
	if err != nil {
		t.Fatal(err)
	}
	c.Params = gin.Params{{Key: "doc_id", Value: "delete-doc-1"}}
	deleteDocument(c)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rr.Code, rr.Body.String())
	}

	pairs, err := chunkStore.ScanPrefix("chunk:")
	if err != nil {
		t.Fatal(err)
	}
	for _, pair := range pairs {
		var chunk model.Chunk
		if err := json.Unmarshal(pair.Value, &chunk); err != nil {
			continue
		}
		if chunk.Metadata.DocID == "delete-doc-1" {
			t.Fatalf("chunk still present after delete: %s", chunk.Metadata.ChunkID)
		}
	}

	c2, rr2, err := newGinContext("DELETE", "/documents/missing-doc", nil)
	if err != nil {
		t.Fatal(err)
	}
	c2.Params = gin.Params{{Key: "doc_id", Value: "missing-doc"}}
	deleteDocument(c2)
	if rr2.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for missing doc, got %d", rr2.Code)
	}
}

func TestSearchDocuments(t *testing.T) {
	// Setup test databases
	tempBleveDir, err := os.MkdirTemp("", "bleve-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.RemoveAll(tempBleveDir) }()

	tempChunkStoreDir, err := os.MkdirTemp("", "f4kvs-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.RemoveAll(tempChunkStoreDir) }()

	setupTestDatabases(t, tempBleveDir, tempChunkStoreDir)
	defer cleanupTestDatabases()

	// Create a test document
	doc := model.LegalDocument{
		ID:      "test-3",
		Title:   "Search Test Document",
		Content: "# Search Test\n\nThis document is for testing search functionality.",
	}

	// Ingest the document
	_, err = indexDocument(doc)
	if err != nil {
		t.Fatalf("Failed to index document: %v", err)
	}

	// Create a test HTTP request for search
	c, rr, err := newGinContext("GET", "/search?q=test", nil)
	if err != nil {
		t.Fatal(err)
	}

	// Call the handler
	searchDocuments(c)

	// Check the status code
	if rr.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, rr.Code)
	}
}

func TestSearchEmptyQuery(t *testing.T) {
	// Create a test HTTP request for search with empty query
	c, rr, err := newGinContext("GET", "/search?q=", nil)
	if err != nil {
		t.Fatal(err)
	}

	// Call the handler
	searchDocuments(c)

	// Check the status code
	if rr.Code != http.StatusBadRequest {
		t.Errorf("Expected status %d, got %d", http.StatusBadRequest, rr.Code)
	}
}

func TestSearchNoResults(t *testing.T) {
	// Setup test databases
	tempBleveDir, err := os.MkdirTemp("", "bleve-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.RemoveAll(tempBleveDir) }()

	tempChunkStoreDir, err := os.MkdirTemp("", "f4kvs-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.RemoveAll(tempChunkStoreDir) }()

	setupTestDatabases(t, tempBleveDir, tempChunkStoreDir)
	defer cleanupTestDatabases()

	// Create a test HTTP request for search with no matching results
	c, rr, err := newGinContext("GET", "/search?q=nonexistentquery12345", nil)
	if err != nil {
		t.Fatal(err)
	}

	// Call the handler
	searchDocuments(c)

	// Check the status code (should still be OK, just with empty results)
	if rr.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, rr.Code)
	}
}

func TestChunkDocument(t *testing.T) {
	// Create a test document
	doc := model.LegalDocument{
		ID:      "chunk-test-1",
		Title:   "Chunk Test Document",
		Content: "# Title 1\n\n## Section 1\n\nThis is some content.\n\n## Section 2\n\nMore content here.",
	}

	// Chunk the document
	chunks, err := ChunkDocument(doc, DefaultChunkConfig())
	if err != nil {
		t.Fatalf("Failed to chunk document: %v", err)
	}

	// Check that we got chunks
	if len(chunks) == 0 {
		t.Error("Expected at least one chunk")
	}

	// Check that each chunk has metadata
	for i, chunk := range chunks {
		if chunk.Metadata.ChunkID == "" {
			t.Errorf("Chunk %d has empty ChunkID", i)
		}
		if chunk.Metadata.DocID != doc.ID {
			t.Errorf("Chunk %d has wrong DocID: expected %s, got %s", i, doc.ID, chunk.Metadata.DocID)
		}
		if chunk.Text == "" {
			t.Errorf("Chunk %d has empty Text", i)
		}
	}
}

func TestChunkHTMLDocument(t *testing.T) {
	// Create a test HTML document
	html := `
<!DOCTYPE html>
<html>
<head><title>HTML Test</title></head>
<body>
<h1>Main Title</h1>
<p>This is a paragraph.</p>
<ul>
<li>Item 1</li>
<li>Item 2</li>
</ul>
<pre><code>function test() { return true; }</code></pre>
</body>
</html>
`

	// Chunk the HTML document
	chunks, err := ChunkHTMLDocument(html, "html-test-1")
	if err != nil {
		t.Fatalf("Failed to chunk HTML document: %v", err)
	}

	// Check that we got chunks
	if len(chunks) == 0 {
		t.Error("Expected at least one chunk")
	}

	// Check that each chunk has metadata
	for i, chunk := range chunks {
		if chunk.Metadata.ChunkID == "" {
			t.Errorf("Chunk %d has empty ChunkID", i)
		}
		if chunk.Metadata.DocID != "html-test-1" {
			t.Errorf("Chunk %d has wrong DocID", i)
		}
		if chunk.Text == "" {
			t.Errorf("Chunk %d has empty Text", i)
		}
	}
}

func TestCountTokens(t *testing.T) {
	tests := []struct {
		input    string
		expected int
	}{
		{"hello world", 2},
		{"one two three four five", 5},
		{"", 0},
		{"a b c d e f g h i j", 10},
	}

	for _, test := range tests {
		result := countTokens(test.input)
		if result != test.expected {
			t.Errorf("countTokens(%q) = %d, want %d", test.input, result, test.expected)
		}
	}
}

func TestDefaultChunkConfig(t *testing.T) {
	config := DefaultChunkConfig()

	if config.MaxTokens == 0 {
		t.Error("DefaultChunkConfig should have MaxTokens > 0")
	}
	if config.OverlapTokens == 0 {
		t.Error("DefaultChunkConfig should have OverlapTokens > 0")
	}
	if config.MinChunkSize == 0 {
		t.Error("DefaultChunkConfig should have MinChunkSize > 0")
	}
}

func TestBuildSectionPath(t *testing.T) {
	tests := []struct {
		title    string
		position int
	}{
		{"Article 1", 0},
		{"Titre I", 1},
		{"Chapitre A", 2},
	}

	for _, test := range tests {
		path := buildSectionPath(test.title, test.position)
		if path == "" {
			t.Errorf("buildSectionPath(%q, %d) returned empty string", test.title, test.position)
		}
	}
}

func TestSplitByTokenCount(t *testing.T) {
	text := "word1 word2 word3 word4 word5 word6 word7 word8 word9 word10"
	config := ChunkConfig{
		MaxTokens:     5,
		OverlapTokens: 2,
	}

	chunks := splitByTokenCount(text, config)

	if len(chunks) == 0 {
		t.Error("Expected at least one chunk")
	}

	// Check that chunks don't exceed max tokens (approximately)
	for i, chunk := range chunks {
		tokenCount := countTokens(chunk)
		if tokenCount > config.MaxTokens+1 { // Allow some flexibility
			t.Errorf("Chunk %d has %d tokens, exceeds max %d", i, tokenCount, config.MaxTokens)
		}
	}
}

func TestStoreChunkMetadata(t *testing.T) {
	// Setup test databases
	tempBleveDir, err := os.MkdirTemp("", "bleve-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.RemoveAll(tempBleveDir) }()

	tempChunkStoreDir, err := os.MkdirTemp("", "f4kvs-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.RemoveAll(tempChunkStoreDir) }()

	setupTestDatabases(t, tempBleveDir, tempChunkStoreDir)
	defer cleanupTestDatabases()

	// Create a test chunk
	chunk := model.Chunk{
		Metadata: model.ChunkMetadata{
			DocID:       "test-doc",
			ChunkID:     "test-chunk-1",
			Title:       "Test Chunk",
			SectionPath: "Section 1",
			Source:      "markdown",
			Position:    0,
		},
		Text:     "This is test content.",
		Original: "# Test\n\nTest content.",
	}

	// Store the chunk metadata
	storeChunkMetadata(chunk)

	// Retrieve and verify
	retrievedChunk := loadStoredChunkByID(t, chunk.Metadata.ChunkID)
	if retrievedChunk.Metadata.ChunkID != chunk.Metadata.ChunkID {
		t.Errorf("Retrieved chunk ID mismatch: expected %s, got %s", chunk.Metadata.ChunkID, retrievedChunk.Metadata.ChunkID)
	}
	if retrievedChunk.Text != chunk.Text {
		t.Errorf("Retrieved chunk text mismatch")
	}
	if retrievedChunk.Original != "" {
		t.Errorf("expected stored chunk to omit Original, got %q", retrievedChunk.Original)
	}
}

func TestIngestParseMarkdown(t *testing.T) {
	md := "# Title\n\nThis is a paragraph.\n\n## Section\n\nMore content."
	text := parseMarkdown(md)

	if text == "" {
		t.Error("Expected non-empty text from parseMarkdown")
	}
}

func TestParseHTMLToChunks(t *testing.T) {
	html := `<html><body><h1>Title</h1><p>Content</p></body></html>`
	chunks, err := parseHTMLToChunks(html, "test-html")

	if err != nil {
		t.Fatalf("Failed to parse HTML: %v", err)
	}

	if len(chunks) == 0 {
		t.Error("Expected at least one chunk")
	}
}

func TestHTMLToMarkdownEdgeCases(t *testing.T) {
	html := `<html><body>
<h1>Heading</h1>
<p>See <a href="https://example.com">example</a>.</p>
<ul><li>First</li><li>Second</li></ul>
<table><tr><th>Col</th></tr><tr><td>Val</td></tr></table>
</body></html>`

	md, err := htmlToMarkdown(html)
	if err != nil {
		t.Fatalf("Failed to convert html to markdown: %v", err)
	}

	expectedFragments := []string{"Heading", "example", "First", "Second", "Col", "Val"}
	for _, fragment := range expectedFragments {
		if !strings.Contains(md, fragment) {
			t.Fatalf("expected markdown to contain %q, got: %q", fragment, md)
		}
	}
}

func TestSplitByHeadings(t *testing.T) {
	md := `# Title 1
Content here.

## Section 1
More content.

## Section 2
Even more content.
`
	sections := splitByHeadings(md)

	if len(sections) == 0 {
		t.Error("Expected at least one section")
	}
}

func TestExtractHeadings(t *testing.T) {
	md := `# Title 1
## Section 1
### Subsection 1.1
`
	headings := extractHeadings(md)

	if len(headings) == 0 {
		t.Error("Expected at least one heading")
	}
}

func TestExtractTextFromNode(t *testing.T) {
	// This test requires a proper AST node, so we'll skip it for now
}

func TestGetHeadingPrefix(t *testing.T) {
	tests := []struct {
		level    int
		expected string
	}{
		{1, "Titre"},
		{2, "Chapitre"},
		{3, "Section"},
		{4, "Paragraphe"},
		{5, "Article-5"}, // Default case
	}

	for _, test := range tests {
		result := getHeadingPrefix(test.level)
		if result != test.expected {
			t.Errorf("getHeadingPrefix(%d) = %s, want %s", test.level, result, test.expected)
		}
	}
}

func TestBuildHeadingPath(t *testing.T) {
	// This test requires a goquery.Selection, so we'll skip it for now
}

func TestChunkDocumentNoDuplicatedOriginal(t *testing.T) {
	var content strings.Builder
	for i := 0; i < 1300; i++ {
		content.WriteString("word ")
	}

	doc := model.LegalDocument{
		ID:      "large-no-dup",
		Title:   "Large Plain Document",
		Content: content.String(),
	}

	chunks, err := ChunkDocument(doc, DefaultChunkConfig())
	if err != nil {
		t.Fatalf("ChunkDocument failed: %v", err)
	}
	if len(chunks) < 2 {
		t.Fatalf("expected multiple token chunks, got %d", len(chunks))
	}

	var originalBytes int
	for _, chunk := range chunks {
		originalBytes += len(chunk.Original)
		if len(chunk.Original) > len(chunk.Text) {
			t.Fatalf("chunk %s Original longer than Text", chunk.Metadata.ChunkID)
		}
	}
	if originalBytes > 0 {
		t.Fatalf("expected no Original payloads on chunks, got %d bytes total", originalBytes)
	}
}

func TestChunkDocumentOversizedFirstSection(t *testing.T) {
	doc := model.LegalDocument{
		ID:      "mixed-sections",
		Title:   "Mixed",
		Content: strings.Repeat("preamble ", 8000) + "\n\n# Small\n\nShort section body with enough words to pass filters.",
	}

	chunks, err := ChunkDocument(doc, DefaultChunkConfig())
	if err != nil {
		t.Fatalf("ChunkDocument failed: %v", err)
	}
	if len(chunks) < 2 {
		t.Fatalf("expected preamble split plus small section chunk, got %d chunks", len(chunks))
	}
}

func TestChunkDocumentFallback(t *testing.T) {
	// Create a document without clear headings
	doc := model.LegalDocument{
		ID:      "fallback-test",
		Title:   "Fallback Test Document",
		Content: "This is a document without clear headings. It has multiple paragraphs that should be chunked based on token count.",
	}

	chunks, err := ChunkDocument(doc, DefaultChunkConfig())
	if err != nil {
		t.Fatalf("Failed to chunk document: %v", err)
	}

	if len(chunks) == 0 {
		t.Error("Expected at least one chunk")
	}
}

func TestChunkDocumentWithLargeContent(t *testing.T) {
	// Create a large document
	var content strings.Builder
	for i := 0; i < 100; i++ {
		_, _ = fmt.Fprintf(&content, "# Section %d\n\nThis is paragraph %d with some content to make it longer.\n", i, i)
	}

	doc := model.LegalDocument{
		ID:      "large-test",
		Title:   "Large Document Test",
		Content: content.String(),
	}

	chunks, err := ChunkDocument(doc, DefaultChunkConfig())
	if err != nil {
		t.Fatalf("Failed to chunk large document: %v", err)
	}

	if len(chunks) == 0 {
		t.Error("Expected at least one chunk")
	}

	// Check that chunks are properly sized
	for i, chunk := range chunks {
		tokenCount := countTokens(chunk.Text)
		if tokenCount > 1000 { // Allow some flexibility
			t.Logf("Chunk %d has %d tokens (may exceed max due to no clear headings)", i, tokenCount)
		}
	}
}

func TestStoreChunkMetadataError(t *testing.T) {
	// Setup test databases
	tempBleveDir, err := os.MkdirTemp("", "bleve-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.RemoveAll(tempBleveDir) }()

	tempChunkStoreDir, err := os.MkdirTemp("", "f4kvs-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.RemoveAll(tempChunkStoreDir) }()

	setupTestDatabases(t, tempBleveDir, tempChunkStoreDir)
	defer cleanupTestDatabases()

	// Create a chunk with invalid metadata (to test error handling)
	chunk := model.Chunk{
		Metadata: model.ChunkMetadata{},
		Text:     "Test content",
	}

	// This should not panic
	storeChunkMetadata(chunk)
}
