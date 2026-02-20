package app

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"

	"snapsync/internal/buildinfo"
)

func TestRunVersionReturnsZeroAndPrintsExpectedLines(t *testing.T) {
	previousVersion, previousCommit, previousDate := buildinfo.Version, buildinfo.Commit, buildinfo.Date
	t.Cleanup(func() {
		buildinfo.Version, buildinfo.Commit, buildinfo.Date = previousVersion, previousCommit, previousDate
	})
	buildinfo.Version = "v0.0.1"
	buildinfo.Commit = "deadbeef"
	buildinfo.Date = "2026-02-01T00:00:00Z"

	output, err := captureStdout(func() int {
		application := New()
		return application.Run([]string{"version"})
	})
	if err != nil {
		t.Fatalf("capture stdout: %v", err)
	}

	if output.exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d", output.exitCode)
	}

	lines := strings.Split(strings.TrimSpace(output.stdout), "\n")
	if len(lines) != 5 {
		t.Fatalf("expected 5 lines, got %d; output=%q", len(lines), output.stdout)
	}

	expectedPrefixes := []string{"SnapSync ", "commit: ", "built:  ", "go:     ", "os/arch:"}
	for i, prefix := range expectedPrefixes {
		if !strings.HasPrefix(lines[i], prefix) {
			t.Fatalf("line %d expected prefix %q, got %q", i+1, prefix, lines[i])
		}
	}
}

type runOutput struct {
	exitCode int
	stdout   string
}

func captureStdout(run func() int) (runOutput, error) {
	originalStdout := os.Stdout
	reader, writer, err := os.Pipe()
	if err != nil {
		return runOutput{}, err
	}

	os.Stdout = writer
	exitCode := run()
	_ = writer.Close()
	os.Stdout = originalStdout

	var buffer bytes.Buffer
	if _, err := io.Copy(&buffer, reader); err != nil {
		return runOutput{}, err
	}

	return runOutput{exitCode: exitCode, stdout: buffer.String()}, nil
}
