package lexical

import "testing"

func TestParseEngine(t *testing.T) {
	for _, in := range []string{"bleve", "TANTIVY", "f4kvs"} {
		if _, err := ParseEngine(in); err != nil {
			t.Fatalf("%s: %v", in, err)
		}
	}
	if _, err := ParseEngine("elastic"); err == nil {
		t.Fatal("expected error")
	}
}
