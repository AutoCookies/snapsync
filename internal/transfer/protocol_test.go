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

func TestOfferEncodeDecodeIncludesSession(t *testing.T) {
	p, err := EncodeOffer("x.bin", 42, "0123456789abcdef0123456789abcdef")
	if err != nil {
		t.Fatalf("EncodeOffer() error = %v", err)
	}
	o, err := DecodeOffer(p)
	if err != nil {
		t.Fatalf("DecodeOffer() error = %v", err)
	}
	if o.SessionID == "" || o.Size != 42 || o.Name != "x.bin" {
		t.Fatalf("unexpected offer: %#v", o)
	}
}

func TestDoneEncodesDecodesRawHash(t *testing.T) {
	raw := bytes.Repeat([]byte{0xAB}, HashSize)
	payload, err := EncodeDone(raw)
	if err != nil {
		t.Fatalf("EncodeDone() error = %v", err)
	}
	got, err := DecodeDone(payload)
	if err != nil {
		t.Fatalf("DecodeDone() error = %v", err)
	}
	if !bytes.Equal(raw, got) {
		t.Fatalf("hash mismatch got %x want %x", got, raw)
	}
}

func TestDoneRejectsMalformedPayload(t *testing.T) {
	if _, err := DecodeDone([]byte{}); err == nil {
		t.Fatal("expected malformed done payload failure")
	}
	bad := make([]byte, 2+HashSize)
	binary.BigEndian.PutUint16(bad[:2], 31)
	if _, err := DecodeDone(bad); err == nil {
		t.Fatal("expected invalid hash length failure")
	}
}

func TestAcceptEncodesDecodesResumeOffset(t *testing.T) {
	payload := EncodeAccept(12345, "0123456789abcdef0123456789abcdef")
	offset, sid, err := DecodeAccept(payload)
	if err != nil {
		t.Fatalf("DecodeAccept() error = %v", err)
	}
	if offset != 12345 || sid == "" {
		t.Fatalf("offset mismatch got %d", offset)
	}
	if _, _, err := DecodeAccept([]byte{1, 2}); err == nil {
		t.Fatal("expected invalid accept payload failure")
	}
}
