package p9fs

import "testing"

func TestParsePlan9Addr(t *testing.T) {
	tests := []struct {
		in      string
		network string
		address string
	}{
		{"unix!/tmp/rag9p", "unix", "/tmp/rag9p"},
		{"tcp!127.0.0.1!5640", "tcp", "127.0.0.1:5640"},
		{"127.0.0.1:9000", "tcp", "127.0.0.1:9000"},
	}
	for _, tc := range tests {
		network, address, err := parsePlan9Addr(tc.in)
		if err != nil {
			t.Fatalf("parsePlan9Addr(%q): %v", tc.in, err)
		}
		if network != tc.network || address != tc.address {
			t.Fatalf("parsePlan9Addr(%q) = %q,%q; want %q,%q", tc.in, network, address, tc.network, tc.address)
		}
	}
}

func TestParseParamsText(t *testing.T) {
	params := parseParamsText("corpus=legal\ntop_k=4\n# comment\n\nlang=fr\n")
	if params["corpus"] != "legal" || params["top_k"] != "4" || params["lang"] != "fr" {
		t.Fatalf("unexpected params: %#v", params)
	}
}
