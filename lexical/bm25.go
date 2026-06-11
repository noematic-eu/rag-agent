package lexical

import "math"

const (
	bm25K1 = 1.2
	bm25B  = 0.75
)

// BM25Global holds corpus-level stats for RAM BM25.
type BM25Global struct {
	N     int
	AvgDL float64
	DF    map[string]int
}

// BM25Chunk holds per-chunk term stats per field.
type BM25Chunk struct {
	Fields   ChunkFields
	TermFreq map[string]map[string]int // field -> term -> tf
	Length   map[string]int            // field -> token count
}

func newBM25Chunk(f ChunkFields) BM25Chunk {
	return BM25Chunk{
		Fields:   f,
		TermFreq: make(map[string]map[string]int),
		Length:   make(map[string]int),
	}
}

func (c *BM25Chunk) addField(fieldName, text string, boost float64) {
	_ = boost
	tokens := Tokenize(text)
	if len(tokens) == 0 {
		return
	}
	tf := make(map[string]int, len(tokens))
	for _, tok := range tokens {
		tf[tok]++
	}
	c.TermFreq[fieldName] = tf
	c.Length[fieldName] = len(tokens)
}

func (g *BM25Global) registerChunk(c BM25Chunk) {
	g.N++
	var totalLen float64
	for _, field := range []string{"text", "title", "doc_title", "section"} {
		totalLen += float64(c.Length[field])
	}
	if g.N == 1 {
		g.AvgDL = totalLen
	} else {
		g.AvgDL = g.AvgDL + (totalLen-g.AvgDL)/float64(g.N)
	}
	seen := make(map[string]struct{})
	for _, tf := range c.TermFreq {
		for term := range tf {
			if _, ok := seen[term]; ok {
				continue
			}
			seen[term] = struct{}{}
			g.DF[term]++
		}
	}
}

func (g *BM25Global) idf(term string) float64 {
	df := float64(g.DF[term])
	if df <= 0 {
		return 0
	}
	return math.Log(1 + (float64(g.N)-df+0.5)/(df+0.5))
}

func bm25FieldScore(tf map[string]int, dl int, query []string, g *BM25Global) float64 {
	if dl == 0 || len(query) == 0 {
		return 0
	}
	var score float64
	dlF := float64(dl)
	avgDL := g.AvgDL
	if avgDL <= 0 {
		avgDL = 1
	}
	for _, term := range query {
		freq := float64(tf[term])
		if freq == 0 {
			continue
		}
		idf := g.idf(term)
		num := freq * (bm25K1 + 1)
		den := freq + bm25K1*(1-bm25B+bm25B*dlF/avgDL)
		score += idf * (num / den)
	}
	return score
}

// ScoreChunkBM25 scores one chunk; uses max across fields with boosts (approximates disjunction).
func ScoreChunkBM25(c BM25Chunk, query []string, g *BM25Global) float64 {
	if len(query) == 0 {
		return 0
	}
	fields := []struct {
		name  string
		boost float64
	}{
		{"text", BoostText},
		{"title", BoostTitle},
		{"doc_title", BoostDocTitle},
		{"section", BoostSection},
		{"article", BoostArticle},
	}
	var best float64
	for _, f := range fields {
		s := bm25FieldScore(c.TermFreq[f.name], c.Length[f.name], query, g) * f.boost
		if s > best {
			best = s
		}
	}
	return best
}

func buildBM25Chunk(f ChunkFields) BM25Chunk {
	c := newBM25Chunk(f)
	c.addField("text", f.Text, BoostText)
	c.addField("title", f.Title, BoostTitle)
	c.addField("doc_title", f.DocTitle, BoostDocTitle)
	c.addField("section", f.Section, BoostSection)
	c.addField("article", f.Article, BoostArticle)
	return c
}

// BuildBM25ChunkForIndex builds BM25 stats for disk indexing.
func BuildBM25ChunkForIndex(f ChunkFields) BM25Chunk {
	return buildBM25Chunk(f)
}

func registerChunkFields(g *BM25Global, chunks *[]BM25Chunk, f ChunkFields) {
	c := buildBM25Chunk(f)
	g.registerChunk(c)
	*chunks = append(*chunks, c)
}

func (g *BM25Global) unregisterChunk(c BM25Chunk) {
	if g.N <= 0 {
		return
	}
	g.N--
	var totalLen float64
	for _, field := range []string{"text", "title", "doc_title", "section"} {
		totalLen += float64(c.Length[field])
	}
	if g.N == 0 {
		g.AvgDL = 0
	} else {
		g.AvgDL = (g.AvgDL*float64(g.N+1) - totalLen) / float64(g.N)
	}
	seen := make(map[string]struct{})
	for _, tf := range c.TermFreq {
		for term := range tf {
			if _, ok := seen[term]; ok {
				continue
			}
			seen[term] = struct{}{}
			if g.DF[term] > 0 {
				g.DF[term]--
			}
		}
	}
}
