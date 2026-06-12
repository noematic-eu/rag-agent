package main

import (
	"context"
	"log"
	"strconv"
	"strings"

	"github.com/noematic-eu/ai-rag-agent/model"
)

type agenticSearchOptions struct {
	initialOutcome *rankOutcome
	autoDecision   *escalationDecision
	autoEnabled    bool
}

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
	opts agenticSearchOptions,
) (rankOutcome, []model.LegalDocument, []string, map[string]string, error) {
	var (
		outcome        rankOutcome
		rewriteQueries []string
		err            error
	)

	if opts.initialOutcome != nil {
		outcome = *opts.initialOutcome
	} else {
		outcome, rewriteQueries, err = runRetrievalPipeline(pipeline)
		if err != nil {
			return rankOutcome{}, nil, rewriteQueries, nil, err
		}
	}

	extraMeta := map[string]string{
		"search_mode": mode.modeLabel(),
	}
	if outcome.noResults {
		docs := maybeApplyWebSupplement(ctx, generationQuery, lang, webGapNoResults, nil, nil, extraMeta, w)
		return outcome, docs, rewriteQueries, extraMeta, nil
	}
	applyLexicalSourceMeta(extraMeta, outcome.lexicalSource)
	if opts.autoDecision != nil {
		for k, v := range escalationExtraMeta(mode.requestedLevelLabel(), *opts.autoDecision) {
			extraMeta[k] = v
		}
	} else if mode.level >= searchLevelLinear {
		extraMeta["search_level"] = strconv.Itoa(mode.level)
		extraMeta["search_level_requested"] = mode.requestedLevelLabel()
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

	if opts.autoDecision != nil && w != nil {
		_ = w.WriteAgentEvent("escalation", escalationPayload(*opts.autoDecision))
	}

	var cragTrace cragTrace
	if mode.cragEnabled {
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

	runAgent := mode.agentEnabled
	if !runAgent && opts.autoEnabled && opts.autoDecision != nil {
		runAgent = shouldPostCRAGEscalateToAgent(true, *opts.autoDecision, cragTrace)
		if runAgent {
			mode.agentEnabled = true
			mode.level = searchLevelAgent
			extraMeta["search_level"] = strconv.Itoa(searchLevelAgent)
			extraMeta["escalation_reason"] = "post_crag_multihop"
			if w != nil {
				_ = w.WriteAgentEvent("escalation", map[string]interface{}{
					"resolved_level": searchLevelAgent,
					"reason":         "post_crag_multihop",
				})
			}
		}
	}

	if runAgent {
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
		if reason, need := detectWebGapAfterAgent(ctx, generationQuery, lang, docs, pipeline.params.topKFinal); need {
			docs = maybeApplyWebSupplement(ctx, generationQuery, lang, reason, cragTrace.FollowUpQueries, docs, extraMeta, w)
		}
		return outcome, docs, rewriteQueries, extraMeta, nil
	}

	if reason, need := detectWebGapAfterCRAG(cragTrace, mode.cragEnabled); need {
		docs = maybeApplyWebSupplement(ctx, generationQuery, lang, reason, cragTrace.FollowUpQueries, docs, extraMeta, w)
	}

	return outcome, docs, rewriteQueries, extraMeta, nil
}

func executeSearch(
	ctx context.Context,
	pipeline retrievalPipelineInput,
	generationQuery, lang string,
	mode searchModeConfig,
	w StreamWriter,
) (rankOutcome, []model.LegalDocument, []string, map[string]string, error) {
	if mode.autoEnabled {
		outcome, rewriteQueries, err := runRetrievalPipeline(pipeline)
		if err != nil {
			return rankOutcome{}, nil, rewriteQueries, nil, err
		}
		if outcome.noResults {
			extraMeta := map[string]string{"search_mode": mode.modeLabel()}
			docs := maybeApplyWebSupplement(ctx, generationQuery, lang, webGapNoResults, nil, nil, extraMeta, w)
			return outcome, docs, rewriteQueries, extraMeta, nil
		}

		decision := decideEscalation(outcome, generationQuery, pipeline.params.topKFinal, mode.escalation)
		resolved := applyEscalationDecision(mode, decision)
		resolved.autoEnabled = true

		if resolved.cragEnabled || resolved.agentEnabled {
			return runSearchWithAgenticModes(ctx, pipeline, generationQuery, lang, resolved, w, agenticSearchOptions{
				initialOutcome: &outcome,
				autoDecision:   &decision,
				autoEnabled:    true,
			})
		}

		if w != nil {
			_ = w.WriteAgentEvent("escalation", escalationPayload(decision))
		}
		docs := retrieveHitsToDocuments(outcome.hits, outcome.chunksByID)
		extraMeta := escalationExtraMeta(mode.requestedLevelLabel(), decision)
		extraMeta["search_mode"] = resolved.modeLabel()
		applyLexicalSourceMeta(extraMeta, outcome.lexicalSource)
		return outcome, docs, rewriteQueries, extraMeta, nil
	}

	if mode.cragEnabled || mode.agentEnabled {
		return runSearchWithAgenticModes(ctx, pipeline, generationQuery, lang, mode, w, agenticSearchOptions{})
	}

	outcome, rewriteQueries, err := runRetrievalPipeline(pipeline)
	if err != nil {
		return rankOutcome{}, nil, rewriteQueries, nil, err
	}
	if outcome.noResults {
		extraMeta := map[string]string{
			"search_mode":            mode.modeLabel(),
			"search_level":           "1",
			"search_level_requested": mode.requestedLevelLabel(),
		}
		docs := maybeApplyWebSupplement(ctx, generationQuery, lang, webGapNoResults, nil, nil, extraMeta, w)
		return outcome, docs, rewriteQueries, extraMeta, nil
	}
	docs := retrieveHitsToDocuments(outcome.hits, outcome.chunksByID)
	extraMeta := map[string]string{
		"search_mode":            mode.modeLabel(),
		"search_level":           "1",
		"search_level_requested": mode.requestedLevelLabel(),
	}
	applyLexicalSourceMeta(extraMeta, outcome.lexicalSource)
	return outcome, docs, rewriteQueries, extraMeta, nil
}
