package main

import (
	"sync"
	"time"
)

// LexicalIndexStats exposes lexical rebuild progress for GET /stats.
type LexicalIndexStats struct {
	Rebuilding    bool      `json:"rebuilding"`
	ChunksIndexed int       `json:"chunks_indexed,omitempty"`
	ChunksTotal   int       `json:"chunks_total,omitempty"`
	StartedAt     time.Time `json:"started_at,omitempty"`
	UpdatedAt     time.Time `json:"updated_at,omitempty"`
	DurationSec   float64   `json:"duration_s,omitempty"`
}

var lexicalProgress struct {
	mu sync.RWMutex
	v  LexicalIndexStats
}

func beginLexicalRebuild(chunksTotal int) {
	now := time.Now().UTC()
	lexicalProgress.mu.Lock()
	lexicalProgress.v = LexicalIndexStats{
		Rebuilding:    true,
		ChunksIndexed: 0,
		ChunksTotal:   chunksTotal,
		StartedAt:     now,
		UpdatedAt:     now,
	}
	lexicalProgress.mu.Unlock()
}

func updateLexicalRebuild(indexed, total int) {
	lexicalProgress.mu.Lock()
	lexicalProgress.v.Rebuilding = true
	lexicalProgress.v.ChunksIndexed = indexed
	if total > 0 {
		lexicalProgress.v.ChunksTotal = total
	}
	lexicalProgress.v.UpdatedAt = time.Now().UTC()
	lexicalProgress.mu.Unlock()
}

func finishLexicalRebuild(indexed, total int, duration time.Duration) {
	now := time.Now().UTC()
	lexicalProgress.mu.Lock()
	lexicalProgress.v = LexicalIndexStats{
		Rebuilding:    false,
		ChunksIndexed: indexed,
		ChunksTotal:   total,
		DurationSec:   duration.Seconds(),
		UpdatedAt:     now,
	}
	lexicalProgress.mu.Unlock()
}

func clearLexicalRebuildOnError() {
	lexicalProgress.mu.Lock()
	lexicalProgress.v.Rebuilding = false
	lexicalProgress.v.UpdatedAt = time.Now().UTC()
	lexicalProgress.mu.Unlock()
}

func currentLexicalIndexStats() LexicalIndexStats {
	lexicalProgress.mu.RLock()
	defer lexicalProgress.mu.RUnlock()
	return lexicalProgress.v
}

func lexicalRebuildProgressCallback() func(indexed, total int) {
	return func(indexed, total int) {
		updateLexicalRebuild(indexed, total)
	}
}
