package p9fs

// Service is the RAG operations surface exposed through the 9P file tree.
type Service interface {
	StatsJSON() ([]byte, error)
	RunCtl(command string) (string, error)
	IngestJSON(data []byte) (string, error)
	Retrieve(query string, params map[string]string) ([]byte, error)
	Search(query string, params map[string]string) (answer string, metadataJSON []byte, err error)
	DeleteDocument(docID string) error
}
