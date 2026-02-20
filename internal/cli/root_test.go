package cli

import (
	"bytes"
	"strings"
	"testing"
)

func TestRootCommandIncludesRequiredSubcommands(t *testing.T) {
	buf := &bytes.Buffer{}
	root := NewRootCommand(buf, buf, strings.NewReader(""))

	names := map[string]bool{}
	for _, command := range root.Commands() {
		names[command.Name()] = true
	}
	for _, required := range []string{"version", "send", "recv"} {
		if !names[required] {
			t.Fatalf("expected root command to include %q subcommand", required)
		}
	}
}

func TestRootCommandHelpReturnsZero(t *testing.T) {
	buf := &bytes.Buffer{}
	root := NewRootCommand(buf, buf, strings.NewReader(""))
	root.SetArgs([]string{"--help"})

	if err := root.Execute(); err != nil {
		t.Fatalf("expected help command to succeed, got error: %v", err)
	}

	if buf.Len() == 0 {
		t.Fatal("expected help output")
	}
}
