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

	"snapsync/internal/resume"
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
	listenAddr, done := startReceiver(t, ReceiverOptions{OutDir: dstDir, AutoAccept: true, Resume: true, Out: recvOut})
	sendErr := Send(SenderOptions{Path: srcPath, Address: listenAddr, Resume: true, Out: sendOut})
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

func TestResumeSuccessAfterInterruption(t *testing.T) {
	srcDir := t.TempDir()
	dstDir := t.TempDir()
	srcPath := filepath.Join(srcDir, "resume.bin")
	srcData := bytes.Repeat([]byte("abcdefghijklmnop"), 1024*3200) // 50MB
	if err := os.WriteFile(srcPath, srcData, 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	listenAddr1, done1 := startReceiver(t, ReceiverOptions{OutDir: dstDir, AutoAccept: true, Resume: true, KeepPartial: false, Out: ioDiscard{}})
	if err := sendPartial(srcPath, listenAddr1, 20*1024*1024); err != nil {
		t.Fatalf("sendPartial() error = %v", err)
	}
	err := <-done1
	if err == nil {
		t.Fatal("expected first receiver run to fail on interrupted transfer")
	}

	metaMatches, err := filepath.Glob(filepath.Join(dstDir, "*.partial.snapsync"))
	if err != nil || len(metaMatches) == 0 {
		t.Fatalf("expected resume metadata file, err=%v matches=%v", err, metaMatches)
	}
	paths := resume.Paths{Meta: metaMatches[0], Partial: strings.TrimSuffix(metaMatches[0], ".snapsync"), Final: strings.TrimSuffix(strings.TrimSuffix(metaMatches[0], ".snapsync"), ".partial")}
	meta, err := resume.LoadMeta(paths.Meta)
	if err != nil {
		t.Fatalf("LoadMeta() error = %v", err)
	}
	if meta.ReceivedOffset == 0 {
		t.Fatal("expected non-zero resume offset after interruption")
	}

	listenAddr2, done2 := startReceiver(t, ReceiverOptions{OutDir: dstDir, AutoAccept: true, Resume: true, KeepPartial: false, Out: ioDiscard{}})
	sendOut := &bytes.Buffer{}
	sendErr := Send(SenderOptions{Path: srcPath, Address: listenAddr2, Resume: true, Out: sendOut})
	recvErr := <-done2
	if sendErr != nil {
		t.Fatalf("Send() resume error = %v", sendErr)
	}
	if recvErr != nil {
		t.Fatalf("receiver resume error = %v", recvErr)
	}
	if !strings.Contains(sendOut.String(), "Resuming at offset") {
		t.Fatalf("expected sender to report resume, got %q", sendOut.String())
	}

	finalData, err := os.ReadFile(paths.Final)
	if err != nil {
		t.Fatalf("ReadFile(final) error = %v", err)
	}
	if !bytes.Equal(finalData, srcData) {
		t.Fatal("final file mismatch after resume")
	}
	if _, err := os.Stat(paths.Partial); !os.IsNotExist(err) {
		t.Fatalf("expected partial removed, stat err=%v", err)
	}
	if _, err := os.Stat(paths.Meta); !os.IsNotExist(err) {
		t.Fatalf("expected meta removed, stat err=%v", err)
	}
}

func TestResumeRejectMismatchState(t *testing.T) {
	dstDir := t.TempDir()
	paths, err := resume.ResolvePaths(dstDir, "bad.bin", false)
	if err != nil {
		t.Fatalf("ResolvePaths() error = %v", err)
	}
	if err := os.WriteFile(paths.Partial, bytes.Repeat([]byte("x"), 1024), 0o644); err != nil {
		t.Fatalf("WriteFile(partial) error = %v", err)
	}
	if err := resume.SaveMetaAtomic(paths.Meta, resume.Meta{ExpectedSize: 9999, ReceivedOffset: 512, OriginalName: "bad.bin"}); err != nil {
		t.Fatalf("SaveMetaAtomic() error = %v", err)
	}

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen() error = %v", err)
	}
	defer func() { _ = ln.Close() }()
	done := make(chan error, 1)
	go func() {
		conn, acceptErr := ln.Accept()
		if acceptErr != nil {
			done <- acceptErr
			return
		}
		defer func() { _ = conn.Close() }()
		done <- HandleConnection(conn, ReceiverOptions{OutDir: dstDir, AutoAccept: true, Resume: true, KeepPartial: false, Out: ioDiscard{}})
	}()

	conn, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatalf("Dial() error = %v", err)
	}
	defer func() { _ = conn.Close() }()
	_ = WriteFrame(conn, Frame{Type: TypeHello})
	offer, _ := EncodeOffer("bad.bin", 1024)
	_ = WriteFrame(conn, Frame{Type: TypeOffer, Payload: offer})
	accept, err := ReadFrame(conn)
	if err != nil {
		t.Fatalf("ReadFrame(accept) error = %v", err)
	}
	offset, err := DecodeAccept(accept.Payload)
	if err != nil {
		t.Fatalf("DecodeAccept() error = %v", err)
	}
	if offset != 0 {
		t.Fatalf("expected restart offset 0 on mismatch, got %d", offset)
	}
	_ = conn.Close()
	<-done
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

	listenAddr, done := startReceiver(t, ReceiverOptions{OutDir: dstDir, AutoAccept: true, Resume: true, Out: ioDiscard{}})
	sendErr := Send(SenderOptions{Path: srcPath, Address: listenAddr, Resume: true, Out: ioDiscard{}})
	recvErr := <-done
	if sendErr == nil {
		t.Fatal("expected sender error due integrity failure")
	}
	if recvErr == nil {
		t.Fatal("expected receiver error due integrity failure")
	}
	paths, _ := resume.ResolvePaths(dstDir, "corrupt.bin", false)
	if _, statErr := os.Stat(paths.Partial); !os.IsNotExist(statErr) {
		t.Fatalf("expected corrupted partial removed, stat err=%v", statErr)
	}
	if _, statErr := os.Stat(paths.Meta); !os.IsNotExist(statErr) {
		t.Fatalf("expected corrupted meta removed, stat err=%v", statErr)
	}
}

type ioDiscard struct{}

func (ioDiscard) Write(p []byte) (int, error) { return len(p), nil }

func startReceiver(t *testing.T, opts ReceiverOptions) (string, <-chan error) {
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
		done <- HandleConnection(conn, opts)
	}()
	return ln.Addr().String(), done
}

func sendPartial(path, addr string, cutoff int64) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer func() { _ = file.Close() }()
	info, err := file.Stat()
	if err != nil {
		return err
	}
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		return err
	}
	defer func() { _ = conn.Close() }()
	_ = WriteFrame(conn, Frame{Type: TypeHello})
	offer, _ := EncodeOffer(info.Name(), uint64(info.Size()))
	_ = WriteFrame(conn, Frame{Type: TypeOffer, Payload: offer})
	accept, err := ReadFrame(conn)
	if err != nil {
		return err
	}
	if accept.Type != TypeAccept {
		return fmt.Errorf("expected accept frame")
	}
	buf := make([]byte, MaxChunkSize)
	var sent int64
	for sent < cutoff {
		n, rerr := file.Read(buf)
		if n > 0 {
			if err := WriteFrame(conn, Frame{Type: TypeData, Payload: buf[:n]}); err != nil {
				return err
			}
			sent += int64(n)
		}
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			return rerr
		}
	}
	return nil
}
