package main

import "testing"

func TestResolveLexicalEngineInvalid(t *testing.T) {
	t.Setenv("RAG_LEXICAL_ENGINE", "")
	_, err := resolveLexicalEngine("invalid")
	if err == nil {
		t.Fatal("expected error for invalid engine")
	}
}

func TestResolveLexicalEngineDefault(t *testing.T) {
	t.Setenv("RAG_LEXICAL_ENGINE", "")
	e, err := resolveLexicalEngine("")
	if err != nil {
		t.Fatal(err)
	}
	if e != "bleve" {
		t.Fatalf("got %q", e)
	}
}
