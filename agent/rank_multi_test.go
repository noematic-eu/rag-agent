package main

import "testing"

func TestFuseMultiQueryRRF(t *testing.T) {
	lists := []map[string]int{
		{"a": 1, "b": 2},
		{"b": 1, "c": 2},
	}
	out := fuseMultiQueryRRF(lists)
	if len(out) != 3 {
		t.Fatalf("expected 3 fused hits, got %d", len(out))
	}
	if out[0].ID != "b" {
		t.Fatalf("expected b first after RRF fusion, got %+v", out)
	}
}

func TestDedupeQueries(t *testing.T) {
	out := dedupeQueries([]string{"a", "A", "b", "a"})
	if len(out) != 2 {
		t.Fatalf("expected 2 unique queries, got %v", out)
	}
}
