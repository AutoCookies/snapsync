package transfer

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"

	apperrors "snapsync/internal/errors"
	"snapsync/internal/progress"
	"snapsync/internal/sanitize"
)

// PromptFunc asks user whether to accept transfer.
type PromptFunc func(name string, size uint64, peer string) (bool, error)

// ReceiverOptions configures receiver behavior.
type ReceiverOptions struct {
	Listen      string
	OutDir      string
	Overwrite   bool
	AutoAccept  bool
	Prompt      PromptFunc
	Out         io.Writer
	OnListening func(addr net.Addr) (func(), error)
}

// ReceiveOnce listens and handles a single transfer.
func ReceiveOnce(opts ReceiverOptions) error {
	if opts.Listen == "" || opts.OutDir == "" {
		return fmt.Errorf("missing required receiver options: %w", apperrors.ErrUsage)
	}
	if opts.Out == nil {
		opts.Out = io.Discard
	}
	if err := os.MkdirAll(opts.OutDir, 0o755); err != nil {
		return fmt.Errorf("create output dir: %w: %w", err, apperrors.ErrIO)
	}

	ln, err := net.Listen("tcp", opts.Listen)
	if err != nil {
		return fmt.Errorf("listen on %s: %w: %w", opts.Listen, err, apperrors.ErrNetwork)
	}
	defer func() { _ = ln.Close() }()
	var stopAdvertise func()
	if opts.OnListening != nil {
		cleanup, cbErr := opts.OnListening(ln.Addr())
		if cbErr != nil {
			return fmt.Errorf("receiver on-listening callback: %w", cbErr)
		}
		stopAdvertise = cleanup
	}
	if stopAdvertise != nil {
		defer stopAdvertise()
	}
	_, _ = fmt.Fprintf(opts.Out, "listening on %s\n", ln.Addr().String())

	conn, err := ln.Accept()
	if err != nil {
		return fmt.Errorf("accept connection: %w: %w", err, apperrors.ErrNetwork)
	}
	defer func() { _ = conn.Close() }()

	return HandleConnection(conn, opts)
}

// HandleConnection processes one transfer session on accepted connection.
func HandleConnection(conn net.Conn, opts ReceiverOptions) error {
	reader := bufio.NewReader(conn)
	writer := bufio.NewWriter(conn)
	peer := conn.RemoteAddr().String()

	hello, err := ReadFrame(reader)
	if err != nil {
		return fmt.Errorf("read hello frame: %w", err)
	}
	if hello.Type != TypeHello {
		return sendProtocolError(writer, fmt.Sprintf("expected HELLO, got %d", hello.Type))
	}
	offerFrame, err := ReadFrame(reader)
	if err != nil {
		return fmt.Errorf("read offer frame: %w", err)
	}
	if offerFrame.Type != TypeOffer {
		return sendProtocolError(writer, fmt.Sprintf("expected OFFER, got %d", offerFrame.Type))
	}
	offer, err := DecodeOffer(offerFrame.Payload)
	if err != nil {
		_ = sendProtocolError(writer, "invalid offer payload")
		return fmt.Errorf("decode offer: %w", err)
	}

	accept := opts.AutoAccept
	if !opts.AutoAccept {
		if opts.Prompt == nil {
			accept = false
		} else {
			choice, promptErr := opts.Prompt(offer.Name, offer.Size, peer)
			if promptErr != nil {
				_ = sendErrorFrame(writer, "receiver prompt failed")
				return fmt.Errorf("prompt accept transfer: %w", promptErr)
			}
			accept = choice
		}
	}
	if !accept {
		if err := sendErrorFrame(writer, "transfer rejected"); err != nil {
			return fmt.Errorf("send reject frame: %w", err)
		}
		return fmt.Errorf("transfer rejected by receiver: %w", apperrors.ErrRejected)
	}

	if err := WriteFrame(writer, Frame{Type: TypeAccept}); err != nil {
		return fmt.Errorf("send accept frame: %w", err)
	}
	if err := writer.Flush(); err != nil {
		return fmt.Errorf("flush accept frame: %w: %w", err, apperrors.ErrNetwork)
	}

	outPath, err := sanitize.ResolveCollisionPath(opts.OutDir, offer.Name, opts.Overwrite)
	if err != nil {
		_ = sendErrorFrame(writer, "unable to create output path")
		return fmt.Errorf("resolve output path: %w: %w", err, apperrors.ErrIO)
	}

	file, err := os.OpenFile(filepath.Clean(outPath), os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		_ = sendErrorFrame(writer, "unable to open output file")
		return fmt.Errorf("open output file: %w: %w", err, apperrors.ErrIO)
	}

	cleanup := true
	defer func() {
		_ = file.Close()
		if cleanup {
			_ = os.Remove(outPath)
		}
	}()

	reporter := progress.NewReporter(opts.Out, "receiving", offer.Size)
	var written uint64
	for written < offer.Size {
		frame, readErr := ReadFrame(reader)
		if readErr != nil {
			return fmt.Errorf("read data frame: %w: %w", readErr, apperrors.ErrNetwork)
		}
		if frame.Type == TypeError {
			msg, _ := DecodeError(frame.Payload)
			return fmt.Errorf("sender reported error: %s: %w", msg, apperrors.ErrNetwork)
		}
		if frame.Type != TypeData {
			_ = sendErrorFrame(writer, "expected DATA frame")
			return fmt.Errorf("expected DATA frame, got %d: %w", frame.Type, apperrors.ErrInvalidProtocol)
		}
		if written+uint64(len(frame.Payload)) > offer.Size {
			_ = sendErrorFrame(writer, "received more data than offered")
			return fmt.Errorf("received more bytes than expected: %w", apperrors.ErrInvalidProtocol)
		}
		n, writeErr := file.Write(frame.Payload)
		if writeErr != nil {
			_ = sendErrorFrame(writer, "receiver failed writing file")
			return fmt.Errorf("write output file: %w: %w", writeErr, apperrors.ErrIO)
		}
		if n != len(frame.Payload) {
			_ = sendErrorFrame(writer, "receiver short write")
			return fmt.Errorf("short write to output file: %w", apperrors.ErrIO)
		}
		written += uint64(n)
		reporter.Update(written)
	}

	done, err := ReadFrame(reader)
	if err != nil {
		_ = sendErrorFrame(writer, "missing DONE frame")
		return fmt.Errorf("read done frame: %w: %w", err, apperrors.ErrNetwork)
	}
	if done.Type != TypeDone {
		_ = sendErrorFrame(writer, "expected DONE frame")
		return fmt.Errorf("expected DONE frame, got %d: %w", done.Type, apperrors.ErrInvalidProtocol)
	}
	if err := file.Sync(); err != nil {
		return fmt.Errorf("sync output file: %w: %w", err, apperrors.ErrIO)
	}
	cleanup = false
	reporter.Done(written, outPath)
	return nil
}

func sendErrorFrame(w *bufio.Writer, message string) error {
	payload, err := EncodeError(message)
	if err != nil {
		return fmt.Errorf("encode error frame payload: %w", err)
	}
	if err := WriteFrame(w, Frame{Type: TypeError, Payload: payload}); err != nil {
		return fmt.Errorf("write error frame: %w", err)
	}
	if err := w.Flush(); err != nil {
		return fmt.Errorf("flush error frame: %w", err)
	}
	return nil
}

func sendProtocolError(w *bufio.Writer, message string) error {
	if err := sendErrorFrame(w, message); err != nil {
		return fmt.Errorf("send protocol error frame: %w", err)
	}
	return fmt.Errorf("%s: %w", message, apperrors.ErrInvalidProtocol)
}
