package main

import "testing"

func TestScoreRetrievalDocHit(t *testing.T) {
	gc := GoldCase{
		ExpectedDocIDs: []string{"doc-a"},
	}
	hits := []retrieveHit{
		{ChunkID: "doc-b-chunk-0", DocID: "doc-b"},
		{ChunkID: "doc-a-chunk-0", DocID: "doc-a", Score: 0.9},
	}
	hit, mrr := scoreRetrieval(gc, hits, 8)
	if !hit || mrr != 0.5 {
		t.Fatalf("got hit=%v mrr=%v", hit, mrr)
	}
}

func TestScoreRetrievalMiss(t *testing.T) {
	gc := GoldCase{ExpectedDocIDs: []string{"doc-x"}}
	hits := []retrieveHit{{DocID: "doc-y", ChunkID: "doc-y-chunk-0"}}
	hit, mrr := scoreRetrieval(gc, hits, 8)
	if hit || mrr != 0 {
		t.Fatalf("got hit=%v mrr=%v", hit, mrr)
	}
}
