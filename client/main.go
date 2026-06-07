package main

import (
	"flag"
	"fmt"
	"os"
)

func main() {
	serverURL := flag.String("server", "http://localhost:8080", "Base URL of the rag-agent API")
	mode := flag.String("mode", "ingest-dir", "Mode: ingest-dir, bench, eval-retrieval, eval-excerpt, eval-generation, or demo")
	dir := flag.String("dir", "", "Directory containing .md and .html files (ingest-dir mode)")
	finalize := flag.Bool("finalize", true, "Call /finalize after directory ingestion")
	batchSize := flag.Int("batch-size", 100, "Log progress every N ingested files")
	resetBefore := flag.Bool("reset-before-ingest", false, "Call POST /reset before directory ingestion")
	corpus := flag.String("corpus", "", "Optional corpus tag applied to all ingested documents")
	goldPath := flag.String("gold", "", "Gold JSONL path (eval-retrieval mode)")
	evalTopK := flag.Int("eval-top-k", 8, "Top-K for retrieval eval")
	minRecall := flag.Float64("min-recall", 0, "Fail eval-retrieval if recall@k is below this (0=disabled)")
	minExcerptPass := flag.Float64("min-excerpt-pass", 0, "Fail eval-excerpt if pass rate is below this (0=disabled)")
	minGenerationPass := flag.Float64("min-generation-pass", 0, "Fail eval-generation if pass rate is below this (0=disabled)")
	evalOut := flag.String("eval-out", "", "Write retrieval eval JSON report to this path")
	evalSet := flag.String("eval-set", "", "Label for eval report (default: gold file basename)")
	flag.Parse()

	switch *mode {
	case "demo":
		ConstitionFrancaise()
	case "bench":
		benchMain([]string{*serverURL})
	case "eval-retrieval":
		if *goldPath == "" {
			fmt.Fprintln(os.Stderr, "missing required -gold for eval-retrieval mode")
			os.Exit(1)
		}
		setName := *evalSet
		if setName == "" {
			setName = *goldPath
		}
		if err := RunRetrievalEval(retrievalEvalConfig{
			ServerURL:  *serverURL,
			GoldPath:   *goldPath,
			TopK:       *evalTopK,
			MinRecall:  *minRecall,
			OutputJSON: *evalOut,
			SetName:    setName,
		}); err != nil {
			fmt.Fprintf(os.Stderr, "retrieval eval failed: %v\n", err)
			os.Exit(1)
		}
	case "eval-excerpt":
		if *goldPath == "" {
			fmt.Fprintln(os.Stderr, "missing required -gold for eval-excerpt mode")
			os.Exit(1)
		}
		setName := *evalSet
		if setName == "" {
			setName = *goldPath
		}
		if err := RunExcerptEval(excerptEvalConfig{
			ServerURL:  *serverURL,
			GoldPath:   *goldPath,
			TopK:       *evalTopK,
			MinPass:    *minExcerptPass,
			OutputJSON: *evalOut,
			SetName:    setName,
		}); err != nil {
			fmt.Fprintf(os.Stderr, "excerpt eval failed: %v\n", err)
			os.Exit(1)
		}
	case "eval-generation":
		if *goldPath == "" {
			fmt.Fprintln(os.Stderr, "missing required -gold for eval-generation mode")
			os.Exit(1)
		}
		setName := *evalSet
		if setName == "" {
			setName = *goldPath
		}
		if err := RunGenerationEval(generationEvalConfig{
			ServerURL:  *serverURL,
			GoldPath:   *goldPath,
			MinPass:    *minGenerationPass,
			OutputJSON: *evalOut,
			SetName:    setName,
		}); err != nil {
			fmt.Fprintf(os.Stderr, "generation eval failed: %v\n", err)
			os.Exit(1)
		}
	case "ingest-dir":
		if *dir == "" {
			fmt.Fprintln(os.Stderr, "missing required -dir for ingest-dir mode")
			os.Exit(1)
		}
		cfg := IngestDirectoryConfig{
			ServerURL:   *serverURL,
			DirPath:     *dir,
			Finalize:    *finalize,
			BatchSize:   *batchSize,
			ResetBefore: *resetBefore,
			Corpus:      *corpus,
		}
		if err := IngestDirectory(cfg); err != nil {
			fmt.Fprintf(os.Stderr, "directory ingestion failed: %v\n", err)
			os.Exit(1)
		}
	default:
		fmt.Fprintf(os.Stderr, "unsupported mode %q\n", *mode)
		os.Exit(1)
	}
}
