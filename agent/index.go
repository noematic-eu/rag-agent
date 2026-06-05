package main

import (
	"bytes"
	"encoding/json"
	"log"
	"math"
	"regexp"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/yuin/goldmark"

	"github.com/noematic-eu/ai-rag-agent/model"
)

// indexDocument indexe le contenu du document dans bleve et calcule ses scores TF-IDF.
// It returns the number of chunks indexed.
func indexDocument(doc model.LegalDocument) (int, error) {
	if err := ensureManifestInitialized(); err != nil {
		return 0, err
	}

	chunks, err := ChunkDocument(doc, DefaultChunkConfig())
	if err != nil {
		log.Printf("Erreur lors du chunking du document %s : %v", doc.ID, err)
		return 0, err
	}

	for i := range chunks {
		chunks[i].Text = normalizeChunkText(chunks[i].Text)
	}

	chunks = filterIndexableChunks(chunks)
	if len(chunks) == 0 {
		fallback := model.Chunk{
			Metadata: model.ChunkMetadata{
				DocID:       doc.ID,
				ChunkID:     doc.ID + "-chunk-0",
				Title:       doc.Title,
				DocTitle:    doc.Title,
				SectionPath: "document",
				Source:      doc.ContentType,
				Position:    0,
				Corpus:      doc.Corpus,
			},
			Text: normalizeChunkText(doc.Content),
		}
		if isIndexableChunk(fallback) {
			chunks = []model.Chunk{fallback}
		}
	}
	if len(chunks) == 0 {
		log.Printf("Document %s: aucun chunk indexable après filtrage", doc.ID)
		return 0, nil
	}

	log.Printf("Document %s divisé en %d chunks indexables", doc.ID, len(chunks))

	chunkTexts := make([]string, 0, len(chunks))
	for _, chunk := range chunks {
		chunkTexts = append(chunkTexts, chunk.Text)
	}
	embeddings := make([][]float64, len(chunks))
	embeddingFailed := false
	if llmConfig.EmbeddingsEnabled {
		embeddedValues, embErr := EmbedTextBatch(chunkTexts)
		if embErr != nil {
			embeddingFailed = true
			log.Printf("Erreur embedding, poursuite sans vecteurs pour %s: %v", doc.ID, embErr)
		} else {
			embeddings = embeddedValues
			for i := range chunks {
				chunks[i].Embedding = embeddings[i]
			}
		}
	}

	indexed := 0
	embeddedChunks := 0
	for i := range chunks {
		if doc.Corpus != "" {
			chunks[i].Metadata.Corpus = doc.Corpus
		}
		if doc.ContentType == contentTypeHTML {
			chunks[i].Metadata.Source = contentTypeHTML
		}
		if strings.TrimSpace(chunks[i].Metadata.Title) == "" {
			chunks[i].Metadata.Title = doc.Title
		}
		chunks[i].Metadata.DocTitle = doc.Title

		text := prepareChunkIndexText(chunks[i].Text)
		if text == "" {
			continue
		}
		chunks[i].Text = text

		chunkID := chunks[i].Metadata.ChunkID
		if err := lexicalBackend.IndexChunk(chunks[i]); err != nil {
			log.Printf("Erreur lors de l'indexation lexicale du chunk %s : %v", chunkID, err)
			continue
		}

		tokens := tokenize(text)
		tf := calculateTF(tokens)
		updateIDF(tokens)

		documentTFIDFs = append(documentTFIDFs, DocumentTFIDF{
			ID:    chunkID,
			TFIDF: tf,
			Norm:  calculateNorm(tf),
		})

		storeChunkMetadata(chunks[i])
		indexed++
		if len(chunks[i].Embedding) > 0 {
			embeddedChunks++
		}
		log.Printf("Chunk %s indexé (%d/%d)", chunkID, indexed, len(chunks))
	}

	totalDocs += float64(indexed)
	if indexed > 0 {
		storeDocumentMetadata(doc)
	}
	recordIngest(doc.ID, doc.Corpus, indexed, embeddedChunks, embeddingFailed)
	log.Printf("Document %s indexé avec %d chunks", doc.ID, indexed)
	return indexed, nil
}

func prepareChunkIndexText(raw string) string {
	text := parseMarkdown(raw)
	if strings.TrimSpace(text) == "" {
		text = strings.TrimSpace(raw)
	}
	return normalizeChunkText(text)
}

// storeChunkMetadata stocke un chunk dans f4kvs (text + metadata only; no duplicated doc source).
func storeChunkMetadata(chunk model.Chunk) {
	chunk.Original = ""
	data, err := json.Marshal(chunk)
	if err != nil {
		log.Printf("Erreur lors du marshalling du chunk %s : %v", chunk.Metadata.ChunkID, err)
		return
	}

	if err := chunkStore.Put("chunk:"+chunk.Metadata.ChunkID, data); err != nil {
		log.Printf("Erreur lors du stockage du chunk %s dans f4kvs : %v", chunk.Metadata.ChunkID, err)
	}
}

// parseMarkdown convertit le markdown en texte brut
func parseMarkdown(md string) string {
	var buf bytes.Buffer
	if err := goldmark.Convert([]byte(md), &buf); err != nil {
		log.Printf("Erreur lors de la conversion markdown : %v", err)
		return md
	}
	doc, err := goquery.NewDocumentFromReader(&buf)
	if err != nil {
		log.Printf("Erreur lors de la création du document goquery : %v", err)
		return md
	}
	return doc.Text()
}

// calculateTF calcule la fréquence des termes pour un document
func calculateTF(tokens []string) map[string]float64 {
	tf := make(map[string]float64)
	total := float64(len(tokens))
	if total == 0 {
		return tf
	}
	for _, token := range tokens {
		tf[token]++
	}
	for token := range tf {
		tf[token] /= total
	}
	return tf
}

// calculateNorm calcule la norme du vecteur TF-IDF
func calculateNorm(tf map[string]float64) float64 {
	var sum float64
	for term, tfScore := range tf {
		if idf, exists := globalIDF[term]; exists {
			sum += tfScore * tfScore * idf * idf
		}
	}
	return math.Sqrt(sum)
}

// cosineSimilarity calcule la similarité cosinus entre deux documents
func cosineSimilarity(doc1, doc2 DocumentTFIDF) float64 {
	var dotProduct float64
	for term, tfidf1 := range doc1.TFIDF {
		if tfidf2, exists := doc2.TFIDF[term]; exists {
			dotProduct += tfidf1 * tfidf2
		}
	}
	if doc1.Norm == 0 || doc2.Norm == 0 {
		return 0
	}
	return dotProduct / (doc1.Norm * doc2.Norm)
}

func tokenize(text string) []string {
	words := strings.Fields(strings.ToLower(text))
	var tokens []string
	for i := 0; i < len(words); i++ {
		cleaned := tokenCleaner.ReplaceAllString(words[i], "")
		if cleaned == "" {
			continue
		}
		if len(cleaned) > 2 || isLegalTerm(cleaned) {
			next := ""
			if i+1 < len(words) {
				next = tokenCleaner.ReplaceAllString(words[i+1], "")
			}
			if next != "" && isLegalPhrase(cleaned, next) {
				tokens = append(tokens, cleaned+" "+next)
				i++
			} else {
				tokens = append(tokens, cleaned)
			}
		}
	}
	return tokens
}

var totalDocs float64
var tokenCleaner = regexp.MustCompile(`[^\p{L}\p{N}]+`)

func updateIDF(tokens []string) {
	seen := make(map[string]bool)
	for _, token := range tokens {
		if !seen[token] {
			globalIDF[token]++
			seen[token] = true
		}
	}
}

func finalizeIDF() {
	for token := range globalIDF {
		globalIDF[token] = math.Log(totalDocs / globalIDF[token])
	}
}
