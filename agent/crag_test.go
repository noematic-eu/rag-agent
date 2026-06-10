package main

import (
	"testing"

	"github.com/noematic-eu/ai-rag-agent/model"
)

func TestParseCragGradeResponse(t *testing.T) {
	raw := `{"sufficient": false, "follow_up_queries": ["article 7 election", "article 16 urgence"], "grades": [{"n": 1, "verdict": "relevant"}, {"n": 2, "verdict": "off_topic"}]}`
	result := parseCragGradeResponse(raw)
	if result.Sufficient {
		t.Fatal("expected insufficient")
	}
	if len(result.FollowUpQueries) != 2 {
		t.Fatalf("expected 2 follow-up queries, got %d", len(result.FollowUpQueries))
	}
	if len(result.Grades) != 2 {
		t.Fatalf("expected 2 grades, got %d", len(result.Grades))
	}
}

func TestParseCragGradeResponseMarkdownWrapped(t *testing.T) {
	raw := "Here is the JSON:\n{\"sufficient\": true, \"follow_up_queries\": [], \"grades\": []}\n"
	result := parseCragGradeResponse(raw)
	if !result.Sufficient {
		t.Fatal("expected sufficient")
	}
}

func TestParseCragGradeResponseCapsFollowUp(t *testing.T) {
	raw := `{"sufficient": false, "follow_up_queries": ["a", "b", "c"], "grades": []}`
	result := parseCragGradeResponse(raw)
	if len(result.FollowUpQueries) != maxCRAGFollowUpQueries {
		t.Fatalf("expected capped follow-ups, got %d", len(result.FollowUpQueries))
	}
}

func TestMergeRetrievalOutcomesDedupes(t *testing.T) {
	primary := rankOutcome{
		hits: []model.RetrieveHit{{ChunkID: "c1", Score: 0.9}},
		chunksByID: map[string]model.Chunk{
			"c1": {Metadata: model.ChunkMetadata{ChunkID: "c1", Title: "A"}},
		},
	}
	secondary := rankOutcome{
		hits: []model.RetrieveHit{
			{ChunkID: "c1", Score: 0.5},
			{ChunkID: "c2", Score: 0.8},
		},
		chunksByID: map[string]model.Chunk{
			"c1": {Metadata: model.ChunkMetadata{ChunkID: "c1", Title: "A"}},
			"c2": {Metadata: model.ChunkMetadata{ChunkID: "c2", Title: "B"}},
		},
	}
	merged := mergeRetrievalOutcomes(primary, secondary, 8)
	if len(merged.hits) != 2 {
		t.Fatalf("expected 2 merged hits, got %d", len(merged.hits))
	}
}
