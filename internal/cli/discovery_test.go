package cli

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"snapsync/internal/discovery"
	"snapsync/internal/transfer"
)

type fakeResolver struct {
	peers []discovery.Peer
}

func (f fakeResolver) Browse(_ context.Context, _ time.Duration) ([]discovery.Peer, error) {
	return f.peers, nil
}
func (f fakeResolver) ResolveByID(_ context.Context, _ string) (discovery.Peer, error) {
	return discovery.Peer{}, nil
}

func TestSendPeerIDResolvesAndCallsTransfer(t *testing.T) {
	buf := &bytes.Buffer{}
	root := NewRootCommand(buf, buf, strings.NewReader(""))
	root.resolver = fakeResolver{peers: []discovery.Peer{{ID: "peer1", Addresses: []string{"192.168.1.10"}, Port: 45999}}}
	called := false
	root.sendFunc = func(opts transfer.SenderOptions) error {
		called = true
		if opts.Address != "192.168.1.10:45999" {
			t.Fatalf("address mismatch got %q", opts.Address)
		}
		return nil
	}
	root.SetArgs([]string{"send", "./file.bin", "--to", "peer1"})
	if err := root.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !called {
		t.Fatal("expected sendFunc to be called")
	}
}

func TestSendHostPortBypassesResolver(t *testing.T) {
	buf := &bytes.Buffer{}
	root := NewRootCommand(buf, buf, strings.NewReader(""))
	root.resolver = fakeResolver{peers: nil}
	root.sendFunc = func(opts transfer.SenderOptions) error {
		if opts.Address != "10.0.0.5:45999" {
			t.Fatalf("address mismatch got %q", opts.Address)
		}
		return nil
	}
	root.SetArgs([]string{"send", "./file.bin", "--to", "10.0.0.5:45999"})
	if err := root.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
}

func TestListPrintsPeers(t *testing.T) {
	buf := &bytes.Buffer{}
	root := NewRootCommand(buf, buf, strings.NewReader(""))
	root.resolver = fakeResolver{peers: []discovery.Peer{{ID: "abc123def456", Name: "Laptop", Addresses: []string{"192.168.1.23"}, Port: 45999, LastSeen: time.Now()}}}
	root.SetArgs([]string{"list", "--timeout", "1s"})
	if err := root.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "ID") || !strings.Contains(out, "abc123def456") {
		t.Fatalf("unexpected list output: %q", out)
	}
}
