package transfer

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"

	apperrors "snapsync/internal/errors"
	"snapsync/internal/hash"
	"snapsync/internal/progress"
)

// SenderOptions configures sender behavior.
type SenderOptions struct {
	Path         string
	Address      string
	OverrideName string
	Out          io.Writer
	Resume       bool
}

var senderChunkMutator func([]byte)

// Send streams one file to a receiver.
func Send(opts SenderOptions) error {
	if opts.Path == "" || opts.Address == "" {
		return fmt.Errorf("missing required sender options: %w", apperrors.ErrUsage)
	}
	if opts.Out == nil {
		opts.Out = io.Discard
	}

	file, info, sendName, err := openSource(opts.Path, opts.OverrideName)
	if err != nil {
		return err
	}
	defer func() { _ = file.Close() }()

	hasher, err := hash.New()
	if err != nil {
		return fmt.Errorf("create sender hasher: %w", err)
	}

	conn, err := net.Dial("tcp", opts.Address)
	if err != nil {
		return fmt.Errorf("dial receiver: %w: %w", err, apperrors.ErrNetwork)
	}
	defer func() { _ = conn.Close() }()

	reader := bufio.NewReader(conn)
	writer := bufio.NewWriter(conn)

	if err := WriteFrame(writer, Frame{Type: TypeHello}); err != nil {
		return fmt.Errorf("send hello: %w: %w", err, apperrors.ErrNetwork)
	}
	offerPayload, err := EncodeOffer(sendName, uint64(info.Size()))
	if err != nil {
		return fmt.Errorf("encode offer: %w", err)
	}
	if err := WriteFrame(writer, Frame{Type: TypeOffer, Payload: offerPayload}); err != nil {
		return fmt.Errorf("send offer: %w: %w", err, apperrors.ErrNetwork)
	}
	if err := writer.Flush(); err != nil {
		return fmt.Errorf("flush offer frames: %w: %w", err, apperrors.ErrNetwork)
	}

	resp, err := ReadFrame(reader)
	if err != nil {
		return fmt.Errorf("read receiver response: %w: %w", err, apperrors.ErrNetwork)
	}
	var resumeOffset uint64
	switch resp.Type {
	case TypeAccept:
		decoded, decErr := DecodeAccept(resp.Payload)
		if decErr != nil {
			return fmt.Errorf("decode accept frame: %w", decErr)
		}
		resumeOffset = decoded
	case TypeError:
		msg, decErr := DecodeError(resp.Payload)
		if decErr != nil {
			return fmt.Errorf("decode receiver error frame: %w", decErr)
		}
		return fmt.Errorf("receiver rejected transfer: %s: %w", msg, apperrors.ErrRejected)
	default:
		return fmt.Errorf("unexpected response frame type %d: %w", resp.Type, apperrors.ErrInvalidProtocol)
	}
	if !opts.Resume {
		resumeOffset = 0
	}
	if resumeOffset > uint64(info.Size()) {
		return fmt.Errorf("receiver resume offset %d exceeds file size %d: %w", resumeOffset, info.Size(), apperrors.ErrInvalidProtocol)
	}
	if resumeOffset > 0 {
		if _, err := fmt.Fprintf(opts.Out, "Resuming at offset %d (%.2f%%)\n", resumeOffset, (float64(resumeOffset)/float64(info.Size()))*100); err != nil {
			return fmt.Errorf("write resume output: %w", err)
		}
		if err := hashPrefix(file, resumeOffset, hasher); err != nil {
			return err
		}
	}
	if _, err := file.Seek(int64(resumeOffset), io.SeekStart); err != nil {
		return fmt.Errorf("seek source file for resume: %w: %w", err, apperrors.ErrIO)
	}

	reporter := progress.NewReporter(opts.Out, "sending", uint64(info.Size()))
	buf := make([]byte, MaxChunkSize)
	sent := resumeOffset
	for {
		n, readErr := file.Read(buf)
		if n > 0 {
			chunk := buf[:n]
			if _, err := hasher.Write(chunk); err != nil {
				return fmt.Errorf("hash source chunk: %w", err)
			}
			if senderChunkMutator != nil {
				mut := append([]byte{}, chunk...)
				senderChunkMutator(mut)
				chunk = mut
			}
			if err := WriteFrame(writer, Frame{Type: TypeData, Payload: chunk}); err != nil {
				return fmt.Errorf("send data frame: %w: %w", err, apperrors.ErrNetwork)
			}
			sent += uint64(n)
			reporter.Update(sent)
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return fmt.Errorf("read source file: %w: %w", readErr, apperrors.ErrIO)
		}
	}

	digest := hasher.Sum()
	donePayload, err := EncodeDone(digest)
	if err != nil {
		return fmt.Errorf("encode done payload: %w", err)
	}
	if err := WriteFrame(writer, Frame{Type: TypeDone, Payload: donePayload}); err != nil {
		return fmt.Errorf("send done frame: %w: %w", err, apperrors.ErrNetwork)
	}
	if err := writer.Flush(); err != nil {
		return fmt.Errorf("flush transfer frames: %w: %w", err, apperrors.ErrNetwork)
	}

	status, readErr := ReadFrame(reader)
	if readErr == nil && status.Type == TypeError {
		msg, _ := DecodeError(status.Payload)
		return fmt.Errorf("integrity check failed on receiver: %s: %w", msg, apperrors.ErrRejected)
	}
	if readErr != nil && !errors.Is(readErr, io.EOF) {
		return fmt.Errorf("read receiver completion status: %w: %w", readErr, apperrors.ErrNetwork)
	}

	reporter.Done(sent, sendName)
	_, _ = fmt.Fprintln(opts.Out, "Transfer complete.")
	_, _ = fmt.Fprintln(opts.Out, "Integrity verified.")
	_, _ = fmt.Fprintf(opts.Out, "blake3: %s\n", hasher.SumHex())
	return nil
}

func openSource(path, overrideName string) (*os.File, os.FileInfo, string, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, nil, "", fmt.Errorf("open source file: %w: %w", err, apperrors.ErrIO)
	}
	info, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return nil, nil, "", fmt.Errorf("stat source file: %w: %w", err, apperrors.ErrIO)
	}
	if !info.Mode().IsRegular() {
		_ = file.Close()
		return nil, nil, "", fmt.Errorf("source is not a regular file: %w", apperrors.ErrUsage)
	}
	name := filepath.Base(path)
	if overrideName != "" {
		name = overrideName
	}
	return file, info, name, nil
}

func hashPrefix(file *os.File, offset uint64, hasher *hash.Hasher) error {
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return fmt.Errorf("seek file for prefix hash: %w: %w", err, apperrors.ErrIO)
	}
	buf := make([]byte, MaxChunkSize)
	remaining := offset
	for remaining > 0 {
		toRead := len(buf)
		if uint64(toRead) > remaining {
			toRead = int(remaining)
		}
		n, err := io.ReadFull(file, buf[:toRead])
		if err != nil {
			return fmt.Errorf("read prefix for resume hash: %w: %w", err, apperrors.ErrIO)
		}
		if _, err := hasher.Write(buf[:n]); err != nil {
			return fmt.Errorf("hash prefix bytes: %w", err)
		}
		remaining -= uint64(n)
	}
	return nil
}
