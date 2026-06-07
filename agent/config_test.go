package main

import (
	"path/filepath"
	"testing"
)

func TestResolveAgentConfigDefaults(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)

	cfg, err := resolveAgentConfig("", "", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Listen != defaultListen {
		t.Fatalf("listen: got %q want %q", cfg.Listen, defaultListen)
	}
	abs, _ := filepath.Abs(defaultDataDir)
	if cfg.DataDir != abs {
		t.Fatalf("data-dir: got %q want %q", cfg.DataDir, abs)
	}
}

func TestResolveAgentConfigFlagOverridesEnv(t *testing.T) {
	t.Setenv("RAG_LISTEN", ":9999")
	t.Setenv("RAG_DATA_DIR", "/should/not/use")

	dir := t.TempDir()
	cfg, err := resolveAgentConfig("127.0.0.1:8081", "", dir, "bleve")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Listen != "127.0.0.1:8081" {
		t.Fatalf("listen: got %q", cfg.Listen)
	}
	abs, _ := filepath.Abs(dir)
	if cfg.DataDir != abs {
		t.Fatalf("data-dir: got %q want %q", cfg.DataDir, abs)
	}
	if cfg.blevePath() != filepath.Join(abs, "legal.bleve") {
		t.Fatalf("bleve path: %s", cfg.blevePath())
	}
}
