package model

// RetrieveHit is a single ranked chunk from GET /retrieve.
type RetrieveHit struct {
	ChunkID     string  `json:"chunk_id"`
	DocID       string  `json:"doc_id"`
	Score       float64 `json:"score"`
	Corpus      string  `json:"corpus,omitempty"`
	Section     string  `json:"section,omitempty"`
	DocTitle    string  `json:"doc_title,omitempty"`
	SectionPath string  `json:"section_path,omitempty"`
	Article     string  `json:"article,omitempty"`
	Excerpt     string  `json:"excerpt,omitempty"`
}

// RetrieveResponse is the JSON body for GET /retrieve.
type RetrieveResponse struct {
	Status string        `json:"status"`
	Hits   []RetrieveHit `json:"hits,omitempty"`
	Query  string        `json:"query,omitempty"`
}
