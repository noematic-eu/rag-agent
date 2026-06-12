package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/noematic-eu/ai-rag-agent/model"
)

func TestURLFetchAllowed_blocksPrivateHosts(t *testing.T) {
	if urlFetchAllowed("https://127.0.0.1/page", nil) {
		t.Fatal("expected localhost to be blocked")
	}
	if urlFetchAllowed("http://example.com/page", nil) {
		t.Fatal("expected non-https to be blocked")
	}
	if !urlFetchAllowed("https://en.wikipedia.org/wiki/Test", nil) {
		t.Fatal("expected public https host to be allowed")
	}
}

func TestWebQueriesForSupplement_prioritizesFollowUp(t *testing.T) {
	got := webQueriesForSupplement("main query", []string{"follow 1", "follow 2", "follow 3"}, 2)
	if len(got) != 2 || got[0] != "follow 1" || got[1] != "follow 2" {
		t.Fatalf("unexpected queries: %#v", got)
	}
}

func TestMergeWebDocs_dedupesByID(t *testing.T) {
	existing := []model.LegalDocument{{ID: "kb::1", Title: "KB"}}
	web := []model.LegalDocument{
		{ID: "web::wikipedia::1", Title: "Wiki", Corpus: "web"},
		{ID: "web::wikipedia::1", Title: "Wiki dup", Corpus: "web"},
	}
	merged := mergeWebDocs(existing, web, 3)
	if len(merged) != 2 {
		t.Fatalf("expected 2 docs, got %d", len(merged))
	}
}

func TestSearchWikipedia_mockServer(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Query().Get("list") {
		case "search":
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"query": map[string]interface{}{
					"search": []map[string]interface{}{
						{"title": "French Revolution", "pageid": 111, "snippet": "revolution"},
					},
				},
			})
		default:
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"query": map[string]interface{}{
					"pages": map[string]interface{}{
						"111": map[string]interface{}{
							"pageid":  111,
							"title":   "French Revolution",
							"extract": "The French Revolution began in 1789.",
						},
					},
				},
			})
		}
	}))
	defer srv.Close()

	oldBase := wikiAPIBaseOverride
	wikiAPIBaseOverride = srv.URL
	defer func() { wikiAPIBaseOverride = oldBase }()

	docs, err := searchWikipedia(context.Background(), "en", "French Revolution", 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(docs) != 1 {
		t.Fatalf("expected 1 doc, got %d", len(docs))
	}
	if docs[0].Corpus != "web" || !strings.Contains(docs[0].BookTitle, "wikipedia.org") {
		t.Fatalf("unexpected doc: %+v", docs[0])
	}
}

func TestRunWebSupplement_wikipediaThenTavily(t *testing.T) {
	wikiSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"query": map[string]interface{}{"search": []interface{}{}}})
	}))
	defer wikiSrv.Close()

	tavilySrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"results": []map[string]interface{}{
				{
					"title":       "French Revolution",
					"url":         "https://en.wikipedia.org/wiki/French_Revolution",
					"raw_content": "Causes and consequences of the revolution in France.",
				},
			},
		})
	}))
	defer tavilySrv.Close()

	oldWiki := wikiAPIBaseOverride
	wikiAPIBaseOverride = wikiSrv.URL
	defer func() { wikiAPIBaseOverride = oldWiki }()

	oldTavily := tavilyAPIURL
	tavilyAPIURL = tavilySrv.URL
	defer func() { tavilyAPIURL = oldTavily }()

	t.Setenv("RAG_WEB_SUPPLEMENT", "1")
	t.Setenv("RAG_WEB_SEARCH_API_KEY", "test-key")
	t.Setenv("RAG_WEB_SEARCH_PROVIDER", "tavily")

	outcome := runWebSupplement(context.Background(), "French Revolution causes", "en", nil)
	if len(outcome.docs) == 0 {
		t.Fatal("expected tavily fallback docs")
	}
	if outcome.provider != "tavily" {
		t.Fatalf("expected tavily provider, got %q", outcome.provider)
	}
}

func TestMaybeApplyWebSupplement_skippedWhenDisabled(t *testing.T) {
	t.Setenv("RAG_WEB_SUPPLEMENT", "0")
	meta := map[string]string{}
	docs := maybeApplyWebSupplement(context.Background(), "q", "en", webGapNoResults, nil, nil, meta, nil)
	if len(docs) != 0 {
		t.Fatal("expected no docs when disabled")
	}
	if meta["web_supplement"] != "" {
		t.Fatalf("unexpected meta: %#v", meta)
	}
}

func TestDetectWebGapAfterCRAG(t *testing.T) {
	reason, need := detectWebGapAfterCRAG(cragTrace{Sufficient: false}, true)
	if !need || reason != webGapCRAGInsufficient {
		t.Fatalf("expected crag insufficient, got %q need=%v", reason, need)
	}
	_, need = detectWebGapAfterCRAG(cragTrace{Sufficient: true}, true)
	if need {
		t.Fatal("expected no gap when sufficient")
	}
}

func TestDetectWebGapAfterAgent_emptyDocs(t *testing.T) {
	reason, need := detectWebGapAfterAgent(context.Background(), "q", "en", nil, 8)
	if !need || reason != webGapAgentInsufficient {
		t.Fatalf("expected agent insufficient, got %q need=%v", reason, need)
	}
}

func TestWebSupplementConfigFromEnv(t *testing.T) {
	t.Setenv("RAG_WEB_SUPPLEMENT", "1")
	t.Setenv("RAG_WEB_MAX_PAGES", "5")
	cfg := webSupplementConfigFromEnv()
	if !cfg.enabled || cfg.maxPages != 5 {
		t.Fatalf("unexpected cfg: %+v", cfg)
	}
}
