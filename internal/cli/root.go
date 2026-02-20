// Package cli implements SnapSync command-line parsing and commands.
package cli

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	apperrors "snapsync/internal/errors"
	"snapsync/internal/transfer"
)

// Command represents an executable CLI command.
type Command struct {
	name string
	run  func(args []string) error
}

// Name returns the command name.
func (c Command) Name() string { return c.name }

// RootCommand handles argument parsing for the SnapSync CLI.
type RootCommand struct {
	out      io.Writer
	errOut   io.Writer
	in       io.Reader
	commands []Command
	args     []string
}

// NewRootCommand creates the SnapSync root command.
func NewRootCommand(out io.Writer, errOut io.Writer, in io.Reader) *RootCommand {
	root := &RootCommand{out: out, errOut: errOut, in: in}
	root.commands = []Command{
		NewVersionCommand(out),
		{name: "send", run: root.runSend},
		{name: "recv", run: root.runRecv},
	}
	return root
}

// SetArgs sets command arguments.
func (r *RootCommand) SetArgs(args []string) { r.args = args }

// Commands returns configured subcommands.
func (r *RootCommand) Commands() []Command { return r.commands }

// Execute parses and runs commands.
func (r *RootCommand) Execute() error {
	if len(r.args) == 0 {
		return r.printHelp()
	}
	switch r.args[0] {
	case "-h", "--help", "help":
		return r.printHelp()
	case "version":
		return r.commands[0].run(r.args[1:])
	case "send":
		return r.commands[1].run(r.args[1:])
	case "recv":
		return r.commands[2].run(r.args[1:])
	default:
		if _, err := fmt.Fprintf(r.errOut, "unknown command %q\n", r.args[0]); err != nil {
			return fmt.Errorf("write unknown command error: %w", err)
		}
		if err := r.printHelp(); err != nil {
			return err
		}
		return fmt.Errorf("unknown command: %s: %w", r.args[0], apperrors.ErrUsage)
	}
}

func (r *RootCommand) printHelp() error {
	const help = "SnapSync is a LAN file transfer tool\n\nUsage:\n  snapsync [command]\n\nAvailable Commands:\n  recv     Receive a file over TCP\n  send     Send a file over TCP\n  version  Print version information\n\nFlags:\n  -h, --help  help for snapsync\n"
	if _, err := fmt.Fprint(r.out, help); err != nil {
		return fmt.Errorf("write help output: %w", err)
	}
	return nil
}

func (r *RootCommand) runSend(args []string) error {
	fs := flag.NewFlagSet("send", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	to := fs.String("to", "", "receiver host:port")
	name := fs.String("name", "", "override transfer filename")
	if err := fs.Parse(args); err != nil {
		return fmt.Errorf("parse send flags: %w: %w", err, apperrors.ErrUsage)
	}
	remaining := fs.Args()
	if len(remaining) != 1 {
		return fmt.Errorf("send requires exactly one file path argument: %w", apperrors.ErrUsage)
	}
	if *to == "" {
		return fmt.Errorf("send requires --to host:port: %w", apperrors.ErrUsage)
	}
	path := filepath.Clean(remaining[0])
	if err := transfer.Send(transfer.SenderOptions{Path: path, Address: *to, OverrideName: *name, Out: r.out}); err != nil {
		return err
	}
	return nil
}

func (r *RootCommand) runRecv(args []string) error {
	fs := flag.NewFlagSet("recv", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	listen := fs.String("listen", "", "listen address")
	outDir := fs.String("out", "", "output directory")
	overwrite := fs.Bool("overwrite", false, "overwrite existing file")
	autoAccept := fs.Bool("accept", false, "automatically accept incoming transfer")
	if err := fs.Parse(args); err != nil {
		return fmt.Errorf("parse recv flags: %w: %w", err, apperrors.ErrUsage)
	}
	if *listen == "" || *outDir == "" {
		return fmt.Errorf("recv requires --listen and --out: %w", apperrors.ErrUsage)
	}
	opts := transfer.ReceiverOptions{
		Listen:     *listen,
		OutDir:     filepath.Clean(*outDir),
		Overwrite:  *overwrite,
		AutoAccept: *autoAccept,
		Prompt:     r.promptAccept,
		Out:        r.out,
	}
	if err := transfer.ReceiveOnce(opts); err != nil {
		return err
	}
	return nil
}

func (r *RootCommand) promptAccept(name string, size uint64, peer string) (bool, error) {
	if _, err := fmt.Fprintf(r.out, "Accept file %s (%d bytes) from %s? [y/N] ", name, size, peer); err != nil {
		return false, fmt.Errorf("write accept prompt: %w", err)
	}
	reader := bufio.NewReader(r.in)
	line, err := reader.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return false, fmt.Errorf("read accept prompt input: %w", err)
	}
	value := strings.TrimSpace(strings.ToLower(line))
	return value == "y" || value == "yes", nil
}

// NewOSRootCommand creates a command wired to process standard streams.
func NewOSRootCommand() *RootCommand {
	return NewRootCommand(os.Stdout, os.Stderr, os.Stdin)
}
