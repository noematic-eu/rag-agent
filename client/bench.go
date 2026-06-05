package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

var benchQueries = []string{
	"how to build a rag",
	"what are the steps of retrieval augmented generation",
	"how to evaluate a RAG system",
	"vector database embedding chunk size",
	"what is BM25 hybrid search",
	"how does chunk overlap work",
	"what is the capital of Mongolia",
	"who won the world cup in 1800",
	"comment construire un système RAG",
	"qu'est-ce que l'embedding",
}

type benchComplete struct {
	Response string `json:"response"`
	Metadata struct {
		Prompt string `json:"prompt"`
	} `json:"metadata"`
}

func RunBench(serverURL string) error {
	fmt.Println("query,latency_ms,response_len,refusal,doc_titles")
	client := &http.Client{Timeout: 5 * time.Minute}

	for _, q := range benchQueries {
		start := time.Now()
		encoded := url.QueryEscape(q)
		resp, err := client.Get(strings.TrimRight(serverURL, "/") + "/search?q=" + encoded)
		if err != nil {
			return err
		}

		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			return err
		}
		latency := time.Since(start).Milliseconds()

		if resp.StatusCode != http.StatusOK {
			fmt.Printf("%q,%d,0,error,http_%d\n", q, latency, resp.StatusCode)
			continue
		}

		complete, titles := parseBenchSSE(string(body))
		refusal := "no"
		lower := strings.ToLower(complete.Response)
		if strings.Contains(lower, "no information") ||
			strings.Contains(lower, "aucune information") ||
			strings.Contains(lower, "do not contain") ||
			strings.Contains(lower, "ne contiennent pas") {
			refusal = "yes"
		}

		fmt.Printf("%q,%d,%d,%s,%q\n", q, latency, len(complete.Response), refusal, strings.Join(titles, "; "))
	}

	return nil
}

func parseBenchSSE(raw string) (benchComplete, []string) {
	var complete benchComplete
	titles := []string{}

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
		if event == "complete" {
			_ = json.Unmarshal([]byte(payload), &complete)
		}
		if event == "metadata" {
			var meta struct {
				Prompt string `json:"prompt"`
			}
			if json.Unmarshal([]byte(payload), &meta) == nil {
				titles = extractDocTitles(meta.Prompt)
			}
		}
	}
	return complete, titles
}

func extractDocTitles(prompt string) []string {
	var titles []string
	for _, line := range strings.Split(prompt, "\n") {
		if strings.HasPrefix(line, "[") && strings.Contains(line, "title=") {
			start := strings.Index(line, "title=") + len("title=")
			titles = append(titles, strings.TrimSpace(line[start:]))
		}
	}
	return titles
}

func benchMain(args []string) {
	server := "http://localhost:8080"
	if len(args) > 0 {
		server = args[0]
	}
	if err := RunBench(server); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
