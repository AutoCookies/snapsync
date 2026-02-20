// Package cli implements SnapSync command-line parsing and commands.
package cli

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"

	"snapsync/internal/discovery"
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
	resolver discovery.Resolver
	sendFunc func(transfer.SenderOptions) error
}

// NewRootCommand creates the SnapSync root command.
func NewRootCommand(out io.Writer, errOut io.Writer, in io.Reader) *RootCommand {
	root := &RootCommand{out: out, errOut: errOut, in: in, resolver: discovery.MDNSResolver{}, sendFunc: transfer.Send}
	root.commands = []Command{
		NewVersionCommand(out),
		{name: "send", run: root.runSend},
		{name: "recv", run: root.runRecv},
		{name: "list", run: root.runList},
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
	case "list":
		return r.commands[3].run(r.args[1:])
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
	const help = "SnapSync is a LAN file transfer tool\n\nUsage:\n  snapsync [command]\n\nAvailable Commands:\n  list     List discovered peers\n  recv     Receive a file over TCP\n  send     Send a file over TCP\n  version  Print version information\n\nFlags:\n  -h, --help  help for snapsync\n"
	if _, err := fmt.Fprint(r.out, help); err != nil {
		return fmt.Errorf("write help output: %w", err)
	}
	return nil
}

func (r *RootCommand) printSendHelp() error {
	const msg = `Usage:
  snapsync send <path> --to <peer-id|host:port> [--timeout 2s] [--name name] [--no-resume]
`
	_, err := fmt.Fprint(r.out, msg)
	return err
}

func (r *RootCommand) printRecvHelp() error {
	const msg = `Usage:
  snapsync recv --listen :45999 --out <dir> [--accept] [--no-discovery] [--no-resume] [--keep-partial] [--force-restart] [--break-lock]
`
	_, err := fmt.Fprint(r.out, msg)
	return err
}

func (r *RootCommand) printListHelp() error {
	const msg = `Usage:
  snapsync list [--timeout 2s] [--json]
`
	_, err := fmt.Fprint(r.out, msg)
	return err
}

func (r *RootCommand) runSend(args []string) error {
	if len(args) == 1 && (args[0] == "--help" || args[0] == "-h") {
		return r.printSendHelp()
	}
	if len(args) == 0 {
		return fmt.Errorf("send requires a file path argument: %w", apperrors.ErrUsage)
	}
	path := filepath.Clean(args[0])
	fs := flag.NewFlagSet("send", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	to := fs.String("to", "", "receiver host:port or peer id")
	name := fs.String("name", "", "override transfer filename")
	timeout := fs.Duration("timeout", 2*time.Second, "discovery timeout")
	noResume := fs.Bool("no-resume", false, "disable resume")
	if err := fs.Parse(args[1:]); err != nil {
		return fmt.Errorf("parse send flags: %w: %w", err, apperrors.ErrUsage)
	}
	if len(fs.Args()) > 0 {
		return fmt.Errorf("send accepts one path followed by flags: %w", apperrors.ErrUsage)
	}
	if *to == "" {
		return fmt.Errorf("send requires --to: %w", apperrors.ErrUsage)
	}

	address := *to
	if !strings.Contains(*to, ":") {
		peer, err := r.resolver.Browse(context.Background(), *timeout)
		if err != nil {
			return fmt.Errorf("discover peers: %w", err)
		}
		found := false
		for _, p := range peer {
			if p.ID == *to {
				best := p.PreferredAddress()
				if best == "" {
					return fmt.Errorf("peer %q has no usable address: %w", p.ID, apperrors.ErrNetwork)
				}
				address = net.JoinHostPort(best, fmt.Sprintf("%d", p.Port))
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("peer id %q not found: %w", *to, apperrors.ErrNetwork)
		}
	}

	if err := r.sendFunc(transfer.SenderOptions{Path: path, Address: address, OverrideName: *name, Out: r.out, Resume: !*noResume}); err != nil {
		return err
	}
	return nil
}

func (r *RootCommand) runRecv(args []string) error {
	if len(args) == 1 && (args[0] == "--help" || args[0] == "-h") {
		return r.printRecvHelp()
	}
	fs := flag.NewFlagSet("recv", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	listen := fs.String("listen", "", "listen address")
	outDir := fs.String("out", "", "output directory")
	overwrite := fs.Bool("overwrite", false, "overwrite existing file")
	autoAccept := fs.Bool("accept", false, "automatically accept incoming transfer")
	alias := fs.String("name", "", "advertised discovery name")
	noDiscovery := fs.Bool("no-discovery", false, "disable mDNS advertisement")
	noResume := fs.Bool("no-resume", false, "disable resume")
	keepPartial := fs.Bool("keep-partial", false, "keep partial files on failure")
	forceRestart := fs.Bool("force-restart", false, "force restart when resume session mismatches")
	breakLock := fs.Bool("break-lock", false, "break existing lock file before receiving")
	if err := fs.Parse(args); err != nil {
		return fmt.Errorf("parse recv flags: %w: %w", err, apperrors.ErrUsage)
	}
	if *listen == "" || *outDir == "" {
		return fmt.Errorf("recv requires --listen and --out: %w", apperrors.ErrUsage)
	}

	peerID, err := discovery.LocalPeerID()
	if err != nil {
		return fmt.Errorf("load local peer id: %w", err)
	}
	display := *alias
	if display == "" {
		h, _ := os.Hostname()
		display = h
	}
	instance := display
	if instance == "" {
		instance = "snapsync"
	}

	opts := transfer.ReceiverOptions{
		Listen:       *listen,
		OutDir:       filepath.Clean(*outDir),
		Overwrite:    *overwrite,
		AutoAccept:   *autoAccept,
		Prompt:       r.promptAccept,
		Out:          r.out,
		Resume:       !*noResume,
		KeepPartial:  *keepPartial,
		ForceRestart: *forceRestart,
		BreakLock:    *breakLock,
	}
	if !*noDiscovery {
		opts.OnListening = func(addr net.Addr) (func(), error) {
			port := 0
			if tcp, ok := addr.(*net.TCPAddr); ok {
				port = tcp.Port
			}
			adv, advErr := discovery.StartAdvertise(discovery.AdvertiseConfig{InstanceName: instance, PeerID: peerID, DisplayName: display, Port: port})
			if advErr != nil {
				return nil, fmt.Errorf("start discovery advertisement: %w", advErr)
			}
			return adv.Stop, nil
		}
	}
	if err := transfer.ReceiveOnce(opts); err != nil {
		return err
	}
	return nil
}

func (r *RootCommand) runList(args []string) error {
	if len(args) == 1 && (args[0] == "--help" || args[0] == "-h") {
		return r.printListHelp()
	}
	fs := flag.NewFlagSet("list", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	timeout := fs.Duration("timeout", 2*time.Second, "discovery timeout")
	jsonOut := fs.Bool("json", false, "print peers as NDJSON")
	if err := fs.Parse(args); err != nil {
		return fmt.Errorf("parse list flags: %w: %w", err, apperrors.ErrUsage)
	}
	peers, err := r.resolver.Browse(context.Background(), *timeout)
	if err != nil {
		return fmt.Errorf("browse peers: %w", err)
	}
	if *jsonOut {
		enc := json.NewEncoder(r.out)
		for _, p := range peers {
			if err := enc.Encode(p); err != nil {
				return fmt.Errorf("encode peer output: %w", err)
			}
		}
		return nil
	}
	if _, err := fmt.Fprintln(r.out, "ID           NAME          ADDRESSES              PORT  AGE"); err != nil {
		return fmt.Errorf("write list header: %w", err)
	}
	now := time.Now()
	for _, p := range peers {
		age := now.Sub(p.LastSeen).Truncate(100 * time.Millisecond)
		if _, err := fmt.Fprintf(r.out, "%-12s %-13s %-22s %-5d %s\n", p.ID, p.Name, strings.Join(p.Addresses, ", "), p.Port, age); err != nil {
			return fmt.Errorf("write list row: %w", err)
		}
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
