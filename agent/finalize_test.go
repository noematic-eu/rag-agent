package main

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/noematic-eu/ai-rag-agent/model"
)

func TestFinalizeReturnsJSON(t *testing.T) {
	setupTestDatabases(t, t.TempDir(), t.TempDir())

	doc := model.LegalDocument{
		ID:      "finalize-doc-1",
		Title:   "Test",
		Content: "Sample content for finalize handler test.",
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

	if rr.Code != http.StatusOK {
		t.Fatalf("status %d body %s", rr.Code, rr.Body.String())
	}

	var resp map[string]string
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v body %s", err, rr.Body.String())
	}
	if resp["status"] != "finalized" {
		t.Fatalf("expected status finalized, got %+v", resp)
	}
}
