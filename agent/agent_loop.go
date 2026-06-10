package main

import (
	"context"
	"log"
	"strconv"
	"strings"

	"github.com/noematic-eu/ai-rag-agent/model"
)

type agentTrace struct {
	Iterations     int      `json:"iterations"`
	RetrievalCalls int      `json:"retrieval_calls"`
	Actions        []string `json:"actions,omitempty"`
}

func runAgentSearchLoop(
	ctx context.Context,
	pipeline retrievalPipelineInput,
	initial rankOutcome,
	generationQuery, lang string,
	emit func(eventType string, payload map[string]interface{}),
) ([]model.LegalDocument, agentTrace, error) {
	trace := agentTrace{}
	toolCtx := newAgentToolContext(retrieveHitsToDocuments(initial.hits, initial.chunksByID))
	topK := pipeline.params.topKFinal

	for i := 0; i < defaultAgentMaxIter; i++ {
		trace.Iterations = i + 1
		raw, err := completeLLM(ctx, agentSystemPrompt(lang), agentTurnUserPrompt(lang, generationQuery, toolCtx.collectedDocs, topK))
		if err != nil {
			log.Printf("agent: LLM turn failed: %v", err)
			break
		}
		action := parseAgentAction(raw)
		trace.Actions = append(trace.Actions, action.Name)

		if emit != nil {
			emit("tool_call", map[string]interface{}{
				"action":   action.Name,
				"query":    action.Query,
				"chunk_id": action.ChunkID,
			})
		}

		var toolResult string
		switch action.Name {
		case "finish":
			if emit != nil {
				emit("tool_result", map[string]interface{}{
					"action": "finish",
					"reason": action.Reason,
				})
			}
			trace.RetrievalCalls = toolCtx.retrievalCount
			return toolCtx.collectedDocs, trace, nil
		case "search_kb":
			toolResult, err = executeSearchKB(toolCtx, pipeline, action.Query, action.Corpus)
		case "get_chunk":
			toolResult, err = executeGetChunk(toolCtx, action.ChunkID)
		default:
			toolResult = "unknown action; use search_kb, get_chunk, or finish"
		}
		if err != nil {
			return toolCtx.collectedDocs, trace, err
		}
		if emit != nil {
			emit("tool_result", map[string]interface{}{
				"action": action.Name,
				"result": truncateForSSE(toolResult, 500),
			})
		}
	}

	trace.RetrievalCalls = toolCtx.retrievalCount
	return toolCtx.collectedDocs, trace, nil
}

func truncateForSSE(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}

func runSearchWithAgenticModes(
	ctx context.Context,
	pipeline retrievalPipelineInput,
	generationQuery, lang string,
	mode searchModeConfig,
	w StreamWriter,
) (rankOutcome, []model.LegalDocument, []string, map[string]string, error) {
	outcome, rewriteQueries, err := runRetrievalPipeline(pipeline)
	if err != nil {
		return rankOutcome{}, nil, rewriteQueries, nil, err
	}
	if outcome.noResults {
		return outcome, nil, rewriteQueries, nil, nil
	}

	extraMeta := map[string]string{
		"search_mode": mode.modeLabel(),
	}

	emitRound := func(round int, eventType string, payload map[string]interface{}) {
		if w == nil {
			return
		}
		payload["round"] = round
		_ = w.WriteAgentEvent(eventType, payload)
	}
	emitAgent := func(eventType string, payload map[string]interface{}) {
		if w == nil {
			return
		}
		_ = w.WriteAgentEvent(eventType, payload)
	}

	if mode.cragEnabled {
		var cragTrace cragTrace
		outcome, cragTrace, err = applyCRAGLoop(ctx, pipeline, outcome, generationQuery, lang, mode.cragMaxRounds, emitRound)
		if err != nil {
			return rankOutcome{}, nil, rewriteQueries, extraMeta, err
		}
		extraMeta["crag_rounds"] = strconv.Itoa(cragTrace.Rounds)
		extraMeta["crag_sufficient"] = strconv.FormatBool(cragTrace.Sufficient)
		if len(cragTrace.FollowUpQueries) > 0 {
			extraMeta["crag_follow_up"] = strings.Join(cragTrace.FollowUpQueries, " | ")
		}
	}

	docs := retrieveHitsToDocuments(outcome.hits, outcome.chunksByID)

	if mode.agentEnabled {
		var agentTrace agentTrace
		docs, agentTrace, err = runAgentSearchLoop(ctx, pipeline, outcome, generationQuery, lang, emitAgent)
		if err != nil {
			return outcome, docs, rewriteQueries, extraMeta, err
		}
		extraMeta["agent_iterations"] = strconv.Itoa(agentTrace.Iterations)
		extraMeta["agent_retrievals"] = strconv.Itoa(agentTrace.RetrievalCalls)
		if len(agentTrace.Actions) > 0 {
			extraMeta["agent_actions"] = strings.Join(agentTrace.Actions, ",")
		}
		return outcome, docs, rewriteQueries, extraMeta, nil
	}

	return outcome, docs, rewriteQueries, extraMeta, nil
}
