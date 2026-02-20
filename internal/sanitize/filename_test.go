package sanitize

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSafeFileNameReplacesWindowsInvalidChars(t *testing.T) {
	got := SafeFileName(`bad<>:"/\\|?*.txt`)
	if strings.ContainsAny(got, `<>:"/\\|?*`) {
		t.Fatalf("sanitized name still contains invalid chars: %q", got)
	}
	if got == "" {
		t.Fatal("sanitized name should not be empty")
	}
}

func TestResolveCollisionPath(t *testing.T) {
	dir := t.TempDir()
	first := filepath.Join(dir, "file.txt")
	if err := os.WriteFile(first, []byte("x"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	got, err := ResolveCollisionPath(dir, "file.txt", false)
	if err != nil {
		t.Fatalf("ResolveCollisionPath() error = %v", err)
	}
	if !strings.HasSuffix(got, "file (1).txt") {
		t.Fatalf("expected collision suffix, got %q", got)
	}
}
