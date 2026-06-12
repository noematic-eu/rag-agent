//go:build ignore

package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/noematic-eu/ai-rag-agent/internal/f4kvs"
)

func main() {
	path := "/data/legal.f4kvs"
	if len(os.Args) > 1 {
		path = os.Args[1]
	}
	store, err := f4kvs.Open(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "open: %v\n", err)
		os.Exit(1)
	}
	defer store.Close()

	counts := map[string]int{}
	for _, prefix := range []string{"chunk:", "lex:", "embed:", "doc:", "meta:"} {
		pairs, err := store.ScanPrefix(prefix)
		if err != nil {
			fmt.Fprintf(os.Stderr, "scan %s: %v\n", prefix, err)
			continue
		}
		counts[prefix] = len(pairs)
	}

	type embedMeta struct {
		Count int `json:"count"`
	}
	if data, err := store.Get("embed:meta"); err == nil {
		var m embedMeta
		_ = json.Unmarshal(data, &m)
		counts["embed_meta_count"] = m.Count
	}

	fmt.Printf("%+#v\n", counts)
}
