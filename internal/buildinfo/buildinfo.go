// Package buildinfo provides build metadata for SnapSync binaries.
package buildinfo

import (
	"fmt"
	"runtime"
)

var (
	// Version is the SnapSync version and is intended to be injected at build time.
	Version string
	// Commit is the source control revision and is intended to be injected at build time.
	Commit string
	// Date is the build timestamp and is intended to be injected at build time.
	Date string
)

// Info contains normalized build metadata.
type Info struct {
	Version string
	Commit  string
	Date    string
	Go      string
	OS      string
	Arch    string
}

// Get returns build metadata with defaults when build flags are not provided.
func Get() Info {
	version := Version
	if version == "" {
		version = "dev"
	}

	commit := Commit
	if commit == "" {
		commit = "unknown"
	}

	date := Date
	if date == "" {
		date = "unknown"
	}

	return Info{
		Version: version,
		Commit:  commit,
		Date:    date,
		Go:      runtime.Version(),
		OS:      runtime.GOOS,
		Arch:    runtime.GOARCH,
	}
}

// String formats build metadata for CLI output.
func (i Info) String() string {
	return fmt.Sprintf("SnapSync %s\ncommit: %s\nbuilt:  %s\ngo:     %s\nos/arch:%s/%s", i.Version, i.Commit, i.Date, i.Go, i.OS, i.Arch)
}
