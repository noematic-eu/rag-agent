package lexical

import (
	"encoding/binary"
	"encoding/json"
	"log"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/noematic-eu/ai-rag-agent/model"
)

type diskIndex struct {
	kv         KV
	mu         sync.RWMutex
	rebuilding atomic.Bool
}

func newDiskIndex(kv KV) *diskIndex {
	return &diskIndex{kv: kv}
}

func (idx *diskIndex) loadMeta() (diskMeta, error) {
	data, err := diskGetOptional(idx.kv, diskPrefixMeta)
	if err != nil {
		return diskMeta{}, err
	}
	return decodeDiskMeta(data)
}

func (idx *diskIndex) saveMeta(m diskMeta) error {
	data, err := encodeDiskMeta(m)
	if err != nil {
		return err
	}
	return idx.kv.Put(diskPrefixMeta, data)
}

func (idx *diskIndex) registerChunkMeta(m diskMeta, c BM25Chunk) diskMeta {
	m.N++
	var totalLen float64
	for _, field := range []string{"text", "title", "doc_title", "section"} {
		totalLen += float64(c.Length[field])
	}
	if m.N == 1 {
		m.AvgDL = totalLen
	} else {
		m.AvgDL = m.AvgDL + (totalLen-m.AvgDL)/float64(m.N)
	}
	m.ChunkCount = m.N
	return m
}

func (idx *diskIndex) unregisterChunkMeta(m diskMeta, c BM25Chunk) diskMeta {
	if m.N <= 0 {
		return m
	}
	m.N--
	var totalLen float64
	for _, field := range []string{"text", "title", "doc_title", "section"} {
		totalLen += float64(c.Length[field])
	}
	if m.N == 0 {
		m.AvgDL = 0
	} else {
		m.AvgDL = (m.AvgDL*float64(m.N+1) - totalLen) / float64(m.N)
	}
	m.ChunkCount = m.N
	return m
}

func (idx *diskIndex) IndexChunk(chunk ChunkFields) error {
	if chunk.ChunkID == "" || chunk.Text == "" {
		return nil
	}
	return idx.indexBM25Chunk(BuildBM25ChunkForIndex(chunk))
}

func (idx *diskIndex) indexBM25Chunk(bm25 BM25Chunk) error {
	idx.mu.Lock()
	defer idx.mu.Unlock()

	if err := idx.deleteChunkLocked(bm25.Fields.ChunkID); err != nil {
		return err
	}
	return idx.indexBM25ChunkUnlocked(bm25)
}

func (idx *diskIndex) indexBM25ChunkUnlocked(bm25 BM25Chunk) error {
	meta, err := idx.loadMeta()
	if err != nil {
		return err
	}
	if meta.Version == 0 {
		meta.Version = diskMetaVersion
	}

	cm := diskChunkMetaFromBM25(bm25)
	cmData, err := encodeDiskChunkMeta(cm)
	if err != nil {
		return err
	}
	if err := idx.kv.Put(diskChunkKey(bm25.Fields.ChunkID), cmData); err != nil {
		return err
	}

	terms := diskCollectTerms(bm25)
	for _, term := range terms {
		entry := diskPostingFromBM25(bm25.Fields.ChunkID, bm25.Fields.Corpus, term, bm25.TermFreq)
		if err := idx.appendPosting(term, entry); err != nil {
			return err
		}
		if err := idx.incrementDF(term); err != nil {
			return err
		}
	}

	termsData, err := json.Marshal(terms)
	if err != nil {
		return err
	}
	if err := idx.kv.Put(diskTermsKey(bm25.Fields.ChunkID), termsData); err != nil {
		return err
	}

	meta = idx.registerChunkMeta(meta, bm25)
	return idx.saveMeta(meta)
}

func diskCollectTerms(c BM25Chunk) []string {
	seen := make(map[string]struct{})
	var terms []string
	for _, tf := range c.TermFreq {
		for term := range tf {
			term = diskNormalizeTerm(term)
			if term == "" {
				continue
			}
			if _, ok := seen[term]; ok {
				continue
			}
			seen[term] = struct{}{}
			terms = append(terms, term)
		}
	}
	return terms
}

func (idx *diskIndex) appendPosting(term string, entry diskPostingEntry) error {
	key := diskPostKey(term)
	var entries []diskPostingEntry
	data, err := diskGetOptional(idx.kv, key)
	if err != nil {
		return err
	}
	if len(data) > 0 {
		entries, err = decodeDiskPostingList(data)
		if err != nil {
			return err
		}
	}
	entries = append(entries, entry)
	return idx.kv.Put(key, encodeDiskPostingList(entries))
}

func (idx *diskIndex) incrementDF(term string) error {
	key := diskDFKey(term)
	var df uint32
	data, err := diskGetOptional(idx.kv, key)
	if err != nil {
		return err
	}
	if len(data) >= 4 {
		df = binary.BigEndian.Uint32(data)
	}
	df++
	buf := make([]byte, 4)
	binary.BigEndian.PutUint32(buf, df)
	return idx.kv.Put(key, buf)
}

func (idx *diskIndex) decrementDF(term string) error {
	key := diskDFKey(term)
	data, err := diskGetOptional(idx.kv, key)
	if err != nil || len(data) < 4 {
		return err
	}
	df := binary.BigEndian.Uint32(data)
	if df <= 1 {
		return idx.kv.Delete(key)
	}
	df--
	buf := make([]byte, 4)
	binary.BigEndian.PutUint32(buf, df)
	return idx.kv.Put(key, buf)
}

func (idx *diskIndex) DeleteChunk(chunkID string) error {
	if chunkID == "" {
		return nil
	}
	idx.mu.Lock()
	defer idx.mu.Unlock()
	return idx.deleteChunkLocked(chunkID)
}

func (idx *diskIndex) deleteChunkLocked(chunkID string) error {
	cmData, err := diskGetOptional(idx.kv, diskChunkKey(chunkID))
	if err != nil || len(cmData) == 0 {
		return err
	}
	cm, err := decodeDiskChunkMeta(cmData)
	if err != nil {
		return err
	}

	var terms []string
	termsData, err := diskGetOptional(idx.kv, diskTermsKey(chunkID))
	if err != nil {
		return err
	}
	if len(termsData) > 0 {
		_ = json.Unmarshal(termsData, &terms)
	}

	bm25 := diskRebuildBM25ForUnregister(cm, terms, idx)
	for _, term := range terms {
		key := diskPostKey(term)
		data, err := diskGetOptional(idx.kv, key)
		if err != nil || len(data) == 0 {
			continue
		}
		entries, err := decodeDiskPostingList(data)
		if err != nil {
			continue
		}
		entries, removed := removeDiskPostingEntry(entries, chunkID)
		if !removed {
			continue
		}
		if len(entries) == 0 {
			_ = idx.kv.Delete(key)
			_ = idx.kv.Delete(diskDFKey(term))
		} else {
			_ = idx.kv.Put(key, encodeDiskPostingList(entries))
			_ = idx.decrementDF(term)
		}
	}

	meta, err := idx.loadMeta()
	if err != nil {
		return err
	}
	meta = idx.unregisterChunkMeta(meta, bm25)
	_ = idx.saveMeta(meta)

	_ = idx.kv.Delete(diskChunkKey(chunkID))
	_ = idx.kv.Delete(diskTermsKey(chunkID))
	return nil
}

func diskRebuildBM25ForUnregister(cm diskChunkMeta, terms []string, idx *diskIndex) BM25Chunk {
	termFreq := make(map[string]map[string]int)
	for _, term := range terms {
		data, err := diskGetOptional(idx.kv, diskPostKey(term))
		if err != nil || len(data) == 0 {
			continue
		}
		entries, err := decodeDiskPostingList(data)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if e.ChunkID == cm.ChunkID {
				diskMergeTermFreq(termFreq, term, e)
				break
			}
		}
	}
	return bm25ChunkFromDiskMeta(cm, termFreq)
}

func (idx *diskIndex) ChunkCount() (int, error) {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	return idx.chunkCountLocked()
}

func (idx *diskIndex) chunkCountLocked() (int, error) {
	meta, err := idx.loadMeta()
	if err != nil {
		return 0, err
	}
	return meta.ChunkCount, nil
}

func (idx *diskIndex) HasMeta() bool {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	meta, err := idx.loadMeta()
	return err == nil && meta.ChunkCount > 0
}

func (idx *diskIndex) IsRebuilding() bool {
	return idx.rebuilding.Load()
}

func (idx *diskIndex) Search(text, corpus string, k int) ([]Hit, error) {
	if idx.rebuilding.Load() {
		return nil, nil
	}

	idx.mu.RLock()
	defer idx.mu.RUnlock()

	if idx.rebuilding.Load() {
		return nil, nil
	}
	query := diskDedupeTerms(Tokenize(text))
	if len(query) == 0 {
		return nil, nil
	}

	meta, err := idx.loadMeta()
	if err != nil || meta.N == 0 {
		return nil, err
	}
	global := BM25Global{
		N:     meta.N,
		AvgDL: meta.AvgDL,
		DF:    make(map[string]int),
	}
	for _, term := range query {
		df, err := idx.loadDF(term)
		if err != nil {
			continue
		}
		global.DF[term] = df
	}

	candidates := idx.collectCandidates(query, corpus)
	if len(candidates) == 0 {
		return nil, nil
	}
	if len(candidates) > diskMaxCandidates {
		candidates = candidates[:diskMaxCandidates]
	}

	type scored struct {
		id    string
		score float64
	}
	scores := make([]scored, 0, len(candidates))
	for _, chunkID := range candidates {
		cmData, err := diskGetOptional(idx.kv, diskChunkKey(chunkID))
		if err != nil || len(cmData) == 0 {
			continue
		}
		cm, err := decodeDiskChunkMeta(cmData)
		if err != nil {
			continue
		}
		if corpus != "" && cm.Corpus != corpus {
			continue
		}
		bm25, err := idx.buildCandidateBM25(cm, query)
		if err != nil {
			continue
		}
		s := ScoreChunkBM25(bm25, query, &global)
		if s > 0 {
			scores = append(scores, scored{id: chunkID, score: s})
		}
	}

	sort.Slice(scores, func(i, j int) bool { return scores[i].score > scores[j].score })
	if k > 0 && len(scores) > k {
		scores = scores[:k]
	}
	hits := make([]Hit, len(scores))
	for i, s := range scores {
		hits[i] = Hit{ChunkID: s.id, Score: s.score}
	}
	return hits, nil
}

func diskDedupeTerms(terms []string) []string {
	seen := make(map[string]struct{}, len(terms))
	out := make([]string, 0, len(terms))
	for _, t := range terms {
		t = diskNormalizeTerm(t)
		if t == "" {
			continue
		}
		if _, ok := seen[t]; ok {
			continue
		}
		seen[t] = struct{}{}
		out = append(out, t)
	}
	return out
}

func (idx *diskIndex) loadDF(term string) (int, error) {
	data, err := diskGetOptional(idx.kv, diskDFKey(term))
	if err != nil || len(data) < 4 {
		return 0, err
	}
	return int(binary.BigEndian.Uint32(data)), nil
}

func (idx *diskIndex) collectCandidates(query []string, corpus string) []string {
	set := make(map[string]struct{})
	for _, term := range query {
		data, err := diskGetOptional(idx.kv, diskPostKey(term))
		if err != nil || len(data) == 0 {
			continue
		}
		entries, err := decodeDiskPostingList(data)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if corpus != "" && e.Corpus != corpus {
				continue
			}
			set[e.ChunkID] = struct{}{}
		}
	}
	out := make([]string, 0, len(set))
	for id := range set {
		out = append(out, id)
	}
	return out
}

func (idx *diskIndex) buildCandidateBM25(cm diskChunkMeta, query []string) (BM25Chunk, error) {
	termFreq := make(map[string]map[string]int)
	for _, term := range query {
		data, err := diskGetOptional(idx.kv, diskPostKey(term))
		if err != nil || len(data) == 0 {
			continue
		}
		entries, err := decodeDiskPostingList(data)
		if err != nil {
			return BM25Chunk{}, err
		}
		for _, e := range entries {
			if e.ChunkID == cm.ChunkID {
				diskMergeTermFreq(termFreq, term, e)
				break
			}
		}
	}
	return bm25ChunkFromDiskMeta(cm, termFreq), nil
}

func (idx *diskIndex) RebuildFromChunks(scan func(yield func(model.Chunk) error) error, chunksTotal int, onProgress RebuildProgressFunc) (RebuildStats, error) {
	idx.mu.Lock()
	idx.rebuilding.Store(true)
	defer func() {
		idx.rebuilding.Store(false)
		idx.mu.Unlock()
	}()

	if onProgress != nil {
		onProgress(0, chunksTotal)
	}

	if err := idx.deleteAllLexKeys(); err != nil {
		return RebuildStats{}, err
	}

	n := 0
	start := time.Now()
	err := scan(func(chunk model.Chunk) error {
		f := FieldsFromChunk(chunk)
		if f.ChunkID == "" || f.Text == "" {
			return nil
		}
		bm25 := BuildBM25ChunkForIndex(f)
		if err := idx.indexBM25ChunkUnlocked(bm25); err != nil {
			return err
		}
		n++
		if n%1000 == 0 {
			log.Printf("f4kvs lexical: rebuild progress %d chunks", n)
		}
		if onProgress != nil && (n <= 10 || n%100 == 0 || n == chunksTotal) {
			onProgress(n, chunksTotal)
		}
		return nil
	})
	if err != nil {
		return RebuildStats{}, err
	}

	if n > 0 {
		meta, _ := idx.loadMeta()
		meta.BuiltAt = time.Now().UTC()
		meta.Version = diskMetaVersion
		_ = idx.saveMeta(meta)
	}

	if onProgress != nil {
		onProgress(n, chunksTotal)
	}

	elapsed := time.Since(start)
	log.Printf("f4kvs lexical: built disk index over %d chunks in %s", n, elapsed.Round(time.Millisecond))
	return RebuildStats{ChunksIndexed: n, ChunksTotal: chunksTotal, Duration: elapsed}, nil
}

func (idx *diskIndex) deleteAllLexKeys() error {
	pairs, err := idx.kv.ScanPrefix(diskPrefixAll)
	if err != nil {
		return err
	}
	for _, pair := range pairs {
		if err := idx.kv.Delete(pair.Key); err != nil {
			return err
		}
	}
	return nil
}
