package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	docmodel "github.com/noematic-eu/ai-rag-agent/model"
)

type IngestDirectoryConfig struct {
	ServerURL   string
	DirPath     string
	Finalize    bool
	BatchSize   int
	ResetBefore bool
	Corpus      string
}

func IngestDirectory(cfg IngestDirectoryConfig) error {
	info, err := os.Stat(cfg.DirPath)
	if err != nil {
		return fmt.Errorf("cannot access directory: %w", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("path is not a directory: %s", cfg.DirPath)
	}
	if cfg.BatchSize <= 0 {
		cfg.BatchSize = 100
	}

	client := NewClient(cfg.ServerURL)
	if cfg.ResetBefore {
		fmt.Println("Resetting index before ingestion...")
		if err := client.Reset(); err != nil {
			return fmt.Errorf("reset before ingest: %w", err)
		}
	}

	total := 0
	ingested := 0

	err = filepath.WalkDir(cfg.DirPath, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}

		contentType := detectContentType(path)
		if contentType == "" {
			return nil
		}
		total++

		raw, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read %s: %w", path, err)
		}

		relPath, err := filepath.Rel(cfg.DirPath, path)
		if err != nil {
			relPath = d.Name()
		}

		doc := docmodel.LegalDocument{
			ID:          stableDocID(relPath),
			Title:       titleFromPath(relPath),
			Content:     string(raw),
			ContentType: contentType,
			Corpus:      cfg.Corpus,
		}
		if contentType == "html" {
			doc.OriginalContent = doc.Content
		}

		if err := client.IngestDocument(doc); err != nil {
			return fmt.Errorf("ingest %s: %w", relPath, err)
		}

		ingested++
		if ingested%cfg.BatchSize == 0 {
			fmt.Printf("Ingested %d files...\n", ingested)
		}
		return nil
	})
	if err != nil {
		return err
	}

	if cfg.Finalize {
		if err := client.Finalize(); err != nil {
			return fmt.Errorf("finalize failed: %w", err)
		}
	}

	fmt.Printf("Done. Ingested %d/%d supported files from %s\n", ingested, total, cfg.DirPath)
	return nil
}

func detectContentType(path string) string {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".md", ".markdown":
		return "markdown"
	case ".html", ".htm":
		return "html"
	default:
		return ""
	}
}

func titleFromPath(relPath string) string {
	base := filepath.Base(relPath)
	ext := filepath.Ext(base)
	return strings.TrimSuffix(base, ext)
}

func stableDocID(relPath string) string {
	return StableDocID(relPath)
}
