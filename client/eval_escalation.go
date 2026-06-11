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

type escalationEvalConfig struct {
	ServerURL  string
	GoldPath   string
	OutputJSON string
	SetName    string
}

type escalationCaseRow struct {
	ID               string `json:"id"`
	ResolvedLevel    string `json:"resolved_level"`
	EscalationReason string `json:"escalation_reason"`
	RequestedLevel   string `json:"requested_level"`
	Status           string `json:"status,omitempty"`
}

type escalationEvalReport struct {
	Set     string              `json:"set"`
	Server  string              `json:"server"`
	Cases   int                 `json:"cases"`
	PerCase []escalationCaseRow `json:"per_case"`
	Levels  map[string]int      `json:"levels"`
}

type searchEscalationResult struct {
	Status           string
	ResolvedLevel    string
	EscalationReason string
	RequestedLevel   string
}

// RunEscalationEval reports resolved search levels for level=auto routing.
func RunEscalationEval(cfg escalationEvalConfig) error {
	cases, err := loadGoldCases(cfg.GoldPath)
	if err != nil {
		return err
	}
	if len(cases) == 0 {
		return fmt.Errorf("no gold cases in %s", cfg.GoldPath)
	}

	client := &http.Client{Timeout: 5 * time.Minute}
	rows := make([]escalationCaseRow, 0, len(cases))
	levelCounts := map[string]int{}

	for _, gc := range cases {
		if gc.GenerationQ == "" {
			continue
		}
		result, err := callSearchEscalation(client, cfg.ServerURL, gc)
		if err != nil {
			return fmt.Errorf("%s: %w", gc.ID, err)
		}
		row := escalationCaseRow{
			ID:               gc.ID,
			ResolvedLevel:    result.ResolvedLevel,
			EscalationReason: result.EscalationReason,
			RequestedLevel:   result.RequestedLevel,
			Status:           result.Status,
		}
		rows = append(rows, row)
		levelKey := result.ResolvedLevel
		if levelKey == "" {
			levelKey = "unknown"
		}
		levelCounts[levelKey]++
	}

	report := escalationEvalReport{
		Set:     cfg.SetName,
		Server:  cfg.ServerURL,
		Cases:   len(rows),
		PerCase: rows,
		Levels:  levelCounts,
	}

	if cfg.OutputJSON != "" {
		if err := writeEscalationJSONReport(cfg.OutputJSON, report); err != nil {
			return err
		}
	}
	printEscalationSummary(report)
	return nil
}

func callSearchEscalation(client *http.Client, server string, gc GoldCase) (searchEscalationResult, error) {
	params := url.Values{}
	params.Set("q", gc.GenerationQ)
	if gc.RetrievalQ != "" {
		params.Set("rq", gc.RetrievalQ)
	}
	params.Set("rewrite", "true")
	params.Set("level", "auto")
	if gc.Corpus != "" {
		params.Set("corpus", gc.Corpus)
	}

	u := strings.TrimRight(server, "/") + "/search?" + params.Encode()
	resp, err := client.Get(u)
	if err != nil {
		return searchEscalationResult{}, err
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return searchEscalationResult{}, err
	}
	if resp.StatusCode != http.StatusOK {
		return searchEscalationResult{}, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
	}

	raw := string(body)
	if strings.HasPrefix(strings.TrimSpace(raw), "{") {
		var payload struct {
			Status string `json:"status"`
		}
		if json.Unmarshal(body, &payload) == nil && payload.Status == "no_results" {
			return searchEscalationResult{Status: "no_results", RequestedLevel: "auto"}, nil
		}
	}

	result := parseSearchEscalationSSE(raw)
	result.RequestedLevel = "auto"
	return result, nil
}

func parseSearchEscalationSSE(raw string) searchEscalationResult {
	var result searchEscalationResult
	scanner := bufio.NewScanner(strings.NewReader(raw))
	var event string
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "event:") {
			event = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
			continue
		}
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		switch event {
		case "escalation":
			var data struct {
				ResolvedLevel int    `json:"resolved_level"`
				Reason        string `json:"reason"`
			}
			if json.Unmarshal([]byte(payload), &data) == nil {
				if data.ResolvedLevel > 0 {
					result.ResolvedLevel = fmt.Sprintf("%d", data.ResolvedLevel)
				}
				if data.Reason != "" {
					result.EscalationReason = data.Reason
				}
			}
		case "complete":
			var complete struct {
				Metadata map[string]string `json:"metadata"`
			}
			if json.Unmarshal([]byte(payload), &complete) == nil {
				if v := complete.Metadata["search_level"]; v != "" {
					result.ResolvedLevel = v
				}
				if v := complete.Metadata["escalation_reason"]; v != "" {
					result.EscalationReason = v
				}
				if v := complete.Metadata["search_level_requested"]; v != "" {
					result.RequestedLevel = v
				}
			}
		}
	}
	result.Status = "ok"
	return result
}

func writeEscalationJSONReport(path string, report escalationEvalReport) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func printEscalationSummary(r escalationEvalReport) {
	fmt.Printf("set=%s escalation_cases=%d levels=%v\n", r.Set, r.Cases, r.Levels)
	for _, row := range r.PerCase {
		fmt.Printf("  %s level=%s reason=%s\n", row.ID, row.ResolvedLevel, row.EscalationReason)
	}
}
