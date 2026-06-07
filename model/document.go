package model

// LegalDocument représente un document juridique (legacy, kept for backward compatibility)
type LegalDocument struct {
	ID              string `json:"id"`
	Title           string `json:"title"`
	Content         string `json:"content"`                    // Canonical markdown content
	ContentType     string `json:"content_type,omitempty"`     // markdown | html
	OriginalContent string `json:"original_content,omitempty"` // Optional source payload (ex: original html)
	Corpus          string `json:"corpus,omitempty"`           // Optional corpus tag for scoped search
	BookTitle       string `json:"book_title,omitempty"`       // Parent document title (filename at ingest)
	Article         string `json:"article,omitempty"`          // Legal article number when chunk maps to one article
}

// ChunkMetadata contient les métadonnées d'un chunk
type ChunkMetadata struct {
	DocID       string `json:"doc_id"`               // ID du document parent
	ChunkID     string `json:"chunk_id"`             // ID unique du chunk
	Title       string `json:"title"`                // Section heading within the document
	DocTitle    string `json:"doc_title,omitempty"`  // Parent document title (ingest filename)
	SectionPath string `json:"section_path"`         // Chemin de section (ex: "Chapitre I -> Section 1")
	Source      string `json:"source"`               // Source du chunk (ex: "markdown", "html")
	Position    int    `json:"position"`             // Position du chunk dans le document
	LineRange   string `json:"line_range,omitempty"` // Plage de lignes (ex: "10-25")
	Corpus      string `json:"corpus,omitempty"`     // Optional corpus tag (inherited from parent doc)
	Article     string `json:"article,omitempty"`    // Article number when chunk is one legal article (ex: "16", "1")
}

// Chunk représente un chunk indexé avec son texte et ses métadonnées
type Chunk struct {
	Metadata  ChunkMetadata `json:"metadata"`
	Text      string        `json:"text"`                // Texte du chunk
	Embedding []float64     `json:"embedding,omitempty"` // Embedding vector (optionnel, pour recherche sémantique)
	Original  string        `json:"original,omitempty"`  // Contenu original (Markdown ou HTML) pour le prompt
}

// DocumentChunkingStats contient les statistiques de chunking d'un document
type DocumentChunkingStats struct {
	DocID        string  `json:"doc_id"`
	TotalChunks  int     `json:"total_chunks"`
	TokenCount   int     `json:"token_count,omitempty"`
	ChunkSizeAvg float64 `json:"chunk_size_avg,omitempty"`
}

// ChunkSearchResult représente un résultat de recherche de chunk
type ChunkSearchResult struct {
	Chunk      Chunk    `json:"chunk"`
	Score      float64  `json:"score"`                 // Score de similarité
	Source     string   `json:"source"`                // Source du score ("bm25", "vector", "hybrid")
	MatchTerms []string `json:"match_terms,omitempty"` // Termes correspondants
}
