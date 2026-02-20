// Package hash provides streaming integrity hashing helpers.
package hash

import (
	"crypto/sha256"
	"encoding/hex"
	"hash"
)

// Hasher wraps incremental hashing for transfer integrity.
type Hasher struct {
	h hash.Hash
}

// New creates a new 32-byte digest hasher.
func New() (*Hasher, error) {
	return &Hasher{h: sha256.New()}, nil
}

// Write adds data to the hash state.
func (h *Hasher) Write(p []byte) (int, error) { return h.h.Write(p) }

// Sum returns raw 32-byte digest.
func (h *Hasher) Sum() []byte { return h.h.Sum(nil) }

// SumHex returns lowercase hex digest.
func (h *Hasher) SumHex() string { return hex.EncodeToString(h.Sum()) }
