package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

type baselineEvalConfig struct {
	ServerURL  string
	TopK       int
	OutputJSON string
	GoldPaths  []string
}

type baselineSetReport struct {
	Set       string  `json:"set"`
	Cases     int     `json:"cases"`
	RecallAtK float64 `json:"recall_at_k"`
	MRR       float64 `json:"mrr"`
}

type baselineEvalReport struct {
	GeneratedAt time.Time           `json:"generated_at"`
	Server      string              `json:"server"`
	TopK        int                 `json:"top_k"`
	Mode        string              `json:"mode"`
	Sets        []baselineSetReport `json:"sets"`
}

// RunBaselineEval measures single-pass retrieval recall on legal and multi-hop gold sets.
func RunBaselineEval(cfg baselineEvalConfig) error {
	if cfg.TopK <= 0 {
		cfg.TopK = 8
	}
	if len(cfg.GoldPaths) == 0 {
		cfg.GoldPaths = []string{
			"eval/gold/legal.jsonl",
			"eval/gold/multihop.jsonl",
		}
	}
	if cfg.OutputJSON == "" {
		cfg.OutputJSON = "eval/out/agentic_baseline.json"
	}

	report := baselineEvalReport{
		GeneratedAt: time.Now().UTC(),
		Server:      cfg.ServerURL,
		TopK:        cfg.TopK,
		Mode:        "single_pass_retrieval",
	}

	client := &http.Client{Timeout: 2 * time.Minute}
	for _, goldPath := range cfg.GoldPaths {
		setName := filepath.Base(goldPath)
		recall, mrr, cases, err := retrievalMetrics(client, cfg.ServerURL, goldPath, cfg.TopK)
		if err != nil {
			return fmt.Errorf("%s: %w", goldPath, err)
		}
		report.Sets = append(report.Sets, baselineSetReport{
			Set:       setName,
			Cases:     cases,
			RecallAtK: recall,
			MRR:       mrr,
		})
		fmt.Printf("baseline set=%s cases=%d recall@%d=%.3f mrr=%.3f\n", setName, cases, cfg.TopK, recall, mrr)
	}

	if err := writeBaselineReport(cfg.OutputJSON, report); err != nil {
		return err
	}
	fmt.Printf("baseline report written to %s\n", cfg.OutputJSON)
	return nil
}

func retrievalMetrics(client *http.Client, serverURL, goldPath string, topK int) (recall, mrr float64, cases int, err error) {
	goldCases, err := loadGoldCases(goldPath)
	if err != nil {
		return 0, 0, 0, err
	}
	var recallSum, mrrSum float64
	for _, gc := range goldCases {
		hits, status, callErr := callRetrieve(client, serverURL, gc, topK)
		if callErr != nil {
			return 0, 0, 0, callErr
		}
		if gc.ExpectNoResults {
			if status == "no_results" || len(hits) == 0 {
				recallSum += 1
			}
			continue
		}
		hit, rr := scoreRetrieval(gc, hits, topK)
		if hit {
			recallSum += 1
		}
		mrrSum += rr
	}
	n := float64(len(goldCases))
	if n == 0 {
		return 0, 0, 0, fmt.Errorf("no cases in %s", goldPath)
	}
	return recallSum / n, mrrSum / n, len(goldCases), nil
}

func writeBaselineReport(path string, report baselineEvalReport) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}
