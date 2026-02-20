// Package app wires SnapSync application execution.
package app

import (
	"fmt"
	"os"

	"snapsync/internal/cli"
	apperrors "snapsync/internal/errors"
)

// App wires CLI execution.
type App struct{}

// New creates an App.
func New() App {
	return App{}
}

// Run executes the application and returns a process exit code.
func (a App) Run(args []string) int {
	root := cli.NewRootCommand(os.Stdout, os.Stderr)
	root.SetArgs(args)

	if err := root.Execute(); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return apperrors.ExitCode(err)
	}

	return 0
}
