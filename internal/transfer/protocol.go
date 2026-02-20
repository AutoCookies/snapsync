// Package transfer implements SnapSync TCP transfer protocol and data flows.
package transfer

import (
	"encoding/binary"
	"fmt"
	"io"

	apperrors "snapsync/internal/errors"
)

const (
	// Magic marks SnapSync wire frames.
	Magic = "SSYN"
	// ProtocolVersion is the current SnapSync wire protocol version.
	ProtocolVersion uint16 = 1
	// HeaderSize is the fixed protocol header length in bytes.
	HeaderSize = 16
	// MaxChunkSize is the max DATA payload bytes per frame.
	MaxChunkSize = 1024 * 1024
	// MaxControlPayload is the max control payload size.
	MaxControlPayload = 4096
	// HashSize is the raw final digest size in bytes.
	HashSize = 32
)

const (
	// TypeHello starts protocol negotiation.
	TypeHello uint16 = 1
	// TypeOffer announces file metadata.
	TypeOffer uint16 = 2
	// TypeAccept returns acceptance with resume/session info.
	TypeAccept uint16 = 3
	// TypeData carries file bytes.
	TypeData uint16 = 4
	// TypeDone completes transfer with final digest.
	TypeDone uint16 = 5
	// TypeError carries receiver/sender error messages.
	TypeError uint16 = 6
)

// Frame is a protocol frame.
type Frame struct {
	Type    uint16
	Payload []byte
}

// OfferPayload represents decoded OFFER payload data.
type OfferPayload struct {
	Name      string
	Size      uint64
	SessionID string
}

// WriteFrame writes one protocol frame to the stream.
func WriteFrame(w io.Writer, frame Frame) error {
	if len(frame.Payload) > maxPayloadByType(frame.Type) {
		return fmt.Errorf("payload too large for type %d: %w", frame.Type, apperrors.ErrInvalidProtocol)
	}
	header := make([]byte, HeaderSize)
	copy(header[0:4], []byte(Magic))
	binary.BigEndian.PutUint16(header[4:6], ProtocolVersion)
	binary.BigEndian.PutUint16(header[6:8], frame.Type)
	binary.BigEndian.PutUint32(header[8:12], uint32(len(frame.Payload)))
	if _, err := w.Write(header); err != nil {
		return fmt.Errorf("write frame header: %w", err)
	}
	if len(frame.Payload) > 0 {
		if _, err := w.Write(frame.Payload); err != nil {
			return fmt.Errorf("write frame payload: %w", err)
		}
	}
	return nil
}

// ReadFrame reads one protocol frame from the stream.
func ReadFrame(r io.Reader) (Frame, error) {
	header := make([]byte, HeaderSize)
	if _, err := io.ReadFull(r, header); err != nil {
		return Frame{}, fmt.Errorf("read frame header: %w", err)
	}
	if string(header[:4]) != Magic {
		return Frame{}, fmt.Errorf("invalid magic: %w", apperrors.ErrInvalidProtocol)
	}
	if binary.BigEndian.Uint16(header[4:6]) != ProtocolVersion {
		return Frame{}, fmt.Errorf("unsupported protocol version: %w", apperrors.ErrInvalidProtocol)
	}
	if binary.BigEndian.Uint32(header[12:16]) != 0 {
		return Frame{}, fmt.Errorf("reserved field must be zero: %w", apperrors.ErrInvalidProtocol)
	}
	t := binary.BigEndian.Uint16(header[6:8])
	ln := binary.BigEndian.Uint32(header[8:12])
	if int(ln) > maxPayloadByType(t) {
		return Frame{}, fmt.Errorf("payload length too large for type %d: %w", t, apperrors.ErrInvalidProtocol)
	}
	payload := make([]byte, int(ln))
	if ln > 0 {
		if _, err := io.ReadFull(r, payload); err != nil {
			return Frame{}, fmt.Errorf("read frame payload: %w", err)
		}
	}
	return Frame{Type: t, Payload: payload}, nil
}

// EncodeOffer builds OFFER payload.
func EncodeOffer(name string, size uint64, sessionID string) ([]byte, error) {
	if len(name) == 0 || len(name) > 1024 || len(sessionID) == 0 || len(sessionID) > 128 {
		return nil, fmt.Errorf("invalid offer fields: %w", apperrors.ErrInvalidProtocol)
	}
	payload := make([]byte, 2+len(name)+8+2+len(sessionID))
	off := 0
	binary.BigEndian.PutUint16(payload[off:off+2], uint16(len(name)))
	off += 2
	copy(payload[off:off+len(name)], []byte(name))
	off += len(name)
	binary.BigEndian.PutUint64(payload[off:off+8], size)
	off += 8
	binary.BigEndian.PutUint16(payload[off:off+2], uint16(len(sessionID)))
	off += 2
	copy(payload[off:], []byte(sessionID))
	return payload, nil
}

// DecodeOffer parses OFFER payload.
func DecodeOffer(payload []byte) (OfferPayload, error) {
	if len(payload) < 12 {
		return OfferPayload{}, fmt.Errorf("offer payload too short: %w", apperrors.ErrInvalidProtocol)
	}
	off := 0
	nameLen := int(binary.BigEndian.Uint16(payload[off : off+2]))
	off += 2
	if nameLen <= 0 || off+nameLen+8+2 > len(payload) {
		return OfferPayload{}, fmt.Errorf("offer payload malformed: %w", apperrors.ErrInvalidProtocol)
	}
	name := string(payload[off : off+nameLen])
	off += nameLen
	size := binary.BigEndian.Uint64(payload[off : off+8])
	off += 8
	sidLen := int(binary.BigEndian.Uint16(payload[off : off+2]))
	off += 2
	if sidLen <= 0 || off+sidLen != len(payload) {
		return OfferPayload{}, fmt.Errorf("offer session malformed: %w", apperrors.ErrInvalidProtocol)
	}
	return OfferPayload{Name: name, Size: size, SessionID: string(payload[off:])}, nil
}

// EncodeAccept builds ACCEPT payload containing resume offset and session id.
func EncodeAccept(offset uint64, sessionID string) []byte {
	payload := make([]byte, 8+2+len(sessionID))
	binary.BigEndian.PutUint64(payload[:8], offset)
	binary.BigEndian.PutUint16(payload[8:10], uint16(len(sessionID)))
	copy(payload[10:], []byte(sessionID))
	return payload
}

// DecodeAccept parses ACCEPT payload.
func DecodeAccept(payload []byte) (uint64, string, error) {
	if len(payload) < 10 {
		return 0, "", fmt.Errorf("invalid accept payload length: %w", apperrors.ErrInvalidProtocol)
	}
	offset := binary.BigEndian.Uint64(payload[:8])
	sidLen := int(binary.BigEndian.Uint16(payload[8:10]))
	if sidLen <= 0 || len(payload) != 10+sidLen {
		return 0, "", fmt.Errorf("invalid accept session: %w", apperrors.ErrInvalidProtocol)
	}
	return offset, string(payload[10:]), nil
}

// EncodeDone encodes the DONE payload carrying the final digest.
func EncodeDone(hash []byte) ([]byte, error) {
	if len(hash) != HashSize {
		return nil, fmt.Errorf("invalid done hash length: %w", apperrors.ErrInvalidProtocol)
	}
	payload := make([]byte, 2+HashSize)
	binary.BigEndian.PutUint16(payload[:2], uint16(HashSize))
	copy(payload[2:], hash)
	return payload, nil
}

// DecodeDone decodes the DONE payload carrying the final digest.
func DecodeDone(payload []byte) ([]byte, error) {
	if len(payload) != 2+HashSize {
		return nil, fmt.Errorf("invalid done payload length: %w", apperrors.ErrInvalidProtocol)
	}
	if int(binary.BigEndian.Uint16(payload[:2])) != HashSize {
		return nil, fmt.Errorf("invalid done hash len field: %w", apperrors.ErrInvalidProtocol)
	}
	h := make([]byte, HashSize)
	copy(h, payload[2:])
	return h, nil
}

// EncodeError encodes an ERROR payload message.
func EncodeError(msg string) ([]byte, error) {
	if len(msg) == 0 || len(msg) > 1024 {
		return nil, fmt.Errorf("invalid error message length: %w", apperrors.ErrInvalidProtocol)
	}
	payload := make([]byte, 2+len(msg))
	binary.BigEndian.PutUint16(payload[:2], uint16(len(msg)))
	copy(payload[2:], []byte(msg))
	return payload, nil
}

// DecodeError decodes an ERROR payload message.
func DecodeError(payload []byte) (string, error) {
	if len(payload) < 2 {
		return "", fmt.Errorf("error payload too short: %w", apperrors.ErrInvalidProtocol)
	}
	ln := int(binary.BigEndian.Uint16(payload[:2]))
	if ln <= 0 || ln+2 != len(payload) {
		return "", fmt.Errorf("error payload malformed: %w", apperrors.ErrInvalidProtocol)
	}
	return string(payload[2:]), nil
}

func maxPayloadByType(t uint16) int {
	switch t {
	case TypeHello:
		return 0
	case TypeAccept:
		return MaxControlPayload
	case TypeDone:
		return 2 + HashSize
	case TypeOffer, TypeError:
		return MaxControlPayload
	case TypeData:
		return MaxChunkSize
	default:
		return MaxControlPayload
	}
}
