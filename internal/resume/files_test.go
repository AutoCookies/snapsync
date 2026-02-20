package resume

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolvePathsAndFinalize(t *testing.T) {
	dir := t.TempDir()
	paths, err := ResolvePaths(dir, "movie.mkv", false)
	if err != nil {
		t.Fatalf("ResolvePaths() error = %v", err)
	}
	if !strings.HasSuffix(paths.Partial, ".partial") || !strings.HasSuffix(paths.Meta, ".partial.snapsync") {
		t.Fatalf("unexpected paths: %#v", paths)
	}
	if err := os.WriteFile(paths.Partial, []byte("x"), 0o644); err != nil {
		t.Fatalf("WriteFile(partial) error = %v", err)
	}
	if err := os.WriteFile(paths.Meta, []byte("{}"), 0o644); err != nil {
		t.Fatalf("WriteFile(meta) error = %v", err)
	}
	if err := Finalize(paths); err != nil {
		t.Fatalf("Finalize() error = %v", err)
	}
	if _, err := os.Stat(paths.Final); err != nil {
		t.Fatalf("expected final file: %v", err)
	}
	if _, err := os.Stat(paths.Meta); !os.IsNotExist(err) {
		t.Fatalf("expected meta removed, stat err=%v", err)
	}
}

func TestResolvePathsCollision(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "file.txt"), []byte("x"), 0o644)
	paths, err := ResolvePaths(dir, "file.txt", false)
	if err != nil {
		t.Fatalf("ResolvePaths() error = %v", err)
	}
	if !strings.Contains(paths.Final, "file (1).txt") {
		t.Fatalf("expected collision suffix, got %s", paths.Final)
	}
}
