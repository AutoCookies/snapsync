// Package cli implements SnapSync command-line parsing and commands.
package cli

import (
	"fmt"
	"io"
)

// Command represents an executable CLI command.
type Command struct {
	name string
	run  func(args []string) error
}

// Name returns the command name.
func (c Command) Name() string {
	return c.name
}

// RootCommand handles argument parsing for the SnapSync CLI.
type RootCommand struct {
	out        io.Writer
	errOut     io.Writer
	commands   []Command
	args       []string
	helpOutput string
}

// NewRootCommand creates the SnapSync root command.
func NewRootCommand(out io.Writer, errOut io.Writer) *RootCommand {
	versionCommand := NewVersionCommand(out)

	return &RootCommand{
		out:    out,
		errOut: errOut,
		commands: []Command{
			versionCommand,
		},
		helpOutput: "SnapSync is a LAN file transfer tool\n\nUsage:\n  snapsync [command]\n\nAvailable Commands:\n  version\tPrint version information\n\nFlags:\n  -h, --help\thelp for snapsync\n",
	}
}

// SetArgs sets command arguments.
func (r *RootCommand) SetArgs(args []string) {
	r.args = args
}

// Commands returns configured subcommands.
func (r *RootCommand) Commands() []Command {
	return r.commands
}

// Execute parses and runs commands.
func (r *RootCommand) Execute() error {
	if len(r.args) == 0 {
		_, err := fmt.Fprint(r.out, r.helpOutput)
		if err != nil {
			return fmt.Errorf("write help output: %w", err)
		}
		return nil
	}

	switch r.args[0] {
	case "-h", "--help", "help":
		_, err := fmt.Fprint(r.out, r.helpOutput)
		if err != nil {
			return fmt.Errorf("write help output: %w", err)
		}
		return nil
	case "version":
		return r.commands[0].run(r.args[1:])
	default:
		_, err := fmt.Fprintf(r.errOut, "unknown command %q\n", r.args[0])
		if err != nil {
			return fmt.Errorf("write unknown command error: %w", err)
		}
		_, err = fmt.Fprint(r.out, r.helpOutput)
		if err != nil {
			return fmt.Errorf("write help output: %w", err)
		}
		return fmt.Errorf("unknown command: %s", r.args[0])
	}
}
