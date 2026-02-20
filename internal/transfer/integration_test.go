package transfer

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"testing"
)

func TestSendReceiveIntegration(t *testing.T) {
	srcDir := t.TempDir()
	dstDir := t.TempDir()
	srcPath := filepath.Join(srcDir, "sample.bin")
	srcData := bytes.Repeat([]byte("0123456789abcdef"), 1024*256) // 4MB
	if err := os.WriteFile(srcPath, srcData, 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	listenAddr, done := startReceiver(t, dstDir)
	sendErr := Send(SenderOptions{Path: srcPath, Address: listenAddr, Out: ioDiscard{}})
	recvErr := <-done
	if sendErr != nil {
		t.Fatalf("Send() error = %v", sendErr)
	}
	if recvErr != nil {
		t.Fatalf("receiver error = %v", recvErr)
	}

	dstPath := filepath.Join(dstDir, "sample.bin")
	got, err := os.ReadFile(dstPath)
	if err != nil {
		t.Fatalf("ReadFile(dst) error = %v", err)
	}
	if len(got) != len(srcData) {
		t.Fatalf("size mismatch got %d want %d", len(got), len(srcData))
	}
	if sha256.Sum256(got) != sha256.Sum256(srcData) {
		t.Fatal("content hash mismatch")
	}
}

func TestReceiverDeletesPartialOnEarlyClose(t *testing.T) {
	dstDir := t.TempDir()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen() error = %v", err)
	}
	defer func() { _ = ln.Close() }()

	recvDone := make(chan error, 1)
	go func() {
		conn, acceptErr := ln.Accept()
		if acceptErr != nil {
			recvDone <- acceptErr
			return
		}
		defer func() { _ = conn.Close() }()
		recvDone <- HandleConnection(conn, ReceiverOptions{OutDir: dstDir, AutoAccept: true, Overwrite: false, Out: ioDiscard{}})
	}()

	conn, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatalf("Dial() error = %v", err)
	}
	_ = WriteFrame(conn, Frame{Type: TypeHello})
	offer, _ := EncodeOffer("partial.bin", 1024*1024)
	_ = WriteFrame(conn, Frame{Type: TypeOffer, Payload: offer})
	frame, err := ReadFrame(conn)
	if err != nil || frame.Type != TypeAccept {
		t.Fatalf("expected ACCEPT, got frame=%#v err=%v", frame, err)
	}
	_ = WriteFrame(conn, Frame{Type: TypeData, Payload: bytes.Repeat([]byte("a"), 1024)})
	_ = conn.Close()

	err = <-recvDone
	if err == nil {
		t.Fatal("expected receiver to fail on early close")
	}
	if _, statErr := os.Stat(filepath.Join(dstDir, "partial.bin")); !os.IsNotExist(statErr) {
		t.Fatalf("expected partial file removed, stat err=%v", statErr)
	}
}

type ioDiscard struct{}

func (ioDiscard) Write(p []byte) (int, error) { return len(p), nil }

func startReceiver(t *testing.T, outDir string) (string, <-chan error) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen() error = %v", err)
	}
	done := make(chan error, 1)
	go func() {
		defer func() { _ = ln.Close() }()
		conn, acceptErr := ln.Accept()
		if acceptErr != nil {
			done <- fmt.Errorf("accept connection: %w", acceptErr)
			return
		}
		defer func() { _ = conn.Close() }()
		done <- HandleConnection(conn, ReceiverOptions{OutDir: outDir, AutoAccept: true, Out: ioDiscard{}})
	}()
	return ln.Addr().String(), done
}
