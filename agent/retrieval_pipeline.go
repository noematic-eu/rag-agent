package main

import (
	"strings"

	"github.com/gin-gonic/gin"
)

type retrievalPipelineInput struct {
	generationQuery   string
	explicitRetrieval string
	params            rankParams
	rewriteEnabled    bool
	hydeForced        bool
	hydeAuto          bool
}

func retrievalPipelineFromContext(c *gin.Context, generationQuery, explicitRetrieval string) retrievalPipelineInput {
	corpus := strings.TrimSpace(c.Query("corpus"))
	rewrite := parseRewriteParam(c.Query("rewrite"), corpus)
	hydeForced, hydeAuto := parseHydeParam(c.Query("hyde"))
	if hydeAuto && corpus == "legal-demo" {
		hydeAuto = true
	}
	return retrievalPipelineInput{
		generationQuery:   generationQuery,
		explicitRetrieval: explicitRetrieval,
		params:            rankParamsFromContext(c, explicitRetrieval),
		rewriteEnabled:    rewrite,
		hydeForced:        hydeForced,
		hydeAuto:          hydeAuto,
	}
}

func retrievalPipelineFromOptions(opts RankOptions, generationQuery, explicitRetrieval string) retrievalPipelineInput {
	rewrite := parseRewriteParam("", opts.Corpus)
	if opts.Corpus == "legal-demo" {
		rewrite = true
	}
	return retrievalPipelineInput{
		generationQuery:   generationQuery,
		explicitRetrieval: explicitRetrieval,
		params:            rankParamsFromOptions(opts),
		rewriteEnabled:    rewrite,
		hydeAuto:          opts.Corpus == "legal-demo",
	}
}

func runRetrievalPipeline(in retrievalPipelineInput) (rankOutcome, []string, error) {
	queries := buildRetrievalQueries(in.generationQuery, in.explicitRetrieval, in.params.corpus, in.rewriteEnabled)

	outcome, err := rankChunksMulti(queries, in.params)
	if err != nil {
		return rankOutcome{}, queries, err
	}

	primaryTopScore := 0.0
	if len(queries) > 0 {
		primary := in.params
		primary.retrievalText = queries[0]
		primaryOutcome, perr := rankChunks(primary)
		if perr == nil && len(primaryOutcome.hits) > 0 {
			primaryTopScore = primaryOutcome.hits[0].Score
		}
	}

	if shouldApplyHyde(in.hydeForced, in.hydeAuto, primaryTopScore) {
		outcome = applyHydeBoost(outcome, in.generationQuery, in.params)
	}

	return outcome, queries, nil
}
