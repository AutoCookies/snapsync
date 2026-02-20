package cli

import (
	"bytes"
	"testing"
)

func TestRootCommandIncludesVersionSubcommand(t *testing.T) {
	buf := &bytes.Buffer{}
	root := NewRootCommand(buf, buf)

	found := false
	for _, command := range root.Commands() {
		if command.Name() == "version" {
			found = true
			break
		}
	}

	if !found {
		t.Fatal("expected root command to include version subcommand")
	}
}

func TestRootCommandHelpReturnsZero(t *testing.T) {
	buf := &bytes.Buffer{}
	root := NewRootCommand(buf, buf)
	root.SetArgs([]string{"--help"})

	if err := root.Execute(); err != nil {
		t.Fatalf("expected help command to succeed, got error: %v", err)
	}

	if buf.Len() == 0 {
		t.Fatal("expected help output")
	}
}
