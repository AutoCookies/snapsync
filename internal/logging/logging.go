// Package logging provides minimal logger construction helpers.
package logging

import (
	"io"
	"log/slog"
)

// New creates a deterministic text logger at the provided level.
func New(w io.Writer, level slog.Leveler) *slog.Logger {
	handler := slog.NewTextHandler(w, &slog.HandlerOptions{
		AddSource: false,
		Level:     level,
	})

	return slog.New(handler)
}
