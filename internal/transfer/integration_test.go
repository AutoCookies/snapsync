package transfer

import (
	"bytes"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSendReceiveIntegritySuccess(t *testing.T) {
	srcDir := t.TempDir()
	dstDir := t.TempDir()
	srcPath := filepath.Join(srcDir, "sample.bin")
	srcData := bytes.Repeat([]byte("0123456789abcdef"), 1024*1280) // 20MB
	if err := os.WriteFile(srcPath, srcData, 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	recvOut := &bytes.Buffer{}
	sendOut := &bytes.Buffer{}
	listenAddr, done := startReceiver(t, dstDir, recvOut)
	sendErr := Send(SenderOptions{Path: srcPath, Address: listenAddr, Out: sendOut})
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
	if !bytes.Equal(got, srcData) {
		t.Fatal("content mismatch")
	}
	if !strings.Contains(sendOut.String(), "Integrity verified") {
		t.Fatalf("expected integrity output on sender, got %q", sendOut.String())
	}
	if !strings.Contains(recvOut.String(), "Integrity verified") {
		t.Fatalf("expected integrity output on receiver, got %q", recvOut.String())
	}
}

func TestReceiverDeletesCorruptedFileOnHashMismatch(t *testing.T) {
	srcDir := t.TempDir()
	dstDir := t.TempDir()
	srcPath := filepath.Join(srcDir, "corrupt.bin")
	srcData := bytes.Repeat([]byte("abcdef0123456789"), 1024*128) // 2MB
	if err := os.WriteFile(srcPath, srcData, 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	prev := senderChunkMutator
	senderChunkMutator = func(chunk []byte) {
		if len(chunk) > 0 {
			chunk[0] ^= 0xFF
			senderChunkMutator = nil
		}
	}
	defer func() { senderChunkMutator = prev }()

	listenAddr, done := startReceiver(t, dstDir, ioDiscard{})
	sendErr := Send(SenderOptions{Path: srcPath, Address: listenAddr, Out: ioDiscard{}})
	recvErr := <-done
	if sendErr == nil {
		t.Fatal("expected sender error due integrity failure")
	}
	if recvErr == nil {
		t.Fatal("expected receiver error due integrity failure")
	}
	if _, statErr := os.Stat(filepath.Join(dstDir, "corrupt.bin")); !os.IsNotExist(statErr) {
		t.Fatalf("expected corrupted file removed, stat err=%v", statErr)
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

func startReceiver(t *testing.T, outDir string, outWriter io.Writer) (string, <-chan error) {
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
		done <- HandleConnection(conn, ReceiverOptions{OutDir: outDir, AutoAccept: true, Out: outWriter})
	}()
	return ln.Addr().String(), done
}
