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
)

// SenderOptions configures sender behavior.
type SenderOptions struct {
	Path         string
	Address      string
	OverrideName string
	Out          io.Writer
}

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
	switch resp.Type {
	case TypeAccept:
	case TypeError:
		msg, decErr := DecodeError(resp.Payload)
		if decErr != nil {
			return fmt.Errorf("decode receiver error frame: %w", decErr)
		}
		return fmt.Errorf("receiver rejected transfer: %s: %w", msg, apperrors.ErrRejected)
	default:
		return fmt.Errorf("unexpected response frame type %d: %w", resp.Type, apperrors.ErrInvalidProtocol)
	}

	reporter := progress.NewReporter(opts.Out, "sending", uint64(info.Size()))
	buf := make([]byte, MaxChunkSize)
	var sent uint64
	for {
		n, readErr := file.Read(buf)
		if n > 0 {
			if err := WriteFrame(writer, Frame{Type: TypeData, Payload: buf[:n]}); err != nil {
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

	if err := WriteFrame(writer, Frame{Type: TypeDone}); err != nil {
		return fmt.Errorf("send done frame: %w: %w", err, apperrors.ErrNetwork)
	}
	if err := writer.Flush(); err != nil {
		return fmt.Errorf("flush transfer frames: %w: %w", err, apperrors.ErrNetwork)
	}
	reporter.Done(sent, sendName)
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
