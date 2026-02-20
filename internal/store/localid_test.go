package store

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestLoadOrCreatePeerIDPersistsValue(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("path behavior differs on windows in this environment")
	}
	home := t.TempDir()
	if err := os.Setenv("HOME", home); err != nil {
		t.Fatalf("Setenv(HOME) error = %v", err)
	}
	want := "abc123def456"
	id, err := LoadOrCreatePeerID(func() (string, error) { return want, nil })
	if err != nil {
		t.Fatalf("LoadOrCreatePeerID() error = %v", err)
	}
	if id != want {
		t.Fatalf("id mismatch got %q want %q", id, want)
	}
	id2, err := LoadOrCreatePeerID(func() (string, error) { return "other", nil })
	if err != nil {
		t.Fatalf("LoadOrCreatePeerID() second call error = %v", err)
	}
	if id2 != want {
		t.Fatalf("expected persisted id %q, got %q", want, id2)
	}
	if _, err := os.Stat(filepath.Join(home, ".config", "snapsync", "peer_id")); err != nil {
		t.Fatalf("expected peer_id file to exist: %v", err)
	}
}
