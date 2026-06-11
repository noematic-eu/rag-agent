package lexical

import "time"

// RebuildStats reports the outcome of a lexical index rebuild.
type RebuildStats struct {
	ChunksIndexed int
	ChunksTotal   int
	Duration      time.Duration
}

// RebuildProgressFunc is called during RebuildFromChunks (indexed may advance every chunk; total is 0 if unknown).
type RebuildProgressFunc func(indexed, total int)
