//go:build tantivy

package lexical

import (
	"os"
	"sync"

	tantivy "github.com/anyproto/tantivy-go"

	"github.com/noematic-eu/ai-rag-agent/model"
)

const (
	tantivyFieldChunkID  = "chunk_id"
	tantivyFieldText     = "text"
	tantivyFieldTitle    = "title"
	tantivyFieldDocTitle = "doc_title"
	tantivyFieldSection  = "section"
	tantivyFieldCorpus   = "corpus"
)

var tantivyLibOnce sync.Once
var tantivyLibErr error

type tantivyBackend struct {
	ctx           *tantivy.TantivyContext
	path          string
	schema        *tantivy.Schema
	corpusByChunk map[string]string
}

func initTantivyLib() error {
	tantivyLibOnce.Do(func() {
		tantivyLibErr = tantivy.LibInit(true, true, "warn")
	})
	return tantivyLibErr
}

func openTantivy(cfg Config) (Backend, error) {
	if err := initTantivyLib(); err != nil {
		return nil, err
	}
	schema, err := buildTantivySchema()
	if err != nil {
		return nil, err
	}
	path := cfg.TantivyPath()
	ctx, err := tantivy.NewTantivyContextWithSchema(path, schema)
	if err != nil {
		return nil, err
	}
	if err := registerTantivyAnalyzers(ctx); err != nil {
		ctx.Free()
		return nil, err
	}
	b := &tantivyBackend{
		ctx: ctx, path: path, schema: schema,
		corpusByChunk: make(map[string]string),
	}
	if cfg.ScanChunks != nil {
		_ = cfg.ScanChunks(func(chunk model.Chunk) error {
			if chunk.Metadata.ChunkID != "" {
				b.corpusByChunk[chunk.Metadata.ChunkID] = chunk.Metadata.Corpus
			}
			return nil
		})
	}
	return b, nil
}

func buildTantivySchema() (*tantivy.Schema, error) {
	builder, err := tantivy.NewSchemaBuilder()
	if err != nil {
		return nil, err
	}
	textOpts := []struct {
		name string
	}{
		{tantivyFieldText}, {tantivyFieldTitle}, {tantivyFieldDocTitle}, {tantivyFieldSection},
	}
	for _, f := range textOpts {
		if err := builder.AddTextField(
			f.name, true, true, false,
			tantivy.IndexRecordOptionWithFreqs,
			tantivy.TokenizerSimple,
		); err != nil {
			return nil, err
		}
	}
	for _, name := range []string{tantivyFieldChunkID, tantivyFieldCorpus} {
		if err := builder.AddTextField(
			name, true, false, true,
			tantivy.IndexRecordOptionBasic,
			tantivy.TokenizerRaw,
		); err != nil {
			return nil, err
		}
	}
	return builder.BuildSchema()
}

func registerTantivyAnalyzers(ctx *tantivy.TantivyContext) error {
	if err := ctx.RegisterTextAnalyzerSimple(tantivy.TokenizerSimple, 40, tantivy.English); err != nil {
		return err
	}
	if err := ctx.RegisterTextAnalyzerRaw(tantivy.TokenizerRaw); err != nil {
		return err
	}
	return nil
}

func (b *tantivyBackend) Engine() string { return EngineTantivy }

func (b *tantivyBackend) IndexChunk(chunk model.Chunk) error {
	f := FieldsFromChunk(chunk)
	doc := tantivy.NewDocument()
	if doc == nil {
		return os.ErrInvalid
	}
	fields := map[string]string{
		tantivyFieldChunkID:  f.ChunkID,
		tantivyFieldText:     f.Text,
		tantivyFieldTitle:    f.Title,
		tantivyFieldDocTitle: f.DocTitle,
		tantivyFieldSection:  f.Section,
		tantivyFieldCorpus:   f.Corpus,
	}
	for name, val := range fields {
		if err := doc.AddField(val, b.ctx, name); err != nil {
			return err
		}
	}
	if err := b.ctx.AddAndConsumeDocuments(doc); err != nil {
		return err
	}
	b.corpusByChunk[f.ChunkID] = f.Corpus
	return nil
}

func (b *tantivyBackend) DeleteChunk(chunkID string) error {
	delete(b.corpusByChunk, chunkID)
	return b.ctx.DeleteDocuments(tantivyFieldChunkID, chunkID)
}

func (b *tantivyBackend) Search(text, corpus string, k int) ([]Hit, error) {
	if k <= 0 {
		return nil, nil
	}
	limit := k
	if corpus != "" {
		limit = k * 20
		if limit < 100 {
			limit = 100
		}
	}
	sCtx := tantivy.NewSearchContextBuilder().
		SetQuery(text).
		SetDocsLimit(uintptr(limit)).
		SetWithHighlights(false).
		AddField(tantivyFieldText, float32(BoostText)).
		AddField(tantivyFieldTitle, float32(BoostTitle)).
		AddField(tantivyFieldDocTitle, float32(BoostDocTitle)).
		AddField(tantivyFieldSection, float32(BoostSection)).
		Build()

	result, err := b.ctx.SearchFastField(sCtx, tantivyFieldChunkID)
	if err != nil {
		return nil, err
	}
	var hits []Hit
	for i, id := range result.Values {
		if corpus != "" && b.corpusByChunk[id] != corpus {
			continue
		}
		score := 0.0
		if i < len(result.Scores) {
			score = float64(result.Scores[i])
		}
		hits = append(hits, Hit{ChunkID: id, Score: score})
		if len(hits) >= k {
			break
		}
	}
	return hits, nil
}

func (b *tantivyBackend) Close() error {
	if b.ctx != nil {
		return b.ctx.Close()
	}
	return nil
}
