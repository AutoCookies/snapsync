// Package errors defines application errors and exit code mapping.
package errors

import sterrors "errors"

var (
	// ErrUsage indicates a command usage failure.
	ErrUsage = sterrors.New("usage error")
)

// ExitCode maps an error to a process exit code.
func ExitCode(err error) int {
	if err == nil {
		return 0
	}

	if sterrors.Is(err, ErrUsage) {
		return 2
	}

	return 1
}
