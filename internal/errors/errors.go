// Package errors defines application errors and exit code mapping.
package errors

import sterrors "errors"

var (
	// ErrUsage indicates a command usage failure.
	ErrUsage = sterrors.New("usage error")
	// ErrInvalidProtocol indicates wire protocol validation failures.
	ErrInvalidProtocol = sterrors.New("invalid protocol")
	// ErrRejected indicates a transfer was rejected by receiver policy/user.
	ErrRejected = sterrors.New("transfer rejected")
	// ErrIO indicates local filesystem or stream I/O failures.
	ErrIO = sterrors.New("io error")
	// ErrNetwork indicates network connectivity failures.
	ErrNetwork = sterrors.New("network error")
)

// ExitCode maps an error to a process exit code.
func ExitCode(err error) int {
	if err == nil {
		return 0
	}

	switch {
	case sterrors.Is(err, ErrUsage):
		return 2
	case sterrors.Is(err, ErrRejected):
		return 3
	case sterrors.Is(err, ErrInvalidProtocol):
		return 4
	case sterrors.Is(err, ErrNetwork):
		return 5
	case sterrors.Is(err, ErrIO):
		return 6
	default:
		return 1
	}
}
