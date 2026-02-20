package buildinfo

import (
	"runtime"
	"strings"
	"testing"
)

func TestGetDefaultsWhenBuildFlagsEmpty(t *testing.T) {
	previousVersion, previousCommit, previousDate := Version, Commit, Date
	t.Cleanup(func() {
		Version, Commit, Date = previousVersion, previousCommit, previousDate
	})

	Version, Commit, Date = "", "", ""

	info := Get()
	if info.Version != "dev" {
		t.Fatalf("expected default version dev, got %q", info.Version)
	}
	if info.Commit != "unknown" {
		t.Fatalf("expected default commit unknown, got %q", info.Commit)
	}
	if info.Date != "unknown" {
		t.Fatalf("expected default date unknown, got %q", info.Date)
	}
}

func TestInfoStringHasStableFormat(t *testing.T) {
	info := Info{
		Version: "v1.2.3",
		Commit:  "abc123",
		Date:    "2026-01-01T00:00:00Z",
		Go:      runtime.Version(),
		OS:      runtime.GOOS,
		Arch:    runtime.GOARCH,
	}

	output := info.String()
	lines := strings.Split(output, "\n")
	if len(lines) != 5 {
		t.Fatalf("expected 5 lines, got %d: %q", len(lines), output)
	}

	expectedPrefixes := []string{
		"SnapSync ",
		"commit: ",
		"built:  ",
		"go:     ",
		"os/arch:",
	}

	for i, prefix := range expectedPrefixes {
		if !strings.HasPrefix(lines[i], prefix) {
			t.Fatalf("line %d expected prefix %q, got %q", i+1, prefix, lines[i])
		}
	}
}
