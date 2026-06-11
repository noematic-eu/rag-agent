package main

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/noematic-eu/ai-rag-agent/model"
)

const (
	searchLevelLinear = 1
	searchLevelCRAG   = 2
	searchLevelAgent  = 3

	escalationReasonSingleDocHighConfidence = "single_doc_high_confidence"
	escalationReasonMultihopQuery           = "multihop_query"
	escalationReasonWeakOrDispersed         = "weak_or_dispersed"
	escalationReasonDefaultLinear           = "default_linear"
)

type escalationConfig struct {
	minLinearScore     float64
	cragScoreThreshold float64
	dominantFraction   float64
}

func defaultEscalationConfig() escalationConfig {
	return escalationConfig{
		minLinearScore:     0.85,
		cragScoreThreshold: 0.55,
		dominantFraction:   0.67,
	}
}

type retrievalSignals struct {
	topScore          float64
	dominantDocID     string
	dominantFraction  float64
	uniqueDocsTopK    int
	topKConsidered    int
	multihopQuery     bool
	multihopQueryHint string
}

type escalationDecision struct {
	Level   int
	Reason  string
	Signals retrievalSignals
}

func computeRetrievalSignals(outcome rankOutcome, topK int, query string) retrievalSignals {
	signals := retrievalSignals{
		topKConsidered: topK,
	}
	if topK <= 0 {
		topK = 8
	}
	if outcome.noResults || len(outcome.hits) == 0 {
		signals.multihopQuery, signals.multihopQueryHint = detectMultihopQuery(query, signals)
		return signals
	}

	signals.topScore = outcome.hits[0].Score

	topN := min(3, len(outcome.hits))
	docCounts := make(map[string]int)
	for i := 0; i < topN; i++ {
		docID := strings.TrimSpace(outcome.hits[i].DocID)
		if docID == "" {
			if chunk, ok := outcome.chunksByID[outcome.hits[i].ChunkID]; ok {
				docID = strings.TrimSpace(chunk.Metadata.DocID)
			}
		}
		if docID != "" {
			docCounts[docID]++
		}
	}
	maxCount := 0
	for docID, count := range docCounts {
		if count > maxCount {
			maxCount = count
			signals.dominantDocID = docID
		}
	}
	if topN > 0 {
		signals.dominantFraction = float64(maxCount) / float64(topN)
	}

	uniqueDocs := make(map[string]struct{})
	limit := min(topK, len(outcome.hits))
	for i := 0; i < limit; i++ {
		docID := strings.TrimSpace(outcome.hits[i].DocID)
		if docID == "" {
			if chunk, ok := outcome.chunksByID[outcome.hits[i].ChunkID]; ok {
				docID = strings.TrimSpace(chunk.Metadata.DocID)
			}
		}
		if docID != "" {
			uniqueDocs[docID] = struct{}{}
		}
	}
	signals.uniqueDocsTopK = len(uniqueDocs)

	signals.multihopQuery, signals.multihopQueryHint = detectMultihopQuery(query, signals)
	return signals
}

var comparativeQueryPattern = regexp.MustCompile(`(?i)\b(compare|versus|vs\.|différence entre|relation entre|link between|how does .+ and .+)\b`)

func detectMultihopQuery(query string, signals retrievalSignals) (bool, string) {
	q := strings.TrimSpace(query)
	if q == "" {
		return false, ""
	}
	if comparativeQueryPattern.MatchString(q) {
		return true, "comparative_pattern"
	}
	if signals.uniqueDocsTopK >= 2 && hasMultiAspectMarkers(q) {
		return true, "multi_aspect"
	}
	return false, ""
}

func hasMultiAspectMarkers(query string) bool {
	lower := strings.ToLower(query)
	andCount := strings.Count(lower, " and ")
	etCount := strings.Count(lower, " et ")
	return andCount+etCount >= 2
}

func decideEscalation(outcome rankOutcome, query string, topK int, cfg escalationConfig) escalationDecision {
	if cfg.minLinearScore == 0 && cfg.cragScoreThreshold == 0 && cfg.dominantFraction == 0 {
		cfg = defaultEscalationConfig()
	}
	signals := computeRetrievalSignals(outcome, topK, query)

	if signals.multihopQuery {
		return escalationDecision{
			Level:   searchLevelAgent,
			Reason:  escalationReasonMultihopQuery,
			Signals: signals,
		}
	}

	if !outcome.noResults && len(outcome.hits) > 0 {
		if signals.topScore >= cfg.minLinearScore && signals.dominantFraction >= cfg.dominantFraction {
			return escalationDecision{
				Level:   searchLevelLinear,
				Reason:  escalationReasonSingleDocHighConfidence,
				Signals: signals,
			}
		}
	}

	if !outcome.noResults && len(outcome.hits) > 0 {
		if signals.topScore < cfg.cragScoreThreshold ||
			(signals.uniqueDocsTopK >= 3 && signals.dominantFraction < 0.5) {
			return escalationDecision{
				Level:   searchLevelCRAG,
				Reason:  escalationReasonWeakOrDispersed,
				Signals: signals,
			}
		}
	}

	return escalationDecision{
		Level:   searchLevelLinear,
		Reason:  escalationReasonDefaultLinear,
		Signals: signals,
	}
}

func (d escalationDecision) shouldRunCRAG() bool {
	return d.Level >= searchLevelCRAG
}

func (d escalationDecision) shouldRunAgent() bool {
	return d.Level >= searchLevelAgent
}

func applyEscalationDecision(mode searchModeConfig, decision escalationDecision) searchModeConfig {
	resolved := mode
	resolved.autoEnabled = false
	resolved.level = decision.Level
	resolved.cragEnabled = decision.shouldRunCRAG()
	resolved.agentEnabled = decision.shouldRunAgent()
	return resolved
}

func escalationPayload(decision escalationDecision) map[string]interface{} {
	return map[string]interface{}{
		"resolved_level":      decision.Level,
		"reason":              decision.Reason,
		"top_score":           decision.Signals.topScore,
		"dominant_doc_id":     decision.Signals.dominantDocID,
		"dominant_fraction":   decision.Signals.dominantFraction,
		"unique_docs":         decision.Signals.uniqueDocsTopK,
		"multihop_query":      decision.Signals.multihopQuery,
		"multihop_query_hint": decision.Signals.multihopQueryHint,
	}
}

func escalationExtraMeta(requestedLevel string, decision escalationDecision) map[string]string {
	return map[string]string{
		"search_level":           strconv.Itoa(decision.Level),
		"search_level_requested": requestedLevel,
		"escalation_reason":      decision.Reason,
	}
}

func shouldPostCRAGEscalateToAgent(autoEnabled bool, decision escalationDecision, cragTrace cragTrace) bool {
	if !autoEnabled {
		return false
	}
	if decision.Level != searchLevelCRAG {
		return false
	}
	if cragTrace.Sufficient {
		return false
	}
	return decision.Signals.multihopQuery
}

func frenchRevolutionFixtureOutcome() rankOutcome {
	docID := "french-revolution"
	return rankOutcome{
		hits: []model.RetrieveHit{
			{ChunkID: "c1", DocID: docID, Score: 0.97},
			{ChunkID: "c2", DocID: docID, Score: 0.96},
			{ChunkID: "c3", DocID: docID, Score: 0.95},
		},
	}
}
