package lexical

import (
	"github.com/blevesearch/bleve/v2"
	"github.com/blevesearch/bleve/v2/mapping"
	"github.com/blevesearch/bleve/v2/search/query"

	"github.com/noematic-eu/ai-rag-agent/model"
)

type bleveBackend struct {
	index bleve.Index
}

type bleveChunkDoc struct {
	Text     string `json:"text"`
	Title    string `json:"title"`
	DocTitle string `json:"doc_title"`
	Section  string `json:"section"`
	Article  string `json:"article"`
	DocID    string `json:"doc_id"`
	Corpus   string `json:"corpus"`
}

func newChunkIndexMapping() mapping.IndexMapping {
	docMapping := bleve.NewDocumentMapping()
	docMapping.AddFieldMappingsAt("text", bleve.NewTextFieldMapping())
	docMapping.AddFieldMappingsAt("title", bleve.NewTextFieldMapping())
	docMapping.AddFieldMappingsAt("doc_title", bleve.NewTextFieldMapping())
	docMapping.AddFieldMappingsAt("section", bleve.NewTextFieldMapping())
	docMapping.AddFieldMappingsAt("article", bleve.NewKeywordFieldMapping())
	docMapping.AddFieldMappingsAt("doc_id", bleve.NewKeywordFieldMapping())
	docMapping.AddFieldMappingsAt("corpus", bleve.NewKeywordFieldMapping())
	indexMapping := bleve.NewIndexMapping()
	indexMapping.AddDocumentMapping("_default", docMapping)
	return indexMapping
}

func openBleve(cfg Config) (Backend, error) {
	path := cfg.BlevePath()
	idx, err := bleve.New(path, newChunkIndexMapping())
	if err != nil {
		idx, err = bleve.Open(path)
		if err != nil {
			return nil, err
		}
	}
	return &bleveBackend{index: idx}, nil
}

func (b *bleveBackend) Engine() string { return EngineBleve }

func (b *bleveBackend) IndexChunk(chunk model.Chunk) error {
	f := FieldsFromChunk(chunk)
	doc := bleveChunkDoc{
		Text: f.Text, Title: f.Title, DocTitle: f.DocTitle,
		Section: f.Section, Article: f.Article, DocID: f.DocID, Corpus: f.Corpus,
	}
	return b.index.Index(f.ChunkID, doc)
}

func (b *bleveBackend) DeleteChunk(chunkID string) error {
	return b.index.Delete(chunkID)
}

func (b *bleveBackend) Search(text, corpus string, k int) ([]Hit, error) {
	q := buildBleveQuery(text, corpus)
	req := bleve.NewSearchRequest(q)
	req.Size = k
	res, err := b.index.Search(req)
	if err != nil {
		return nil, err
	}
	hits := make([]Hit, 0, len(res.Hits))
	for _, h := range res.Hits {
		hits = append(hits, Hit{ChunkID: h.ID, Score: h.Score})
	}
	return hits, nil
}

func (b *bleveBackend) Close() error {
	return b.index.Close()
}

func buildBleveQuery(queryText, corpus string) query.Query {
	textQ := bleve.NewMatchQuery(queryText)
	textQ.SetField("text")

	titleQ := bleve.NewMatchQuery(queryText)
	titleQ.SetField("title")
	titleQ.SetBoost(BoostTitle)

	docTitleQ := bleve.NewMatchQuery(queryText)
	docTitleQ.SetField("doc_title")
	docTitleQ.SetBoost(BoostDocTitle)

	sectionQ := bleve.NewMatchQuery(queryText)
	sectionQ.SetField("section")
	sectionQ.SetBoost(BoostSection)

	articleQ := bleve.NewMatchQuery(queryText)
	articleQ.SetField("article")
	articleQ.SetBoost(BoostArticle)

	contentQuery := bleve.NewDisjunctionQuery(textQ, titleQ, docTitleQ, sectionQ, articleQ)
	if corpus == "" {
		return contentQuery
	}
	corpusQuery := bleve.NewTermQuery(corpus)
	corpusQuery.SetField("corpus")
	booleanQuery := bleve.NewBooleanQuery()
	booleanQuery.AddMust(contentQuery)
	booleanQuery.AddMust(corpusQuery)
	return booleanQuery
}
