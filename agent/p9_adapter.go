package main

import (
	"encoding/json"
	"fmt"

	docmodel "github.com/noematic-eu/ai-rag-agent/model"
)

type p9Service struct {
	agent *Agent
}

func newP9Service(agent *Agent) *p9Service {
	return &p9Service{agent: agent}
}

func (s *p9Service) StatsJSON() ([]byte, error) {
	snapshot := s.agent.Stats()
	data, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}

func (s *p9Service) RunCtl(command string) (string, error) {
	return s.agent.RunCtl(command)
}

func (s *p9Service) IngestJSON(data []byte) (string, error) {
	var doc docmodel.LegalDocument
	if err := json.Unmarshal(data, &doc); err != nil {
		return "", fmt.Errorf("invalid ingest JSON: %w", err)
	}
	result, err := s.agent.Ingest(doc)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("ok %d chunks", result.Chunks), nil
}

func (s *p9Service) Retrieve(query string, params map[string]string) ([]byte, error) {
	opts := parseRankOptionsFromParams(query, "", params)
	resp, err := s.agent.Retrieve(opts)
	if err != nil {
		return nil, err
	}
	data, err := json.MarshalIndent(resp, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}

func (s *p9Service) Search(query string, params map[string]string) (string, []byte, error) {
	opts := parseRankOptionsFromParams("", query, params)
	result, err := s.agent.Search(opts, query)
	if err != nil {
		return "", nil, err
	}
	if result.Status == "no_results" {
		return result.Message, nil, nil
	}
	meta, err := json.MarshalIndent(result.Metadata, "", "  ")
	if err != nil {
		return result.Answer, nil, err
	}
	return result.Answer, append(meta, '\n'), nil
}

func (s *p9Service) DeleteDocument(docID string) error {
	_, err := s.agent.DeleteDocument(docID)
	return err
}
