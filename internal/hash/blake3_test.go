package hash

import "testing"

func TestKnownVector(t *testing.T) {
	h, err := New()
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	_, _ = h.Write([]byte("abc"))
	got := h.SumHex()
	const want = "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad"
	if got != want {
		t.Fatalf("hash mismatch got %s want %s", got, want)
	}
}

func TestStreamingEqualsSingleWrite(t *testing.T) {
	a, _ := New()
	_, _ = a.Write([]byte("hello world"))
	b, _ := New()
	_, _ = b.Write([]byte("hello "))
	_, _ = b.Write([]byte("world"))
	if a.SumHex() != b.SumHex() {
		t.Fatalf("streaming mismatch got %s vs %s", a.SumHex(), b.SumHex())
	}
}
