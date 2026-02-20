package transfer

import (
	"bufio"
	"crypto/subtle"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"

	apperrors "snapsync/internal/errors"
	"snapsync/internal/hash"
	"snapsync/internal/progress"
	"snapsync/internal/resume"
)

const resumeMetaUpdateBytes = 4 * 1024 * 1024

// PromptFunc asks user whether to accept a transfer.
type PromptFunc func(name string, size uint64, peer string) (bool, error)

// ReceiverOptions configures receiver behavior.
type ReceiverOptions struct {
	Listen       string
	OutDir       string
	Overwrite    bool
	AutoAccept   bool
	Prompt       PromptFunc
	Out          io.Writer
	OnListening  func(addr net.Addr) (func(), error)
	Resume       bool
	KeepPartial  bool
	ForceRestart bool
	BreakLock    bool
}

// ReceiveOnce listens and serves one incoming transfer.
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

// HandleConnection serves one accepted connection transfer session.
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
		_ = sendErrorFrame(writer, "transfer rejected")
		return fmt.Errorf("transfer rejected by receiver: %w", apperrors.ErrRejected)
	}

	paths, err := resume.ResolvePaths(opts.OutDir, offer.Name, opts.Overwrite)
	if err != nil {
		_ = sendErrorFrame(writer, "unable to resolve output path")
		return fmt.Errorf("resolve output paths: %w: %w", err, apperrors.ErrIO)
	}
	lock, err := resume.AcquireLock(paths.Lock, offer.SessionID, peer, opts.BreakLock)
	if err != nil {
		_ = sendErrorFrame(writer, err.Error())
		return err
	}
	defer lock.Release()

	resumeOffset, err := prepareResumeState(paths, offer, opts)
	if err != nil {
		_ = sendErrorFrame(writer, err.Error())
		return err
	}
	if resumeOffset > 0 {
		_, _ = fmt.Fprintf(opts.Out, "Resuming at offset %d (%.2f%%)\n", resumeOffset, (float64(resumeOffset)/float64(offer.Size))*100)
	}

	if err := WriteFrame(writer, Frame{Type: TypeAccept, Payload: EncodeAccept(resumeOffset, offer.SessionID)}); err != nil {
		return fmt.Errorf("send accept frame: %w", err)
	}
	if err := writer.Flush(); err != nil {
		return fmt.Errorf("flush accept frame: %w: %w", err, apperrors.ErrNetwork)
	}

	file, err := os.OpenFile(filepath.Clean(paths.Partial), os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return fmt.Errorf("open partial output file: %w: %w", err, apperrors.ErrIO)
	}
	if _, err := file.Seek(int64(resumeOffset), io.SeekStart); err != nil {
		_ = file.Close()
		return fmt.Errorf("seek partial output file: %w: %w", err, apperrors.ErrIO)
	}

	cleanup := true
	preservePartial := false
	defer func() {
		_ = file.Close()
		if cleanup && !opts.KeepPartial && !preservePartial {
			_ = os.Remove(paths.Partial)
			_ = os.Remove(paths.Meta)
		}
	}()

	meta := resume.Meta{ExpectedSize: offer.Size, ReceivedOffset: resumeOffset, OriginalName: offer.Name, SessionID: offer.SessionID}
	if err := resume.SaveMetaAtomic(paths.Meta, meta); err != nil {
		return fmt.Errorf("write initial resume metadata: %w: %w", err, apperrors.ErrIO)
	}
	hasher, err := hash.New()
	if err != nil {
		return fmt.Errorf("create receiver hasher: %w", err)
	}

	reporter := progress.NewReporter(opts.Out, "receiving", offer.Size)
	written := resumeOffset
	lastMetaSync := resumeOffset
	for written < offer.Size {
		frame, readErr := ReadFrame(reader)
		if readErr != nil {
			preservePartial = true
			return fmt.Errorf("read data frame: %w: %w", readErr, apperrors.ErrNetwork)
		}
		if frame.Type != TypeData {
			_ = sendErrorFrame(writer, "expected DATA frame")
			return fmt.Errorf("expected DATA frame, got %d: %w", frame.Type, apperrors.ErrInvalidProtocol)
		}
		if written+uint64(len(frame.Payload)) > offer.Size {
			_ = sendErrorFrame(writer, "received more data than offered")
			return fmt.Errorf("received more bytes than expected: %w", apperrors.ErrInvalidProtocol)
		}
		n, werr := file.Write(frame.Payload)
		if werr != nil || n != len(frame.Payload) {
			return fmt.Errorf("write output file: %w: %w", werr, apperrors.ErrIO)
		}
		if resumeOffset == 0 {
			if _, err := hasher.Write(frame.Payload); err != nil {
				return fmt.Errorf("hash received chunk: %w", err)
			}
		}
		written += uint64(n)
		reporter.Update(written)
		if written-lastMetaSync >= resumeMetaUpdateBytes {
			meta.ReceivedOffset = written
			if err := resume.SaveMetaAtomic(paths.Meta, meta); err != nil {
				return fmt.Errorf("periodic resume metadata update: %w: %w", err, apperrors.ErrIO)
			}
			lastMetaSync = written
		}
	}
	meta.ReceivedOffset = written
	if err := resume.SaveMetaAtomic(paths.Meta, meta); err != nil {
		return fmt.Errorf("final resume metadata update: %w: %w", err, apperrors.ErrIO)
	}

	done, err := ReadFrame(reader)
	if err != nil {
		preservePartial = true
		return fmt.Errorf("read done frame: %w: %w", err, apperrors.ErrNetwork)
	}
	if done.Type != TypeDone {
		return fmt.Errorf("expected DONE frame, got %d: %w", done.Type, apperrors.ErrInvalidProtocol)
	}
	expectedDigest, err := DecodeDone(done.Payload)
	if err != nil {
		return fmt.Errorf("decode done payload: %w", err)
	}
	var actualDigest []byte
	if resumeOffset > 0 {
		actualDigest, err = hashFile(paths.Partial)
		if err != nil {
			return fmt.Errorf("rehash resumed file: %w", err)
		}
	} else {
		actualDigest = hasher.Sum()
	}
	if subtle.ConstantTimeCompare(expectedDigest, actualDigest) != 1 {
		_ = sendErrorFrame(writer, "integrity check failed")
		return fmt.Errorf("integrity check failed: %w", apperrors.ErrIntegrity)
	}
	if err := file.Sync(); err != nil {
		return fmt.Errorf("sync output file: %w: %w", err, apperrors.ErrIO)
	}
	if err := resume.Finalize(paths); err != nil {
		return fmt.Errorf("finalize partial file: %w: %w", err, apperrors.ErrIO)
	}
	cleanup = false
	reporter.Done(written, paths.Final)
	_, _ = fmt.Fprintln(opts.Out, "Transfer complete.")
	_, _ = fmt.Fprintln(opts.Out, "Integrity verified.")
	_, _ = fmt.Fprintf(opts.Out, "blake3: %x\n", actualDigest)
	return nil
}

func prepareResumeState(paths resume.Paths, offer OfferPayload, opts ReceiverOptions) (uint64, error) {
	if !opts.Resume {
		_ = os.Remove(paths.Partial)
		_ = os.Remove(paths.Meta)
		return 0, nil
	}
	partialInfo, partialErr := os.Stat(paths.Partial)
	meta, metaErr := resume.LoadMeta(paths.Meta)
	if errors.Is(partialErr, os.ErrNotExist) && errors.Is(metaErr, os.ErrNotExist) {
		return 0, nil
	}
	if errors.Is(partialErr, os.ErrNotExist) && metaErr == nil {
		_ = os.Remove(paths.Meta)
		return 0, nil
	}
	if partialErr == nil && errors.Is(metaErr, os.ErrNotExist) {
		_ = os.Truncate(paths.Partial, 0)
		return 0, nil
	}
	if partialErr != nil {
		return 0, fmt.Errorf("stat partial file: %w", partialErr)
	}
	if metaErr != nil {
		_ = os.Truncate(paths.Partial, 0)
		_ = os.Remove(paths.Meta)
		return 0, nil
	}
	if meta.SessionID != offer.SessionID {
		if !opts.ForceRestart {
			return 0, fmt.Errorf("resume session mismatch: %w", apperrors.ErrRejected)
		}
		_ = os.Remove(paths.Partial)
		_ = os.Remove(paths.Meta)
		return 0, nil
	}
	if meta.ExpectedSize != offer.Size {
		if !opts.ForceRestart {
			return 0, fmt.Errorf("resume size mismatch: %w", apperrors.ErrRejected)
		}
		_ = os.Remove(paths.Partial)
		_ = os.Remove(paths.Meta)
		return 0, nil
	}
	size := uint64(partialInfo.Size())
	if size > offer.Size {
		_ = os.Truncate(paths.Partial, int64(offer.Size))
		size = offer.Size
	}
	offset := meta.ReceivedOffset
	if offset > size {
		offset = size
	}
	return offset, nil
}

func hashFile(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open file for integrity rehash: %w", err)
	}
	defer func() { _ = f.Close() }()
	h, _ := hash.New()
	buf := make([]byte, MaxChunkSize)
	for {
		n, rerr := f.Read(buf)
		if n > 0 {
			_, _ = h.Write(buf[:n])
		}
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			return nil, fmt.Errorf("read file for rehash: %w", rerr)
		}
	}
	return h.Sum(), nil
}

func sendErrorFrame(w *bufio.Writer, message string) error {
	payload, err := EncodeError(message)
	if err != nil {
		return err
	}
	if err := WriteFrame(w, Frame{Type: TypeError, Payload: payload}); err != nil {
		return err
	}
	return w.Flush()
}

func sendProtocolError(w *bufio.Writer, message string) error {
	_ = sendErrorFrame(w, message)
	return fmt.Errorf("%s: %w", message, apperrors.ErrInvalidProtocol)
}
