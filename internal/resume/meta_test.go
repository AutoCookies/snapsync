package resume

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSaveAndLoadMetaAtomic(t *testing.T) {
	path := filepath.Join(t.TempDir(), "file.partial.snapsync")
	meta := Meta{ExpectedSize: 100, ReceivedOffset: 20, OriginalName: "x.bin"}
	if err := SaveMetaAtomic(path, meta); err != nil {
		t.Fatalf("SaveMetaAtomic() error = %v", err)
	}
	got, err := LoadMeta(path)
	if err != nil {
		t.Fatalf("LoadMeta() error = %v", err)
	}
	if got.ReceivedOffset != 20 || got.ExpectedSize != 100 || got.Version != MetaVersion {
		t.Fatalf("unexpected meta: %#v", got)
	}
}

func TestLoadMetaRejectsCorrupted(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bad.snapsync")
	if err := os.WriteFile(path, []byte("not-json"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if _, err := LoadMeta(path); err == nil {
		t.Fatal("expected LoadMeta to fail for corrupted file")
	}
}
