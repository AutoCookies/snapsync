// Package sanitize provides safe cross-platform filename handling helpers.
package sanitize

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

var reservedNames = map[string]struct{}{
	"CON": {}, "PRN": {}, "AUX": {}, "NUL": {},
	"COM1": {}, "COM2": {}, "COM3": {}, "COM4": {}, "COM5": {}, "COM6": {}, "COM7": {}, "COM8": {}, "COM9": {},
	"LPT1": {}, "LPT2": {}, "LPT3": {}, "LPT4": {}, "LPT5": {}, "LPT6": {}, "LPT7": {}, "LPT8": {}, "LPT9": {},
}

// SafeFileName sanitizes filename for cross-platform compatibility.
func SafeFileName(name string) string {
	base := filepath.Base(strings.TrimSpace(name))
	if base == "." || base == string(filepath.Separator) || base == "" {
		base = "file"
	}
	replacer := strings.NewReplacer("<", "_", ">", "_", ":", "_", "\"", "_", "/", "_", "\\", "_", "|", "_", "?", "_", "*", "_")
	base = replacer.Replace(base)
	base = strings.Trim(base, " .")
	if base == "" {
		base = "file"
	}
	if _, ok := reservedNames[strings.ToUpper(base)]; ok {
		base = base + "_"
	}
	return base
}

// ResolveCollisionPath returns available output path, applying (n) suffix when needed.
func ResolveCollisionPath(dir, name string, overwrite bool) (string, error) {
	safe := SafeFileName(name)
	path := filepath.Join(dir, safe)
	if overwrite {
		return path, nil
	}
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return path, nil
		}
		return "", fmt.Errorf("stat output path: %w", err)
	}

	ext := filepath.Ext(safe)
	stem := strings.TrimSuffix(safe, ext)
	for i := 1; i < 10000; i++ {
		candidate := filepath.Join(dir, fmt.Sprintf("%s (%d)%s", stem, i, ext))
		if _, err := os.Stat(candidate); err != nil {
			if os.IsNotExist(err) {
				return candidate, nil
			}
			return "", fmt.Errorf("stat collision candidate: %w", err)
		}
	}
	return "", fmt.Errorf("could not resolve unique filename for %q", safe)
}
