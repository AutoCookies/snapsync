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
	for _, required := range []string{"version", "send", "recv", "list"} {
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

	out := buf.String()
	if !strings.Contains(out, "list") || !strings.Contains(out, "send") || !strings.Contains(out, "recv") {
		t.Fatalf("expected command names in help output, got: %q", out)
	}
}

func TestSendHelpIncludesRequiredFlags(t *testing.T) {
	buf := &bytes.Buffer{}
	root := NewRootCommand(buf, buf, strings.NewReader(""))
	root.SetArgs([]string{"send", "--help"})
	if err := root.Execute(); err != nil {
		t.Fatalf("expected send --help to succeed, got error: %v", err)
	}
	out := buf.String()
	for _, token := range []string{"--to", "--timeout", "--name", "--no-resume"} {
		if !strings.Contains(out, token) {
			t.Fatalf("expected token %q in help output: %q", token, out)
		}
	}
}
