//go:build !tantivy

package lexical

import "fmt"

func openTantivy(cfg Config) (Backend, error) {
	return nil, fmt.Errorf("tantivy engine requires build tag tantivy and native libs (make tantivy)")
}
