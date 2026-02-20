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
	// ProtocolVersion is the current protocol version.
	ProtocolVersion uint16 = 1
	// HeaderSize is fixed frame header size.
	HeaderSize = 16
	// MaxChunkSize limits DATA payload size.
	MaxChunkSize = 1024 * 1024
	// MaxControlPayload limits control frame payload sizes.
	MaxControlPayload = 4096
)

const (
	// TypeHello starts protocol negotiation.
	TypeHello uint16 = 1
	// TypeOffer announces file name and size.
	TypeOffer uint16 = 2
	// TypeAccept accepts offered transfer.
	TypeAccept uint16 = 3
	// TypeData carries file bytes.
	TypeData uint16 = 4
	// TypeDone finishes transfer.
	TypeDone uint16 = 5
	// TypeError carries rejection/failure reason.
	TypeError uint16 = 6
)

// Frame is a protocol frame.
type Frame struct {
	Type    uint16
	Payload []byte
}

// OfferPayload represents decoded OFFER payload data.
type OfferPayload struct {
	Name string
	Size uint64
}

// WriteFrame writes one frame to writer.
func WriteFrame(w io.Writer, frame Frame) error {
	if len(frame.Payload) > maxPayloadByType(frame.Type) {
		return fmt.Errorf("payload too large for type %d: %w", frame.Type, apperrors.ErrInvalidProtocol)
	}

	header := make([]byte, HeaderSize)
	copy(header[0:4], []byte(Magic))
	binary.BigEndian.PutUint16(header[4:6], ProtocolVersion)
	binary.BigEndian.PutUint16(header[6:8], frame.Type)
	binary.BigEndian.PutUint32(header[8:12], uint32(len(frame.Payload)))
	binary.BigEndian.PutUint32(header[12:16], 0)

	if _, err := w.Write(header); err != nil {
		return fmt.Errorf("write frame header: %w", err)
	}
	if len(frame.Payload) == 0 {
		return nil
	}
	if _, err := w.Write(frame.Payload); err != nil {
		return fmt.Errorf("write frame payload: %w", err)
	}
	return nil
}

// ReadFrame reads one frame from reader.
func ReadFrame(r io.Reader) (Frame, error) {
	header := make([]byte, HeaderSize)
	if _, err := io.ReadFull(r, header); err != nil {
		return Frame{}, fmt.Errorf("read frame header: %w", err)
	}
	if string(header[0:4]) != Magic {
		return Frame{}, fmt.Errorf("invalid magic: %w", apperrors.ErrInvalidProtocol)
	}
	version := binary.BigEndian.Uint16(header[4:6])
	if version != ProtocolVersion {
		return Frame{}, fmt.Errorf("unsupported version %d: %w", version, apperrors.ErrInvalidProtocol)
	}
	reserved := binary.BigEndian.Uint32(header[12:16])
	if reserved != 0 {
		return Frame{}, fmt.Errorf("reserved field must be zero: %w", apperrors.ErrInvalidProtocol)
	}
	frameType := binary.BigEndian.Uint16(header[6:8])
	length := binary.BigEndian.Uint32(header[8:12])
	if int(length) > maxPayloadByType(frameType) {
		return Frame{}, fmt.Errorf("payload length too large for type %d: %w", frameType, apperrors.ErrInvalidProtocol)
	}

	payload := make([]byte, int(length))
	if length > 0 {
		if _, err := io.ReadFull(r, payload); err != nil {
			return Frame{}, fmt.Errorf("read frame payload: %w", err)
		}
	}
	return Frame{Type: frameType, Payload: payload}, nil
}

// EncodeOffer builds OFFER payload.
func EncodeOffer(name string, size uint64) ([]byte, error) {
	if len(name) == 0 || len(name) > 1024 {
		return nil, fmt.Errorf("invalid offer name length: %w", apperrors.ErrInvalidProtocol)
	}
	payload := make([]byte, 2+len(name)+8)
	binary.BigEndian.PutUint16(payload[0:2], uint16(len(name)))
	copy(payload[2:2+len(name)], []byte(name))
	binary.BigEndian.PutUint64(payload[2+len(name):], size)
	return payload, nil
}

// DecodeOffer parses OFFER payload.
func DecodeOffer(payload []byte) (OfferPayload, error) {
	if len(payload) < 10 {
		return OfferPayload{}, fmt.Errorf("offer payload too short: %w", apperrors.ErrInvalidProtocol)
	}
	nameLen := int(binary.BigEndian.Uint16(payload[:2]))
	if nameLen <= 0 || 2+nameLen+8 != len(payload) {
		return OfferPayload{}, fmt.Errorf("offer payload malformed: %w", apperrors.ErrInvalidProtocol)
	}
	name := string(payload[2 : 2+nameLen])
	size := binary.BigEndian.Uint64(payload[2+nameLen:])
	return OfferPayload{Name: name, Size: size}, nil
}

// EncodeError builds ERROR payload.
func EncodeError(msg string) ([]byte, error) {
	if len(msg) == 0 || len(msg) > 1024 {
		return nil, fmt.Errorf("invalid error message length: %w", apperrors.ErrInvalidProtocol)
	}
	payload := make([]byte, 2+len(msg))
	binary.BigEndian.PutUint16(payload[:2], uint16(len(msg)))
	copy(payload[2:], []byte(msg))
	return payload, nil
}

// DecodeError parses ERROR payload.
func DecodeError(payload []byte) (string, error) {
	if len(payload) < 2 {
		return "", fmt.Errorf("error payload too short: %w", apperrors.ErrInvalidProtocol)
	}
	msgLen := int(binary.BigEndian.Uint16(payload[:2]))
	if msgLen <= 0 || msgLen+2 != len(payload) {
		return "", fmt.Errorf("error payload malformed: %w", apperrors.ErrInvalidProtocol)
	}
	return string(payload[2:]), nil
}

func maxPayloadByType(frameType uint16) int {
	switch frameType {
	case TypeHello, TypeAccept, TypeDone:
		return 0
	case TypeOffer, TypeError:
		return MaxControlPayload
	case TypeData:
		return MaxChunkSize
	default:
		return MaxControlPayload
	}
}
