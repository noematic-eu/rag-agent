package main

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/noematic-eu/ai-rag-agent/internal/f4kvs"
	"github.com/noematic-eu/ai-rag-agent/lexical"
	"github.com/noematic-eu/ai-rag-agent/model"
)

func setupTestDiskLexical(t *testing.T) {
	t.Helper()

	chunkDir := mustTempChunkStoreDir(t, "f4kvs-disk-test-*")
	store, err := f4kvs.Open(chunkDir)
	if err != nil {
		t.Fatal(err)
	}
	chunkStore = &f4kvsChunkStore{store: store}

	storeCfg = agentConfig{
		DataDir:          chunkDir,
		LexicalEngine:    lexical.EngineF4KVS,
		F4KVSLexicalMode: lexical.F4KVSLexicalModeDisk,
	}

	lexCfg := lexical.Config{
		DataDir:          chunkDir,
		Engine:           lexical.EngineF4KVS,
		F4KVSLexicalMode: lexical.F4KVSLexicalModeDisk,
		KV:               newLexicalKVAdapter(chunkStore),
		ScanChunks:       scanChunksFromStore,
	}
	b, err := lexical.Open(lexCfg)
	if err != nil {
		t.Fatal(err)
	}
	lexicalBackend = b

	t.Cleanup(func() {
		_ = closeLexicalBackend()
		chunkStore = nil
	})
}

func TestIndexDocumentIndexesDiskLexicalAtIngest(t *testing.T) {
	setupTestDiskLexical(t)

	doc := model.LegalDocument{
		ID:      "defer-lex-doc",
		Title:   "Defer Lex",
		Content: "This document has enough content to produce indexable chunks for testing disk lexical indexing during ingest.",
		Corpus:  "test-corpus",
	}
	n, err := indexDocument(doc)
	if err != nil {
		t.Fatalf("indexDocument: %v", err)
	}
	if n == 0 {
		t.Fatal("expected chunks indexed")
	}

	pairs, err := chunkStore.ScanPrefix("lex:")
	if err != nil {
		t.Fatal(err)
	}
	if len(pairs) == 0 {
		t.Fatal("expected lex:* keys after ingest")
	}

	result, err := rebuildLexicalFromChunkStore(nil, false)
	if err != nil {
		t.Fatalf("incremental finalize: %v", err)
	}
	if result.ChunksIndexed != 0 {
		t.Fatalf("expected incremental finalize to skip already-indexed chunks, got %+v", result)
	}
	if result.ChunksSkipped == 0 {
		t.Fatalf("expected skipped chunks, got %+v", result)
	}

	hits, err := lexicalBackend.Search("disk lexical indexing", "test-corpus", 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) == 0 {
		t.Fatal("expected lexical hits after finalize")
	}
}

func TestMaybeRebuildLexicalIfStaleDisk(t *testing.T) {
	setupTestDiskLexical(t)

	statsState.mu.Lock()
	statsState.ingest.ChunksTotal = 200
	statsState.mu.Unlock()

	maybeRebuildLexicalIfStale()

	indexed := lexical.F4KVSIndexedChunkCount(lexicalBackend)
	if indexed > 0 {
		t.Fatalf("expected no rebuild without chunks, indexed=%d", indexed)
	}

	doc := model.LegalDocument{
		ID:      "stale-lex-doc",
		Title:   "Stale",
		Content: "Content for stale lexical rebuild test with sufficient length to create chunks in the chunk store for disk mode.",
		Corpus:  "test-corpus",
	}
	if _, err := indexDocument(doc); err != nil {
		t.Fatal(err)
	}

	statsState.mu.Lock()
	statsState.ingest.ChunksTotal = 200
	statsState.mu.Unlock()

	maybeRebuildLexicalIfStale()

	indexed = lexical.F4KVSIndexedChunkCount(lexicalBackend)
	if indexed == 0 {
		t.Fatal("expected stale rebuild to index chunks")
	}
}

func TestFinalizeStreamReturnsProgress(t *testing.T) {
	setupTestDiskLexical(t)

	doc := model.LegalDocument{
		ID:      "stream-finalize-doc",
		Title:   "Stream",
		Content: "Document for streaming finalize progress with enough text to index.",
		Corpus:  "test-corpus",
	}
	if _, err := indexDocument(doc); err != nil {
		t.Fatal(err)
	}

	c, rr, err := newGinContext("POST", "/finalize?stream=1", nil)
	if err != nil {
		t.Fatal(err)
	}
	c.Request.Header.Set("Accept", "text/event-stream")
	finalize(c)

	if rr.Code != http.StatusOK {
		t.Fatalf("status %d body %s", rr.Code, rr.Body.String())
	}
	body := rr.Body.String()
	if !strings.Contains(body, "event:progress") {
		t.Fatalf("expected progress events, got %q", body)
	}
	if !strings.Contains(body, "event:complete") {
		t.Fatalf("expected complete event, got %q", body)
	}
}

func TestFinalizeStreamAttachToRunningRebuild(t *testing.T) {
	setupTestDiskLexical(t)

	beginLexicalRebuild(100)
	updateLexicalRebuild(42, 100)
	go func() {
		time.Sleep(50 * time.Millisecond)
		finishLexicalRebuild(100, 100, time.Second)
	}()

	c, rr, err := newGinContext("POST", "/finalize?stream=1", nil)
	if err != nil {
		t.Fatal(err)
	}
	c.Request.Header.Set("Accept", "text/event-stream")
	finalize(c)

	body := rr.Body.String()
	if !strings.Contains(body, "event:status") {
		t.Fatalf("expected status event, got %q", body)
	}
	if !strings.Contains(body, "event:progress") {
		t.Fatalf("expected progress event, got %q", body)
	}
	if !strings.Contains(body, "event:complete") {
		t.Fatalf("expected complete event, got %q", body)
	}
}

func TestFinalizeReturnsAccepted(t *testing.T) {
	setupTestDatabases(t, t.TempDir(), t.TempDir())

	doc := model.LegalDocument{
		ID:      "finalize-doc-1",
		Title:   "Test",
		Content: "Sample content for finalize handler test with enough text to pass chunk filters.",
		Corpus:  "eval-public",
	}
	if _, err := indexDocument(doc); err != nil {
		t.Fatalf("indexDocument: %v", err)
	}

	c, rr, err := newGinContext("POST", "/finalize", nil)
	if err != nil {
		t.Fatal(err)
	}
	finalize(c)

	if rr.Code != http.StatusAccepted {
		t.Fatalf("status %d body %s", rr.Code, rr.Body.String())
	}

	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if !currentLexicalIndexStats().Rebuilding {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if currentLexicalIndexStats().Rebuilding {
		t.Fatal("expected background finalize to complete")
	}
}
