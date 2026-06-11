package lexical

import (
	"encoding/binary"
	"io"
)

type diskPostingEntry struct {
	ChunkID string
	Corpus  string
	TFText  uint8
	TFTitle uint8
	TFDoc   uint8
	TFSec   uint8
	TFArt   uint8
}

func diskPostingFromBM25(chunkID, corpus, term string, termFreq map[string]map[string]int) diskPostingEntry {
	fieldTF := func(field string) uint8 {
		if termFreq[field] == nil {
			return 0
		}
		n := termFreq[field][term]
		if n > 255 {
			return 255
		}
		return uint8(n)
	}
	return diskPostingEntry{
		ChunkID: chunkID,
		Corpus:  corpus,
		TFText:  fieldTF("text"),
		TFTitle: fieldTF("title"),
		TFDoc:   fieldTF("doc_title"),
		TFSec:   fieldTF("section"),
		TFArt:   fieldTF("article"),
	}
}

func encodeDiskPostingList(entries []diskPostingEntry) []byte {
	buf := make([]byte, 4)
	binary.BigEndian.PutUint32(buf, uint32(len(entries)))
	for _, e := range entries {
		buf = append(buf, encodeDiskPostingEntry(e)...)
	}
	return buf
}

func encodeDiskPostingEntry(e diskPostingEntry) []byte {
	var buf []byte
	buf = diskAppendLenPrefixed(buf, e.ChunkID)
	buf = diskAppendLenPrefixed(buf, e.Corpus)
	buf = append(buf, e.TFText, e.TFTitle, e.TFDoc, e.TFSec, e.TFArt)
	return buf
}

func decodeDiskPostingList(data []byte) ([]diskPostingEntry, error) {
	if len(data) < 4 {
		return nil, nil
	}
	count := int(binary.BigEndian.Uint32(data[:4]))
	entries := make([]diskPostingEntry, 0, count)
	off := 4
	for i := 0; i < count; i++ {
		e, n, err := decodeDiskPostingEntry(data[off:])
		if err != nil {
			return nil, err
		}
		entries = append(entries, e)
		off += n
	}
	return entries, nil
}

func decodeDiskPostingEntry(data []byte) (diskPostingEntry, int, error) {
	var e diskPostingEntry
	off := 0
	var n int
	var err error

	e.ChunkID, n, err = diskReadLenPrefixed(data[off:])
	if err != nil {
		return e, 0, err
	}
	off += n

	e.Corpus, n, err = diskReadLenPrefixed(data[off:])
	if err != nil {
		return e, 0, err
	}
	off += n

	if len(data[off:]) < 5 {
		return e, 0, io.ErrUnexpectedEOF
	}
	e.TFText = data[off]
	e.TFTitle = data[off+1]
	e.TFDoc = data[off+2]
	e.TFSec = data[off+3]
	e.TFArt = data[off+4]
	off += 5
	return e, off, nil
}

func diskAppendLenPrefixed(buf []byte, s string) []byte {
	b := []byte(s)
	lb := make([]byte, 2)
	binary.BigEndian.PutUint16(lb, uint16(len(b)))
	return append(append(buf, lb...), b...)
}

func diskReadLenPrefixed(data []byte) (string, int, error) {
	if len(data) < 2 {
		return "", 0, io.ErrUnexpectedEOF
	}
	l := int(binary.BigEndian.Uint16(data[:2]))
	if len(data) < 2+l {
		return "", 0, io.ErrUnexpectedEOF
	}
	return string(data[2 : 2+l]), 2 + l, nil
}

func diskMergeTermFreq(termFreq map[string]map[string]int, term string, e diskPostingEntry) {
	set := func(field string, tf uint8) {
		if tf == 0 {
			return
		}
		if termFreq[field] == nil {
			termFreq[field] = make(map[string]int)
		}
		termFreq[field][term] = int(tf)
	}
	set("text", e.TFText)
	set("title", e.TFTitle)
	set("doc_title", e.TFDoc)
	set("section", e.TFSec)
	set("article", e.TFArt)
}

func removeDiskPostingEntry(entries []diskPostingEntry, chunkID string) ([]diskPostingEntry, bool) {
	out := entries[:0]
	removed := false
	for _, e := range entries {
		if e.ChunkID == chunkID {
			removed = true
			continue
		}
		out = append(out, e)
	}
	return out, removed
}
