package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// GoldCase is one labeled evaluation row (JSONL).
type GoldCase struct {
	ID                    string              `json:"id"`
	Corpus                string              `json:"corpus,omitempty"`
	RetrievalQ            string              `json:"retrieval_q"`
	GenerationQ           string              `json:"generation_q,omitempty"`
	ExpectedChunkIDs      []string            `json:"expected_chunk_ids,omitempty"`
	ExpectedDocIDs        []string            `json:"expected_doc_ids,omitempty"`
	ExpectedSections      []string            `json:"expected_sections,omitempty"`
	MatchAllSections      bool                `json:"match_all_sections,omitempty"`
	MatchAllDocIDs        bool                `json:"match_all_doc_ids,omitempty"`
	ExcerptTermsBySection map[string][]string `json:"excerpt_terms_by_section,omitempty"`
	GenerationPhrases     []string            `json:"generation_phrases,omitempty"`
	ReferenceAnswer       string              `json:"reference_answer,omitempty"`
	ExpectNoResults       bool                `json:"expect_no_results,omitempty"`
}

type retrievalEvalConfig struct {
	ServerURL  string
	GoldPath   string
	TopK       int
	MinRecall  float64
	OutputJSON string
	SetName    string
}

type retrievalEvalReport struct {
	Set        string             `json:"set"`
	Server     string             `json:"server"`
	TopK       int                `json:"top_k"`
	Cases      int                `json:"cases"`
	RecallAtK  float64            `json:"recall_at_k"`
	MRR        float64            `json:"mrr"`
	RefusalAcc float64            `json:"refusal_accuracy,omitempty"`
	PerCase    []retrievalCaseRow `json:"per_case"`
}

type retrievalCaseRow struct {
	ID         string  `json:"id"`
	Hit        bool    `json:"hit"`
	Reciprocal float64 `json:"reciprocal_rank"`
	Retrieved  int     `json:"retrieved"`
	ExpectMiss bool    `json:"expect_no_results"`
}

type retrieveHit struct {
	ChunkID string  `json:"chunk_id"`
	DocID   string  `json:"doc_id"`
	Score   float64 `json:"score"`
	Section string  `json:"section,omitempty"`
	Article string  `json:"article,omitempty"`
	Excerpt string  `json:"excerpt,omitempty"`
}

type retrieveAPIResponse struct {
	Status string        `json:"status"`
	Hits   []retrieveHit `json:"hits"`
}

func RunRetrievalEval(cfg retrievalEvalConfig) error {
	cases, err := loadGoldCases(cfg.GoldPath)
	if err != nil {
		return err
	}
	if len(cases) == 0 {
		return fmt.Errorf("no gold cases in %s", cfg.GoldPath)
	}
	if cfg.TopK <= 0 {
		cfg.TopK = 8
	}

	client := &http.Client{Timeout: 2 * time.Minute}
	rows := make([]retrievalCaseRow, 0, len(cases))
	var recallSum, mrrSum float64
	var refusalTotal, refusalOK int

	for _, gc := range cases {
		hits, status, err := callRetrieve(client, cfg.ServerURL, gc, cfg.TopK)
		if err != nil {
			return fmt.Errorf("%s: %w", gc.ID, err)
		}

		if gc.ExpectNoResults {
			refusalTotal++
			ok := status == "no_results" || len(hits) == 0
			if ok {
				refusalOK++
			}
			rows = append(rows, retrievalCaseRow{
				ID: gc.ID, Hit: ok, Reciprocal: 0, Retrieved: len(hits), ExpectMiss: true,
			})
			if ok {
				recallSum += 1
			}
			continue
		}

		hit, rr := scoreRetrieval(gc, hits, cfg.TopK)
		rows = append(rows, retrievalCaseRow{
			ID: gc.ID, Hit: hit, Reciprocal: rr, Retrieved: len(hits),
		})
		if hit {
			recallSum += 1
		}
		mrrSum += rr
	}

	n := float64(len(cases))
	report := retrievalEvalReport{
		Set:       cfg.SetName,
		Server:    cfg.ServerURL,
		TopK:      cfg.TopK,
		Cases:     len(cases),
		RecallAtK: recallSum / n,
		MRR:       mrrSum / n,
		PerCase:   rows,
	}
	if refusalTotal > 0 {
		report.RefusalAcc = float64(refusalOK) / float64(refusalTotal)
	}

	if cfg.OutputJSON != "" {
		if err := writeJSONReport(cfg.OutputJSON, report); err != nil {
			return err
		}
	}

	printRetrievalSummary(report)
	if cfg.MinRecall > 0 && report.RecallAtK < cfg.MinRecall {
		return fmt.Errorf("recall@%d %.3f below threshold %.3f", cfg.TopK, report.RecallAtK, cfg.MinRecall)
	}
	return nil
}

type excerptEvalConfig struct {
	ServerURL  string
	GoldPath   string
	TopK       int
	MinPass    float64
	OutputJSON string
	SetName    string
}

type excerptEvalReport struct {
	Set      string           `json:"set"`
	Server   string           `json:"server"`
	TopK     int              `json:"top_k"`
	Cases    int              `json:"cases"`
	PassRate float64          `json:"pass_rate"`
	PerCase  []excerptCaseRow `json:"per_case"`
}

type excerptCaseRow struct {
	ID     string   `json:"id"`
	Pass   bool     `json:"pass"`
	Missed []string `json:"missed,omitempty"`
}

func RunExcerptEval(cfg excerptEvalConfig) error {
	cases, err := loadGoldCases(cfg.GoldPath)
	if err != nil {
		return err
	}
	excerptCases := make([]GoldCase, 0, len(cases))
	for _, gc := range cases {
		if len(gc.ExcerptTermsBySection) > 0 {
			excerptCases = append(excerptCases, gc)
		}
	}
	if len(excerptCases) == 0 {
		return fmt.Errorf("no gold cases with excerpt_terms_by_section in %s", cfg.GoldPath)
	}
	if cfg.TopK <= 0 {
		cfg.TopK = 8
	}

	client := &http.Client{Timeout: 2 * time.Minute}
	rows := make([]excerptCaseRow, 0, len(excerptCases))
	var passSum float64

	for _, gc := range excerptCases {
		hits, _, err := callRetrieveWithExcerpts(client, cfg.ServerURL, gc, cfg.TopK)
		if err != nil {
			return fmt.Errorf("%s: %w", gc.ID, err)
		}
		pass, missed := scoreExcerptTerms(gc, hits, cfg.TopK)
		rows = append(rows, excerptCaseRow{ID: gc.ID, Pass: pass, Missed: missed})
		if pass {
			passSum++
		}
	}

	n := float64(len(excerptCases))
	report := excerptEvalReport{
		Set:      cfg.SetName,
		Server:   cfg.ServerURL,
		TopK:     cfg.TopK,
		Cases:    len(excerptCases),
		PassRate: passSum / n,
		PerCase:  rows,
	}

	if cfg.OutputJSON != "" {
		if err := writeExcerptJSONReport(cfg.OutputJSON, report); err != nil {
			return err
		}
	}

	printExcerptSummary(report)
	if cfg.MinPass > 0 && report.PassRate < cfg.MinPass {
		return fmt.Errorf("excerpt pass rate %.3f below threshold %.3f", report.PassRate, cfg.MinPass)
	}
	return nil
}

func callRetrieveWithExcerpts(client *http.Client, server string, gc GoldCase, topK int) ([]retrieveHit, string, error) {
	params := url.Values{}
	if gc.RetrievalQ != "" {
		params.Set("rq", gc.RetrievalQ)
	} else {
		params.Set("q", gc.GenerationQ)
		params.Set("rewrite", "true")
	}
	params.Set("top_k", fmt.Sprintf("%d", topK))
	params.Set("include_text", "1")
	if gc.Corpus != "" {
		params.Set("corpus", gc.Corpus)
	}

	u := strings.TrimRight(server, "/") + "/retrieve?" + params.Encode()
	resp, err := client.Get(u)
	if err != nil {
		return nil, "", err
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
	}

	var parsed retrieveAPIResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, "", err
	}
	return parsed.Hits, parsed.Status, nil
}

func scoreExcerptTerms(gc GoldCase, hits []retrieveHit, topK int) (pass bool, missed []string) {
	for section, terms := range gc.ExcerptTermsBySection {
		excerpt := excerptForSection(hits, section, topK)
		if excerpt == "" {
			missed = append(missed, section+": no excerpt")
			continue
		}
		lower := strings.ToLower(excerpt)
		for _, term := range terms {
			if !strings.Contains(lower, strings.ToLower(term)) {
				missed = append(missed, section+": missing "+term)
			}
		}
	}
	return len(missed) == 0, missed
}

func excerptForSection(hits []retrieveHit, want string, topK int) string {
	for rank, h := range hits {
		if rank >= topK {
			break
		}
		if sectionMatches(h, want) {
			return h.Excerpt
		}
	}
	return ""
}

func writeExcerptJSONReport(path string, report excerptEvalReport) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func printExcerptSummary(r excerptEvalReport) {
	fmt.Printf("set=%s excerpt_cases=%d pass_rate=%.3f\n", r.Set, r.Cases, r.PassRate)
	for _, row := range r.PerCase {
		mark := "FAIL"
		if row.Pass {
			mark = "PASS"
		}
		fmt.Printf("  %s %s", row.ID, mark)
		if len(row.Missed) > 0 {
			fmt.Printf(" missed=%v", row.Missed)
		}
		fmt.Println()
	}
}

func loadGoldCases(path string) ([]GoldCase, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()

	var cases []GoldCase
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		var gc GoldCase
		if err := json.Unmarshal([]byte(line), &gc); err != nil {
			return nil, fmt.Errorf("parse gold: %w", err)
		}
		if gc.RetrievalQ == "" && gc.GenerationQ == "" {
			return nil, fmt.Errorf("gold case %q missing retrieval_q or generation_q", gc.ID)
		}
		cases = append(cases, gc)
	}
	return cases, sc.Err()
}

func callRetrieve(client *http.Client, server string, gc GoldCase, topK int) ([]retrieveHit, string, error) {
	params := url.Values{}
	if gc.RetrievalQ != "" {
		params.Set("rq", gc.RetrievalQ)
	} else {
		params.Set("q", gc.GenerationQ)
		params.Set("rewrite", "true")
	}
	params.Set("top_k", fmt.Sprintf("%d", topK))
	if gc.Corpus != "" {
		params.Set("corpus", gc.Corpus)
	}

	u := strings.TrimRight(server, "/") + "/retrieve?" + params.Encode()
	resp, err := client.Get(u)
	if err != nil {
		return nil, "", err
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", err
	}
	if resp.StatusCode != http.StatusOK {
		if resp.StatusCode == http.StatusNotFound {
			return nil, "", fmt.Errorf("HTTP 404: /retrieve not found — rebuild and restart the agent (make agent, then restart ./bin/agent)")
		}
		return nil, "", fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
	}

	var parsed retrieveAPIResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, "", err
	}
	return parsed.Hits, parsed.Status, nil
}

func scoreRetrieval(gc GoldCase, hits []retrieveHit, topK int) (hit bool, mrr float64) {
	if len(gc.ExpectedSections) > 0 {
		return scoreSectionRetrieval(gc, hits, topK)
	}

	chunkSet := make(map[string]struct{}, len(gc.ExpectedChunkIDs))
	for _, id := range gc.ExpectedChunkIDs {
		chunkSet[id] = struct{}{}
	}
	docSet := make(map[string]struct{}, len(gc.ExpectedDocIDs))
	for _, id := range gc.ExpectedDocIDs {
		docSet[id] = struct{}{}
	}
	if len(chunkSet) == 0 && len(docSet) == 0 {
		return false, 0
	}

	if gc.MatchAllDocIDs && len(docSet) > 0 {
		found := make(map[string]bool, len(gc.ExpectedDocIDs))
		bestRank := 0
		for rank, h := range hits {
			if rank >= topK {
				break
			}
			if _, ok := docSet[h.DocID]; ok {
				if !found[h.DocID] {
					found[h.DocID] = true
					if bestRank == 0 || rank+1 < bestRank {
						bestRank = rank + 1
					}
				}
			}
		}
		if len(found) == len(docSet) {
			return true, 1.0 / float64(bestRank)
		}
		return false, 0
	}

	for rank, h := range hits {
		if rank >= topK {
			break
		}
		if _, ok := chunkSet[h.ChunkID]; ok {
			return true, 1.0 / float64(rank+1)
		}
		if _, ok := docSet[h.DocID]; ok {
			return true, 1.0 / float64(rank+1)
		}
	}
	return false, 0
}

func scoreSectionRetrieval(gc GoldCase, hits []retrieveHit, topK int) (hit bool, mrr float64) {
	if gc.MatchAllSections {
		found := make(map[string]bool, len(gc.ExpectedSections))
		bestRank := 0
		for rank, h := range hits {
			if rank >= topK {
				break
			}
			for _, want := range gc.ExpectedSections {
				if sectionMatches(h, want) {
					if !found[want] {
						found[want] = true
						if bestRank == 0 || rank+1 < bestRank {
							bestRank = rank + 1
						}
					}
				}
			}
		}
		if len(found) == len(gc.ExpectedSections) {
			return true, 1.0 / float64(bestRank)
		}
		return false, 0
	}

	for rank, h := range hits {
		if rank >= topK {
			break
		}
		for _, want := range gc.ExpectedSections {
			if sectionMatches(h, want) {
				return true, 1.0 / float64(rank+1)
			}
		}
	}
	return false, 0
}

func sectionMatches(h retrieveHit, want string) bool {
	want = strings.ToUpper(strings.TrimSpace(want))
	if want == "" {
		return false
	}
	section := strings.ToUpper(h.Section)
	if strings.Contains(section, want) {
		return true
	}
	if h.Article != "" && strings.Contains(want, "ARTICLE "+strings.ToUpper(h.Article)) {
		return true
	}
	return false
}

func writeJSONReport(path string, report retrievalEvalReport) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func printRetrievalSummary(r retrievalEvalReport) {
	fmt.Printf("set=%s cases=%d recall@%d=%.3f mrr=%.3f\n", r.Set, r.Cases, r.TopK, r.RecallAtK, r.MRR)
	if r.RefusalAcc > 0 {
		fmt.Printf("refusal_accuracy=%.3f\n", r.RefusalAcc)
	}
	for _, row := range r.PerCase {
		mark := "MISS"
		if row.Hit {
			mark = "HIT"
		}
		fmt.Printf("  %s %s rr=%.3f retrieved=%d\n", row.ID, mark, row.Reciprocal, row.Retrieved)
	}
}

type generationEvalConfig struct {
	ServerURL  string
	GoldPath   string
	MinPass    float64
	OutputJSON string
	SetName    string
	SearchMode string
}

type generationEvalReport struct {
	Set      string              `json:"set"`
	Server   string              `json:"server"`
	Cases    int                 `json:"cases"`
	PassRate float64             `json:"pass_rate"`
	PerCase  []generationCaseRow `json:"per_case"`
}

type generationCaseRow struct {
	ID     string   `json:"id"`
	Pass   bool     `json:"pass"`
	Missed []string `json:"missed,omitempty"`
}

func RunGenerationEval(cfg generationEvalConfig) error {
	cases, err := loadGoldCases(cfg.GoldPath)
	if err != nil {
		return err
	}
	genCases := make([]GoldCase, 0, len(cases))
	for _, gc := range cases {
		if len(gc.GenerationPhrases) > 0 && gc.GenerationQ != "" {
			genCases = append(genCases, gc)
		}
	}
	if len(genCases) == 0 {
		return fmt.Errorf("no gold cases with generation_phrases and generation_q in %s", cfg.GoldPath)
	}

	client := &http.Client{Timeout: 5 * time.Minute}
	rows := make([]generationCaseRow, 0, len(genCases))
	var passSum float64

	for _, gc := range genCases {
		answer, err := callSearch(client, cfg.ServerURL, gc, cfg.SearchMode)
		if err != nil {
			return fmt.Errorf("%s: %w", gc.ID, err)
		}
		pass, missed := scoreGenerationPhrases(gc, answer)
		rows = append(rows, generationCaseRow{ID: gc.ID, Pass: pass, Missed: missed})
		if pass {
			passSum++
		}
	}

	n := float64(len(genCases))
	report := generationEvalReport{
		Set:      cfg.SetName,
		Server:   cfg.ServerURL,
		Cases:    len(genCases),
		PassRate: passSum / n,
		PerCase:  rows,
	}

	if cfg.OutputJSON != "" {
		if err := writeGenerationJSONReport(cfg.OutputJSON, report); err != nil {
			return err
		}
	}

	printGenerationSummary(report)
	if cfg.MinPass > 0 && report.PassRate < cfg.MinPass {
		return fmt.Errorf("generation pass rate %.3f below threshold %.3f", report.PassRate, cfg.MinPass)
	}
	return nil
}

func callSearch(client *http.Client, server string, gc GoldCase, searchMode string) (string, error) {
	params := url.Values{}
	params.Set("q", gc.GenerationQ)
	if gc.RetrievalQ != "" {
		params.Set("rq", gc.RetrievalQ)
	}
	params.Set("rewrite", "true")
	if gc.Corpus != "" {
		params.Set("corpus", gc.Corpus)
	}
	if searchMode != "" {
		if searchMode == "auto" {
			params.Set("level", "auto")
		} else {
			params.Set("mode", searchMode)
		}
	}

	u := strings.TrimRight(server, "/") + "/search?" + params.Encode()
	resp, err := client.Get(u)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
	}

	complete, _ := parseBenchSSE(string(body))
	return complete.Response, nil
}

func scoreGenerationPhrases(gc GoldCase, answer string) (pass bool, missed []string) {
	lower := strings.ToLower(answer)
	for _, phrase := range gc.GenerationPhrases {
		if !strings.Contains(lower, strings.ToLower(phrase)) {
			missed = append(missed, phrase)
		}
	}
	return len(missed) == 0, missed
}

func writeGenerationJSONReport(path string, report generationEvalReport) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func printGenerationSummary(r generationEvalReport) {
	fmt.Printf("set=%s generation_cases=%d pass_rate=%.3f\n", r.Set, r.Cases, r.PassRate)
	for _, row := range r.PerCase {
		mark := "FAIL"
		if row.Pass {
			mark = "PASS"
		}
		fmt.Printf("  %s %s", row.ID, mark)
		if len(row.Missed) > 0 {
			fmt.Printf(" missed=%v", row.Missed)
		}
		fmt.Println()
	}
}
