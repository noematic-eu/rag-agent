package main

import (
	"testing"

	"github.com/noematic-eu/ai-rag-agent/model"
)

func TestRetrieveLexicalScanFallbackWhenIndexEmpty(t *testing.T) {
	setupTestDiskLexical(t)

	doc := model.LegalDocument{
		ID:      "scan-fallback-doc",
		Title:   "Revolution",
		Content: "The French Revolution of 1789 had deep social and economic causes including estates and taxation.",
		Corpus:  "test-corpus",
	}
	chunks, err := ChunkDocument(doc, DefaultChunkConfig())
	if err != nil {
		t.Fatal(err)
	}
	if len(chunks) == 0 {
		t.Fatal("expected chunks")
	}
	chunks[0].Metadata.Corpus = doc.Corpus
	storeChunkMetadata(chunks[0])

	hits, source, err := retrieveLexicalHits(
		"French Revolution causes",
		"test-corpus",
		5,
		parseLexicalRetrievalFromString("auto", 0),
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) == 0 {
		t.Fatal("expected scan fallback hits with empty lex index")
	}
	if source != lexicalSourceScan {
		t.Fatalf("expected source scan, got %q", source)
	}
}

func TestRetrieveLexicalUsesIndexWhenBuilt(t *testing.T) {
	setupTestDiskLexical(t)

	doc := model.LegalDocument{
		ID:      "index-source-doc",
		Title:   "Indexed",
		Content: "Unique indexed lexical term zephyr winds for source detection testing with enough text to pass chunk size filters and produce indexable content.",
		Corpus:  "test-corpus",
	}
	if _, err := indexDocument(doc); err != nil {
		t.Fatal(err)
	}
	if _, err := rebuildLexicalFromChunkStore(nil, false); err != nil {
		t.Fatal(err)
	}

	hits, source, err := retrieveLexicalHits(
		"zephyr winds",
		"test-corpus",
		5,
		parseLexicalRetrievalFromString("auto", 0),
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) == 0 {
		t.Fatal("expected index hits")
	}
	if source != lexicalSourceIndex {
		t.Fatalf("expected source index, got %q", source)
	}
}

func TestRankChunksScanFallbackNoRebuild(t *testing.T) {
	setupTestDiskLexical(t)

	doc := model.LegalDocument{
		ID:      "rank-scan-doc",
		Title:   "Rank",
		Content: "Brutal scan fallback rank test with French Revolution taxation estates content and enough words to satisfy minimum chunk size requirements for ingestion.",
		Corpus:  "test-corpus",
	}
	chunks, err := ChunkDocument(doc, DefaultChunkConfig())
	if err != nil {
		t.Fatal(err)
	}
	if len(chunks) == 0 {
		t.Fatal("expected chunks")
	}
	chunks[0].Metadata.Corpus = doc.Corpus
	storeChunkMetadata(chunks[0])

	outcome, err := rankChunks(rankParams{
		retrievalText:    "French Revolution taxation",
		topKBM25:         5,
		topKVector:       0,
		topKFinal:        3,
		minScore:         0.01,
		corpus:           "test-corpus",
		maxPerDoc:        3,
		fusionMode:       "weighted",
		fusionWeight:     1.0,
		lexicalRetrieval: parseLexicalRetrievalFromString("auto", 0),
	})
	if err != nil {
		t.Fatal(err)
	}
	if outcome.noResults || len(outcome.hits) == 0 {
		t.Fatalf("expected hits via scan fallback, got %+v", outcome)
	}
	if outcome.lexicalSource != lexicalSourceScan {
		t.Fatalf("expected lexical source scan, got %q", outcome.lexicalSource)
	}

	pairs, err := chunkStore.ScanPrefix("lex:")
	if err != nil {
		t.Fatal(err)
	}
	if len(pairs) != 0 {
		t.Fatalf("rank must not trigger lex rebuild, found %d lex keys", len(pairs))
	}
}

func TestResolveLexicalChain(t *testing.T) {
	if chain := resolveLexicalChain("auto"); len(chain) != 2 || chain[0] != "index" {
		t.Fatalf("auto chain: %+v", chain)
	}
	if chain := resolveLexicalChain("scan"); len(chain) != 1 || chain[0] != "scan" {
		t.Fatalf("scan chain: %+v", chain)
	}
	if chain := resolveLexicalChain("index,scan"); len(chain) != 2 {
		t.Fatalf("index,scan chain: %+v", chain)
	}
}
