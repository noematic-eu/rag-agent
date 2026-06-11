package lexical

import (
	"encoding/json"
	"time"
)

type diskMeta struct {
	Version    int       `json:"version"`
	N          int       `json:"n"`
	AvgDL      float64   `json:"avg_dl"`
	ChunkCount int       `json:"chunk_count"`
	BuiltAt    time.Time `json:"built_at"`
}

func encodeDiskMeta(m diskMeta) ([]byte, error) {
	return json.Marshal(m)
}

func decodeDiskMeta(data []byte) (diskMeta, error) {
	var m diskMeta
	if len(data) == 0 {
		return m, nil
	}
	err := json.Unmarshal(data, &m)
	return m, err
}

type diskChunkMeta struct {
	ChunkID  string `json:"chunk_id"`
	DocID    string `json:"doc_id"`
	Corpus   string `json:"corpus"`
	LenText  int    `json:"len_text"`
	LenTitle int    `json:"len_title"`
	LenDoc   int    `json:"len_doc_title"`
	LenSec   int    `json:"len_section"`
	LenArt   int    `json:"len_article"`
}

func diskChunkMetaFromBM25(c BM25Chunk) diskChunkMeta {
	return diskChunkMeta{
		ChunkID:  c.Fields.ChunkID,
		DocID:    c.Fields.DocID,
		Corpus:   c.Fields.Corpus,
		LenText:  c.Length["text"],
		LenTitle: c.Length["title"],
		LenDoc:   c.Length["doc_title"],
		LenSec:   c.Length["section"],
		LenArt:   c.Length["article"],
	}
}

func encodeDiskChunkMeta(m diskChunkMeta) ([]byte, error) {
	return json.Marshal(m)
}

func decodeDiskChunkMeta(data []byte) (diskChunkMeta, error) {
	var m diskChunkMeta
	err := json.Unmarshal(data, &m)
	return m, err
}

func bm25ChunkFromDiskMeta(m diskChunkMeta, termFreq map[string]map[string]int) BM25Chunk {
	return BM25Chunk{
		Fields: ChunkFields{
			ChunkID: m.ChunkID,
			DocID:   m.DocID,
			Corpus:  m.Corpus,
		},
		TermFreq: termFreq,
		Length: map[string]int{
			"text":      m.LenText,
			"title":     m.LenTitle,
			"doc_title": m.LenDoc,
			"section":   m.LenSec,
			"article":   m.LenArt,
		},
	}
}
