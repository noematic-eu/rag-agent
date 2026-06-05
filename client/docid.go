package main

import (
	"crypto/sha1"
	"encoding/hex"
)

// StableDocID returns the document id used by directory ingest for a relative path.
func StableDocID(relPath string) string {
	sum := sha1.Sum([]byte(relPath))
	return "doc-" + hex.EncodeToString(sum[:])
}
