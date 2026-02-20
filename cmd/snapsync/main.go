// Package main is the SnapSync CLI entrypoint.
package main

import (
	"os"

	"snapsync/internal/app"
)

func main() {
	application := app.New()
	os.Exit(application.Run(os.Args[1:]))
}
