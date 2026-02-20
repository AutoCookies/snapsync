package transfer

import (
	"bytes"
	"encoding/binary"
	"strings"
	"testing"

	apperrors "snapsync/internal/errors"
)

func TestFrameRoundTrip(t *testing.T) {
	var buf bytes.Buffer
	in := Frame{Type: TypeData, Payload: []byte("hello")}
	if err := WriteFrame(&buf, in); err != nil {
		t.Fatalf("WriteFrame() error = %v", err)
	}
	out, err := ReadFrame(&buf)
	if err != nil {
		t.Fatalf("ReadFrame() error = %v", err)
	}
	if out.Type != in.Type || string(out.Payload) != string(in.Payload) {
		t.Fatalf("frame mismatch got %#v want %#v", out, in)
	}
}

func TestReadFrameRejectsInvalidMagicAndVersion(t *testing.T) {
	header := make([]byte, HeaderSize)
	copy(header[:4], []byte("NOPE"))
	binary.BigEndian.PutUint16(header[4:6], ProtocolVersion)
	binary.BigEndian.PutUint16(header[6:8], TypeHello)
	if _, err := ReadFrame(bytes.NewReader(header)); err == nil || !strings.Contains(err.Error(), apperrors.ErrInvalidProtocol.Error()) {
		t.Fatalf("expected invalid protocol error for bad magic, got %v", err)
	}

	copy(header[:4], []byte(Magic))
	binary.BigEndian.PutUint16(header[4:6], ProtocolVersion+1)
	if _, err := ReadFrame(bytes.NewReader(header)); err == nil || !strings.Contains(err.Error(), apperrors.ErrInvalidProtocol.Error()) {
		t.Fatalf("expected invalid protocol error for bad version, got %v", err)
	}
}

func TestLengthLimitsRespected(t *testing.T) {
	payload := make([]byte, MaxChunkSize+1)
	if err := WriteFrame(&bytes.Buffer{}, Frame{Type: TypeData, Payload: payload}); err == nil {
		t.Fatal("expected WriteFrame to reject payload larger than max chunk")
	}

	header := make([]byte, HeaderSize)
	copy(header[:4], []byte(Magic))
	binary.BigEndian.PutUint16(header[4:6], ProtocolVersion)
	binary.BigEndian.PutUint16(header[6:8], TypeData)
	binary.BigEndian.PutUint32(header[8:12], uint32(MaxChunkSize+1))
	if _, err := ReadFrame(bytes.NewReader(header)); err == nil {
		t.Fatal("expected ReadFrame to reject oversized payload")
	}
}
