package cli

import (
	"fmt"
	"io"

	"snapsync/internal/buildinfo"
	apperrors "snapsync/internal/errors"
)

// NewVersionCommand creates the version subcommand.
func NewVersionCommand(out io.Writer) Command {
	return Command{
		name: "version",
		run: func(args []string) error {
			if len(args) > 0 {
				return fmt.Errorf("version accepts no arguments: %w", apperrors.ErrUsage)
			}

			if _, err := fmt.Fprintln(out, buildinfo.Get().String()); err != nil {
				return fmt.Errorf("write version output: %w", err)
			}

			return nil
		},
	}
}
