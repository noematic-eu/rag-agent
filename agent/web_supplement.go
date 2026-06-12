package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/noematic-eu/ai-rag-agent/model"
)

const (
	webUserAgent         = "ai-rag-agent/1.0 (https://github.com/noematic-eu/ai-rag-agent)"
	defaultWebMaxPages   = 3
	defaultWebMaxQueries = 2
	maxWebPageChars      = 4000
	webHTTPTimeout       = 10 * time.Second
)

type webGapReason string

const (
	webGapNoResults         webGapReason = "no_results"
	webGapCRAGInsufficient  webGapReason = "crag_insufficient"
	webGapAgentInsufficient webGapReason = "agent_insufficient"
)

type webSupplementConfig struct {
	enabled        bool
	provider       string
	apiKey         string
	maxPages       int
	maxQueries     int
	fetchAllowlist []string
}

type webSupplementOutcome struct {
	docs     []model.LegalDocument
	provider string
	queries  []string
	urls     []string
}

var webHTTPClient = &http.Client{Timeout: webHTTPTimeout}

func webSupplementConfigFromEnv() webSupplementConfig {
	cfg := webSupplementConfig{
		enabled:    parseBool(os.Getenv("RAG_WEB_SUPPLEMENT")),
		provider:   strings.ToLower(strings.TrimSpace(envOr("RAG_WEB_SEARCH_PROVIDER", "tavily"))),
		apiKey:     strings.TrimSpace(os.Getenv("RAG_WEB_SEARCH_API_KEY")),
		maxPages:   defaultWebMaxPages,
		maxQueries: defaultWebMaxQueries,
	}
	if v := strings.TrimSpace(os.Getenv("RAG_WEB_MAX_PAGES")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			cfg.maxPages = n
		}
	}
	if v := strings.TrimSpace(os.Getenv("RAG_WEB_MAX_QUERIES")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			cfg.maxQueries = n
		}
	}
	if raw := strings.TrimSpace(os.Getenv("RAG_WEB_FETCH_ALLOWLIST")); raw != "" {
		for _, part := range strings.Split(raw, ",") {
			if h := strings.TrimSpace(part); h != "" {
				cfg.fetchAllowlist = append(cfg.fetchAllowlist, strings.ToLower(h))
			}
		}
	}
	return cfg
}

func wikiLangCode(lang string) string {
	if lang == "fr" {
		return "fr"
	}
	return "en"
}

func truncateWebText(s string, max int) string {
	s = strings.TrimSpace(s)
	if max <= 0 || len(s) <= max {
		return s
	}
	return strings.TrimSpace(s[:max]) + "…"
}

type wikiSearchResponse struct {
	Query struct {
		Search []struct {
			Title   string `json:"title"`
			PageID  int    `json:"pageid"`
			Snippet string `json:"snippet"`
		} `json:"search"`
	} `json:"query"`
}

type wikiExtractResponse struct {
	Query struct {
		Pages map[string]struct {
			PageID  int    `json:"pageid"`
			Title   string `json:"title"`
			Extract string `json:"extract"`
			Missing bool   `json:"missing"`
		} `json:"pages"`
	} `json:"query"`
}

var (
	wikiAPIBaseOverride string
	tavilyAPIURL        = "https://api.tavily.com/search"
	braveAPIURL         = "https://api.search.brave.com/res/v1/web/search"
)

func wikipediaAPIBase(lang string) string {
	if wikiAPIBaseOverride != "" {
		return wikiAPIBaseOverride
	}
	return fmt.Sprintf("https://%s.wikipedia.org/w/api.php", wikiLangCode(lang))
}

func searchWikipedia(ctx context.Context, lang, query string, limit int) ([]model.LegalDocument, error) {
	if limit <= 0 {
		return nil, nil
	}
	params := url.Values{}
	params.Set("action", "query")
	params.Set("list", "search")
	params.Set("srsearch", query)
	params.Set("srlimit", strconv.Itoa(limit))
	params.Set("format", "json")

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, wikipediaAPIBase(lang)+"?"+params.Encode(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", webUserAgent)

	resp, err := webHTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("wikipedia search status %d: %s", resp.StatusCode, truncateWebText(string(body), 200))
	}

	var parsed wikiSearchResponse
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return nil, err
	}
	if len(parsed.Query.Search) == 0 {
		return nil, nil
	}

	titles := make([]string, 0, len(parsed.Query.Search))
	for _, hit := range parsed.Query.Search {
		titles = append(titles, hit.Title)
	}

	extractParams := url.Values{}
	extractParams.Set("action", "query")
	extractParams.Set("prop", "extracts")
	extractParams.Set("explaintext", "1")
	extractParams.Set("exintro", "1")
	extractParams.Set("titles", strings.Join(titles, "|"))
	extractParams.Set("format", "json")

	extractReq, err := http.NewRequestWithContext(ctx, http.MethodGet, wikipediaAPIBase(lang)+"?"+extractParams.Encode(), nil)
	if err != nil {
		return nil, err
	}
	extractReq.Header.Set("User-Agent", webUserAgent)

	extractResp, err := webHTTPClient.Do(extractReq)
	if err != nil {
		return nil, err
	}
	defer func() { _ = extractResp.Body.Close() }()
	if extractResp.StatusCode < 200 || extractResp.StatusCode >= 300 {
		body, _ := io.ReadAll(extractResp.Body)
		return nil, fmt.Errorf("wikipedia extract status %d: %s", extractResp.StatusCode, truncateWebText(string(body), 200))
	}

	var extracts wikiExtractResponse
	if err := json.NewDecoder(extractResp.Body).Decode(&extracts); err != nil {
		return nil, err
	}

	docs := make([]model.LegalDocument, 0, len(extracts.Query.Pages))
	wikiHost := wikiLangCode(lang) + ".wikipedia.org"
	for _, page := range extracts.Query.Pages {
		if page.Missing || strings.TrimSpace(page.Extract) == "" {
			continue
		}
		pageURL := fmt.Sprintf("https://%s/wiki/%s", wikiHost, url.PathEscape(strings.ReplaceAll(page.Title, " ", "_")))
		docs = append(docs, model.LegalDocument{
			ID:        fmt.Sprintf("web::wikipedia::%d", page.PageID),
			Title:     page.Title,
			BookTitle: pageURL,
			Corpus:    "web",
			Content:   truncateWebText(page.Extract, maxWebPageChars),
		})
	}
	return docs, nil
}

type tavilySearchRequest struct {
	APIKey            string `json:"api_key"`
	Query             string `json:"query"`
	SearchDepth       string `json:"search_depth"`
	MaxResults        int    `json:"max_results"`
	IncludeRawContent bool   `json:"include_raw_content"`
}

type tavilySearchResponse struct {
	Results []struct {
		Title      string `json:"title"`
		URL        string `json:"url"`
		Content    string `json:"content"`
		RawContent string `json:"raw_content"`
	} `json:"results"`
}

func searchTavily(ctx context.Context, cfg webSupplementConfig, query string, limit int) ([]model.LegalDocument, error) {
	if cfg.apiKey == "" {
		return nil, fmt.Errorf("tavily: missing RAG_WEB_SEARCH_API_KEY")
	}
	body, err := json.Marshal(tavilySearchRequest{
		APIKey:            cfg.apiKey,
		Query:             query,
		SearchDepth:       "basic",
		MaxResults:        limit,
		IncludeRawContent: true,
	})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tavilyAPIURL, strings.NewReader(string(body)))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", webUserAgent)

	resp, err := webHTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		raw, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("tavily status %d: %s", resp.StatusCode, truncateWebText(string(raw), 200))
	}

	var parsed tavilySearchResponse
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return nil, err
	}
	docs := make([]model.LegalDocument, 0, len(parsed.Results))
	for _, r := range parsed.Results {
		text := strings.TrimSpace(r.RawContent)
		if text == "" {
			text = strings.TrimSpace(r.Content)
		}
		title := strings.TrimSpace(r.Title)
		pageURL := strings.TrimSpace(r.URL)
		if title == "" || pageURL == "" || text == "" {
			continue
		}
		if !urlFetchAllowed(pageURL, cfg.fetchAllowlist) {
			continue
		}
		docs = append(docs, webDocumentFromURL(title, pageURL, text))
	}
	return docs, nil
}

type braveSearchResponse struct {
	Web struct {
		Results []struct {
			Title       string `json:"title"`
			URL         string `json:"url"`
			Description string `json:"description"`
		} `json:"results"`
	} `json:"web"`
}

func searchBrave(ctx context.Context, cfg webSupplementConfig, query string, limit int) ([]model.LegalDocument, error) {
	if cfg.apiKey == "" {
		return nil, fmt.Errorf("brave: missing RAG_WEB_SEARCH_API_KEY")
	}
	params := url.Values{}
	params.Set("q", query)
	params.Set("count", strconv.Itoa(limit))

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, braveAPIURL+"?"+params.Encode(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-Subscription-Token", cfg.apiKey)
	req.Header.Set("User-Agent", webUserAgent)

	resp, err := webHTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		raw, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("brave status %d: %s", resp.StatusCode, truncateWebText(string(raw), 200))
	}

	var parsed braveSearchResponse
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return nil, err
	}
	return webDocsFromBraveResults(parsed.Web.Results, cfg)
}

func webDocsFromBraveResults(results []struct {
	Title       string `json:"title"`
	URL         string `json:"url"`
	Description string `json:"description"`
}, cfg webSupplementConfig) ([]model.LegalDocument, error) {
	docs := make([]model.LegalDocument, 0, len(results))
	for _, r := range results {
		title := strings.TrimSpace(r.Title)
		pageURL := strings.TrimSpace(r.URL)
		text := strings.TrimSpace(r.Description)
		if title == "" || pageURL == "" {
			continue
		}
		if !urlFetchAllowed(pageURL, cfg.fetchAllowlist) {
			continue
		}
		if len(text) < 200 {
			if fetched, err := fetchURLText(context.Background(), pageURL, cfg); err == nil && fetched != "" {
				text = fetched
			}
		}
		if text == "" {
			continue
		}
		docs = append(docs, webDocumentFromURL(title, pageURL, text))
	}
	return docs, nil
}

func webDocumentFromURL(title, pageURL, text string) model.LegalDocument {
	id := "web::url::" + strings.ReplaceAll(url.QueryEscape(pageURL), "%", "_")
	return model.LegalDocument{
		ID:        id,
		Title:     title,
		BookTitle: pageURL,
		Corpus:    "web",
		Content:   truncateWebText(text, maxWebPageChars),
	}
}

func urlFetchAllowed(rawURL string, allowlist []string) bool {
	u, err := url.Parse(rawURL)
	if err != nil || u.Scheme != "https" || u.Host == "" {
		return false
	}
	host := strings.ToLower(u.Hostname())
	if isPrivateOrLocalHost(host) {
		return false
	}
	if len(allowlist) == 0 {
		return true
	}
	for _, allowed := range allowlist {
		if host == allowed || strings.HasSuffix(host, "."+allowed) {
			return true
		}
	}
	return false
}

func isPrivateOrLocalHost(host string) bool {
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return false
	}
	return ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast()
}

func fetchURLText(ctx context.Context, rawURL string, cfg webSupplementConfig) (string, error) {
	if !urlFetchAllowed(rawURL, cfg.fetchAllowlist) {
		return "", fmt.Errorf("url not allowed: %s", rawURL)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", webUserAgent)

	resp, err := webHTTPClient.Do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("fetch status %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 256*1024))
	if err != nil {
		return "", err
	}
	return truncateWebText(stripHTMLTags(string(body)), maxWebPageChars), nil
}

func stripHTMLTags(s string) string {
	var b strings.Builder
	inTag := false
	for _, r := range s {
		switch {
		case r == '<':
			inTag = true
		case r == '>':
			inTag = false
		case !inTag:
			b.WriteRune(r)
		}
	}
	return strings.Join(strings.Fields(b.String()), " ")
}

func searchWebAPI(ctx context.Context, cfg webSupplementConfig, query string, limit int) ([]model.LegalDocument, error) {
	switch cfg.provider {
	case "brave":
		return searchBrave(ctx, cfg, query, limit)
	default:
		return searchTavily(ctx, cfg, query, limit)
	}
}

func webQueriesForSupplement(mainQuery string, followUp []string, maxQueries int) []string {
	queries := make([]string, 0, maxQueries)
	seen := make(map[string]struct{})
	add := func(q string) {
		q = strings.TrimSpace(q)
		if q == "" {
			return
		}
		key := strings.ToLower(q)
		if _, ok := seen[key]; ok {
			return
		}
		seen[key] = struct{}{}
		queries = append(queries, q)
	}
	for _, q := range followUp {
		if len(queries) >= maxQueries {
			break
		}
		add(q)
	}
	if len(queries) < maxQueries {
		add(mainQuery)
	}
	return queries
}

func mergeWebDocs(existing []model.LegalDocument, webDocs []model.LegalDocument, maxPages int) []model.LegalDocument {
	seen := make(map[string]struct{}, len(existing)+len(webDocs))
	out := make([]model.LegalDocument, 0, len(existing)+maxPages)
	for _, doc := range existing {
		key := doc.ID
		if key == "" {
			key = doc.Title + doc.BookTitle
		}
		seen[key] = struct{}{}
		out = append(out, doc)
	}
	added := 0
	for _, doc := range webDocs {
		if added >= maxPages {
			break
		}
		key := doc.ID
		if key == "" {
			key = doc.Title + doc.BookTitle
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, doc)
		added++
	}
	return out
}

func runWebSupplement(ctx context.Context, generationQuery, lang string, followUp []string) webSupplementOutcome {
	cfg := webSupplementConfigFromEnv()
	if !cfg.enabled {
		return webSupplementOutcome{}
	}

	queries := webQueriesForSupplement(generationQuery, followUp, cfg.maxQueries)
	if len(queries) == 0 {
		return webSupplementOutcome{}
	}

	var collected []model.LegalDocument
	provider := ""
	for _, q := range queries {
		if len(collected) >= cfg.maxPages {
			break
		}
		remaining := cfg.maxPages - len(collected)
		docs, err := searchWikipedia(ctx, lang, q, remaining)
		if err != nil {
			log.Printf("web supplement: wikipedia search failed for %q: %v", q, err)
			continue
		}
		if len(docs) > 0 {
			provider = "wikipedia"
		}
		collected = append(collected, docs...)
	}

	if len(collected) == 0 {
		apiDocs, err := searchWebAPI(ctx, cfg, generationQuery, cfg.maxPages)
		if err != nil {
			log.Printf("web supplement: search API failed: %v", err)
		} else if len(apiDocs) > 0 {
			collected = apiDocs
			provider = cfg.provider
		}
	}

	if len(collected) > cfg.maxPages {
		collected = collected[:cfg.maxPages]
	}

	urls := make([]string, 0, len(collected))
	for _, doc := range collected {
		if u := strings.TrimSpace(doc.BookTitle); u != "" {
			urls = append(urls, u)
		}
	}

	return webSupplementOutcome{
		docs:     collected,
		provider: provider,
		queries:  queries,
		urls:     urls,
	}
}

func applyWebSupplementMeta(extraMeta map[string]string, reason webGapReason, outcome webSupplementOutcome) {
	if extraMeta == nil {
		return
	}
	if len(outcome.docs) == 0 {
		extraMeta["web_supplement"] = "false"
		return
	}
	extraMeta["web_supplement"] = "true"
	extraMeta["web_gap_reason"] = string(reason)
	if outcome.provider != "" {
		extraMeta["web_provider"] = outcome.provider
	}
	if len(outcome.urls) > 0 {
		extraMeta["web_urls"] = strings.Join(outcome.urls, " | ")
	}
}

func emitWebSupplementEvent(w StreamWriter, reason webGapReason, outcome webSupplementOutcome) {
	if w == nil || len(outcome.docs) == 0 {
		return
	}
	_ = w.WriteAgentEvent("web_supplement", map[string]interface{}{
		"gap_reason": string(reason),
		"provider":   outcome.provider,
		"queries":    outcome.queries,
		"pages":      outcome.docsToTitles(),
		"urls":       outcome.urls,
	})
}

func (o webSupplementOutcome) docsToTitles() []string {
	titles := make([]string, 0, len(o.docs))
	for _, doc := range o.docs {
		titles = append(titles, doc.Title)
	}
	return titles
}

func maybeApplyWebSupplement(
	ctx context.Context,
	generationQuery, lang string,
	reason webGapReason,
	followUp []string,
	docs []model.LegalDocument,
	extraMeta map[string]string,
	w StreamWriter,
) []model.LegalDocument {
	cfg := webSupplementConfigFromEnv()
	if !cfg.enabled {
		return docs
	}

	outcome := runWebSupplement(ctx, generationQuery, lang, followUp)
	if len(outcome.docs) == 0 {
		applyWebSupplementMeta(extraMeta, reason, outcome)
		return docs
	}

	merged := mergeWebDocs(docs, outcome.docs, cfg.maxPages)
	applyWebSupplementMeta(extraMeta, reason, outcome)
	emitWebSupplementEvent(w, reason, outcome)
	return merged
}

func detectWebGapAfterCRAG(cragTrace cragTrace, cragEnabled bool) (webGapReason, bool) {
	if !cragEnabled {
		return "", false
	}
	if !cragTrace.Sufficient {
		return webGapCRAGInsufficient, true
	}
	return "", false
}

func detectWebGapAfterAgent(ctx context.Context, generationQuery, lang string, docs []model.LegalDocument, topK int) (webGapReason, bool) {
	if len(docs) == 0 {
		return webGapAgentInsufficient, true
	}
	grade := gradeRetrievalContext(ctx, generationQuery, lang, docs, topK)
	if !grade.Sufficient {
		return webGapAgentInsufficient, true
	}
	return "", false
}
