package main

import (
	"bytes"
	"fmt"
	"log"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"

	"github.com/noematic-eu/ai-rag-agent/model"
)

// ChunkConfig configure les paramètres de chunking
type ChunkConfig struct {
	MaxTokens     int    // Nombre maximal de tokens par chunk (défaut: 600)
	OverlapTokens int    // Nombre de tokens de chevauchement (défaut: 60)
	MinChunkSize  int    // Taille minimale du chunk (défaut: 100)
	Separator     string // Séparateur de texte (défaut: "\n\n")
}

// DefaultChunkConfig retourne la configuration par défaut
func DefaultChunkConfig() ChunkConfig {
	return ChunkConfig{
		MaxTokens:     600,
		OverlapTokens: 60,
		MinChunkSize:  100,
		Separator:     "\n\n",
	}
}

// ChunkResult représente un chunk généré
type ChunkResult struct {
	Chunk       model.Chunk
	TokenCount  int
	SentencePos int // Position de la phrase dans le document original
}

// extractHeadings extrait les titres d'un document markdown
func extractHeadings(md string) []string {
	var headings []string
	parser := goldmark.New()
	reader := text.NewReader([]byte(md))
	p := parser.Parser()
	document := p.Parse(reader)

	_ = ast.Walk(document, func(node ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		switch node := node.(type) {
		case *ast.Heading:
			level := node.Level
			var buf bytes.Buffer
			if err := parser.Renderer().Render(&buf, reader.Source(), document); err != nil {
				return ast.WalkContinue, nil
			}
			// Extraire le texte du heading
			text := extractTextFromNode(node, reader.Source())
			if text != "" {
				headings = append(headings, fmt.Sprintf("%s %d: %s", getHeadingPrefix(level), level, text))
			}
		}
		return ast.WalkContinue, nil
	})

	return headings
}

// getHeadingPrefix retourne le préfixe du heading (Article, Titre, etc.)
func getHeadingPrefix(level int) string {
	prefixes := map[int]string{
		1: "Titre",
		2: "Chapitre",
		3: "Section",
		4: "Paragraphe",
	}
	if prefix, ok := prefixes[level]; ok {
		return prefix
	}
	return fmt.Sprintf("Article-%d", level)
}

// extractTextFromNode extrait le texte d'un nœud AST
func extractTextFromNode(node ast.Node, source []byte) string {
	var buf bytes.Buffer
	for child := node.FirstChild(); child != nil; child = child.NextSibling() {
		if text, ok := child.(*ast.Text); ok {
			buf.Write(text.Segment.Value(source))
		} else if s, ok := child.(*ast.String); ok {
			buf.WriteString(string(s.Value))
		} else if code, ok := child.(*ast.CodeSpan); ok {
			writeCodeSpanText(&buf, code, source)
		}
	}
	return buf.String()
}

func writeCodeSpanText(buf *bytes.Buffer, code *ast.CodeSpan, source []byte) {
	for gc := code.FirstChild(); gc != nil; gc = gc.NextSibling() {
		if t, ok := gc.(*ast.Text); ok {
			buf.Write(t.Segment.Value(source))
		}
	}
}

// splitByHeadings divise le contenu par les headings
func splitByHeadings(md string) []string {
	parser := goldmark.New()
	reader := text.NewReader([]byte(md))
	p := parser.Parser()
	document := p.Parse(reader)

	var sections []string
	var currentSection strings.Builder

	_ = ast.Walk(document, func(node ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}

		switch node := node.(type) {
		case *ast.Heading:
			// Si on a déjà du contenu, on l'ajoute aux sections
			if currentSection.Len() > 0 {
				sections = append(sections, strings.TrimSpace(currentSection.String()))
				currentSection.Reset()
			}
			// Ajouter le heading
			var buf bytes.Buffer
			for child := node.FirstChild(); child != nil; child = child.NextSibling() {
				if text, ok := child.(*ast.Text); ok {
					buf.Write(text.Segment.Value(reader.Source()))
				} else if s, ok := child.(*ast.String); ok {
					buf.WriteString(string(s.Value))
				}
			}
			currentSection.WriteString(buf.String())
			currentSection.WriteString("\n\n")
		case *ast.Paragraph:
			var buf bytes.Buffer
			for child := node.FirstChild(); child != nil; child = child.NextSibling() {
				if text, ok := child.(*ast.Text); ok {
					buf.Write(text.Segment.Value(reader.Source()))
				} else if s, ok := child.(*ast.String); ok {
					buf.WriteString(string(s.Value))
				} else if code, ok := child.(*ast.CodeSpan); ok {
					writeCodeSpanText(&buf, code, reader.Source())
				}
			}
			currentSection.WriteString(buf.String())
			currentSection.WriteString("\n\n")
		case *ast.CodeBlock:
			lines := node.Lines()
			for i := 0; i < lines.Len(); i++ {
				segment := lines.At(i)
				currentSection.Write(segment.Value(reader.Source()))
			}
			currentSection.WriteString("\n\n")
		case *ast.List:
			var buf bytes.Buffer
			for child := node.FirstChild(); child != nil; child = child.NextSibling() {
				if listitem, ok := child.(*ast.ListItem); ok {
					for subchild := listitem.FirstChild(); subchild != nil; subchild = subchild.NextSibling() {
						if paragraph, ok := subchild.(*ast.Paragraph); ok {
							for gchild := paragraph.FirstChild(); gchild != nil; gchild = gchild.NextSibling() {
								if text, ok := gchild.(*ast.Text); ok {
									buf.Write(text.Segment.Value(reader.Source()))
								} else if s, ok := gchild.(*ast.String); ok {
									buf.WriteString(string(s.Value))
								}
							}
						}
					}
				}
			}
			currentSection.WriteString(buf.String())
			currentSection.WriteString("\n\n")
		}

		return ast.WalkContinue, nil
	})

	// Ajouter la dernière section
	if currentSection.Len() > 0 {
		sections = append(sections, strings.TrimSpace(currentSection.String()))
	}

	return sections
}

// countTokens compte approximativement le nombre de tokens dans un texte
func countTokens(text string) int {
	// Approximation: 1 token ≈ 4 caractères en anglais, 3 en français
	words := strings.Fields(strings.TrimSpace(text))
	return len(words)
}

// splitByTokenCount divise un texte en chunks de taille approximative
func splitByTokenCount(text string, config ChunkConfig) []string {
	words := strings.Fields(strings.TrimSpace(text))
	var chunks []string
	var currentChunk strings.Builder

	for i := 0; i < len(words); {
		currentChunk.Reset()
		tokenCount := 0
		// Remplir le chunk jusqu'à la limite
		for i < len(words) && tokenCount+countTokens(words[i]) <= config.MaxTokens {
			if currentChunk.Len() > 0 {
				currentChunk.WriteString(" ")
			}
			currentChunk.WriteString(words[i])
			tokenCount += countTokens(words[i])
			i++
		}

		// Si on a au moins MinChunkSize tokens, ajouter le chunk
		if tokenCount >= config.MinChunkSize || i == len(words) {
			chunks = append(chunks, strings.TrimSpace(currentChunk.String()))

			// Gérer l'overlap
			if i < len(words) && config.OverlapTokens > 0 {
				// Retourner en arrière pour ajouter de l'overlap
				overlapWords := 0
				overlapIndex := i - 1

				for overlapIndex >= 0 && overlapWords < config.OverlapTokens {
					overlapWords += countTokens(words[overlapIndex])
					overlapIndex--
				}

				if overlapIndex >= 0 {
					i = overlapIndex + 1
				}
			}
		} else {
			// Si le chunk est trop petit, on continue avec les mots suivants
			i++
		}
	}

	return chunks
}

// sectionDisplayText derives title and indexable text from a heading-delimited section.
func sectionDisplayText(section string) (title, text string) {
	section = strings.TrimSpace(section)
	if section == "" {
		return "", ""
	}
	lines := strings.SplitN(section, "\n", 2)
	title = strings.TrimSpace(lines[0])
	body := ""
	if len(lines) > 1 {
		body = strings.TrimSpace(lines[1])
	}
	text = section
	if title != "" && body != "" && !strings.Contains(text, body) {
		text = title + "\n\n" + body
	}
	return title, strings.TrimSpace(text)
}

func sectionExceedsChunkLimit(text string, config ChunkConfig) bool {
	return countTokens(text) > config.MaxTokens*2
}

func appendTokenSplitChunks(chunks *[]model.Chunk, index *int, docID, title, sectionPath, text string, config ChunkConfig) {
	for _, chunkText := range splitByTokenCount(text, config) {
		chunkText = strings.TrimSpace(stripCopyrightLines(chunkText))
		if chunkText == "" {
			continue
		}
		chunkTitle := deriveChunkTitle(title, chunkText, *index)
		chunkSectionPath := sectionPath
		if isBoilerplateTitle(title) || chunkTitle != title {
			chunkSectionPath = buildSectionPath(chunkTitle, *index)
		}
		*chunks = append(*chunks, model.Chunk{
			Metadata: model.ChunkMetadata{
				DocID:       docID,
				ChunkID:     fmt.Sprintf("%s-chunk-%d", docID, *index),
				Title:       chunkTitle,
				SectionPath: chunkSectionPath,
				Source:      "markdown",
				Position:    *index,
			},
			Text: chunkText,
		})
		*index++
	}
}

// buildSectionPath construit le chemin de section
func buildSectionPath(title string, position int) string {
	// Déterminer le type de section
	var prefix string
	level := 1

	if strings.HasPrefix(strings.ToLower(title), "article") {
		prefix = "Article"
		level = 3
	} else if strings.HasPrefix(strings.ToLower(title), "titre") {
		prefix = "Titre"
		level = 1
	} else if strings.HasPrefix(strings.ToLower(title), "chapitre") {
		prefix = "Chapitre"
		level = 2
	} else if strings.HasPrefix(strings.ToLower(title), "section") {
		prefix = "Section"
		level = 3
	} else if strings.HasPrefix(strings.ToLower(title), "paragraphe") {
		prefix = "Paragraphe"
		level = 4
	} else {
		prefix = fmt.Sprintf("Section-%d", level)
	}

	if level == 1 {
		return title
	}

	return fmt.Sprintf("%s %d", prefix, position+1)
}

// parseHTMLToChunks parse du HTML et crée des chunks
func parseHTMLToChunks(html string, docID string) ([]model.Chunk, error) {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		return nil, fmt.Errorf("erreur lors de la création du document goquery: %v", err)
	}

	var chunks []model.Chunk
	chunkID := 0

	// Parcourir les éléments de haut niveau (h1, h2, p, etc.)
	doc.Find("body").Children().Each(func(i int, s *goquery.Selection) {
		tag := goquery.NodeName(s)

		switch tag {
		case "h1", "h2", "h3", "h4", "h5", "h6":
			// C'est un titre, on peut l'utiliser comme section
			text := strings.TrimSpace(s.Text())
			if text != "" {
				chunkID++
				chunk := model.Chunk{
					Metadata: model.ChunkMetadata{
						DocID:       docID,
						ChunkID:     fmt.Sprintf("%s-chunk-%d", docID, chunkID),
						Title:       text,
						SectionPath: buildHeadingPath(s, tag),
						Source:      "html",
						Position:    chunkID,
					},
					Text:     text,
					Original: selectionHTML(s),
				}
				chunks = append(chunks, chunk)
			}

		case "p":
			// C'est un paragraphe
			text := strings.TrimSpace(s.Text())
			if text != "" {
				chunkID++
				// Trouver le titre précédent
				var title string
				s.PrevAll().Each(func(j int, prev *goquery.Selection) {
					prevTag := goquery.NodeName(prev)
					if prevTag >= "h1" && prevTag <= "h6" {
						title = strings.TrimSpace(prev.Text())
						return
					}
				})

				chunk := model.Chunk{
					Metadata: model.ChunkMetadata{
						DocID:       docID,
						ChunkID:     fmt.Sprintf("%s-chunk-%d", docID, chunkID),
						Title:       title,
						SectionPath: buildHeadingPath(s, "p"),
						Source:      "html",
						Position:    chunkID,
					},
					Text:     text,
					Original: selectionHTML(s),
				}
				chunks = append(chunks, chunk)
			}

		case "ul", "ol":
			// Liste
			s.Find("li").Each(func(j int, li *goquery.Selection) {
				text := strings.TrimSpace(li.Text())
				if text != "" {
					chunkID++
					chunk := model.Chunk{
						Metadata: model.ChunkMetadata{
							DocID:       docID,
							ChunkID:     fmt.Sprintf("%s-chunk-%d", docID, chunkID),
							Title:       "",
							SectionPath: buildHeadingPath(s, "li"),
							Source:      "html",
							Position:    chunkID,
						},
						Text:     text,
						Original: selectionHTML(li),
					}
					chunks = append(chunks, chunk)
				}
			})

		case "pre", "code":
			// Bloc de code
			text := strings.TrimSpace(s.Text())
			if text != "" {
				chunkID++
				chunk := model.Chunk{
					Metadata: model.ChunkMetadata{
						DocID:       docID,
						ChunkID:     fmt.Sprintf("%s-chunk-%d", docID, chunkID),
						Title:       "",
						SectionPath: buildHeadingPath(s, tag),
						Source:      "html",
						Position:    chunkID,
					},
					Text:     text,
					Original: selectionHTML(s),
				}
				chunks = append(chunks, chunk)
			}
		}
	})

	return chunks, nil
}

// buildHeadingPath construit le chemin de heading pour HTML
func buildHeadingPath(s *goquery.Selection, tag string) string {
	var path []string

	// Trouver tous les headings parents
	s.Parents().Each(func(i int, parent *goquery.Selection) {
		parentTag := goquery.NodeName(parent)
		if parentTag >= "h1" && parentTag <= "h6" {
			path = append(path, strings.TrimSpace(parent.Text()))
		}
	})

	// Inverser pour avoir du plus haut au plus bas
	for i, j := 0, len(path)-1; i < j; i, j = i+1, j-1 {
		path[i], path[j] = path[j], path[i]
	}

	if len(path) == 0 {
		return "root"
	}

	return strings.Join(path, " > ")
}

// ChunkDocument divise un document en chunks avec metadata.
// Full document source is stored once via storeDocumentMetadata at ingest, not per chunk.
func ChunkDocument(doc model.LegalDocument, config ChunkConfig) ([]model.Chunk, error) {
	parser := goldmark.New()
	var buf bytes.Buffer
	if err := parser.Convert([]byte(doc.Content), &buf); err != nil {
		return nil, fmt.Errorf("erreur lors de la conversion markdown: %v", err)
	}

	plainText := strings.TrimSpace(buf.String())
	if plainText == "" {
		plainText = strings.TrimSpace(doc.Content)
	}

	sections := splitByHeadings(doc.Content)
	var chunks []model.Chunk
	chunkIndex := 0

	if len(sections) <= 1 && hasLegalArticleStructure(doc.Content) {
		legalSections := splitByLegalArticles(doc.Content)
		if len(legalSections) > 1 {
			appendLegalSections(&chunks, &chunkIndex, doc.ID, legalSections, config)
			if len(chunks) == 0 {
				appendTokenSplitChunks(&chunks, &chunkIndex, doc.ID, doc.Title, "document", plainText, config)
			}
			return chunks, nil
		}
	}

	if len(sections) == 0 {
		appendTokenSplitChunks(&chunks, &chunkIndex, doc.ID, doc.Title, "document", plainText, config)
		return chunks, nil
	}

	for i, section := range sections {
		title, text := sectionDisplayText(section)
		text = strings.TrimSpace(stripCopyrightLines(text))
		if text == "" {
			continue
		}
		title = deriveChunkTitle(title, text, i)
		sectionPath := buildSectionPath(title, i)
		if sectionExceedsChunkLimit(text, config) {
			log.Printf("Token-split oversized section %d for document %s", i, doc.ID)
			appendTokenSplitChunks(&chunks, &chunkIndex, doc.ID, title, sectionPath, text, config)
			continue
		}
		chunks = append(chunks, model.Chunk{
			Metadata: model.ChunkMetadata{
				DocID:       doc.ID,
				ChunkID:     fmt.Sprintf("%s-chunk-%d", doc.ID, chunkIndex),
				Title:       title,
				SectionPath: sectionPath,
				Source:      "markdown",
				Position:    chunkIndex,
			},
			Text: text,
		})
		chunkIndex++
	}

	if len(chunks) == 0 {
		appendTokenSplitChunks(&chunks, &chunkIndex, doc.ID, doc.Title, "document", plainText, config)
	}

	return chunks, nil
}

// ChunkHTMLDocument divise un document HTML en chunks
func ChunkHTMLDocument(html string, docID string) ([]model.Chunk, error) {
	return parseHTMLToChunks(html, docID)
}

func selectionHTML(s *goquery.Selection) string {
	html, err := s.Html()
	if err != nil {
		return ""
	}
	return html
}
