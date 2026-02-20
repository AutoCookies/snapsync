package resume

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"snapsync/internal/sanitize"
)

// Paths describes resolved final/partial/meta files for one transfer.
type Paths struct {
	Final   string
	Partial string
	Meta    string
}

// ResolvePaths finds stable destination paths for a transfer.
func ResolvePaths(outDir, originalName string, overwrite bool) (Paths, error) {
	safe := sanitize.SafeFileName(originalName)
	ext := filepath.Ext(safe)
	stem := strings.TrimSuffix(safe, ext)

	for i := 0; i < 10000; i++ {
		name := safe
		if i > 0 {
			name = fmt.Sprintf("%s (%d)%s", stem, i, ext)
		}
		finalPath := filepath.Join(outDir, name)
		partialPath := finalPath + ".partial"
		metaPath := partialPath + ".snapsync"

		if overwrite {
			return Paths{Final: finalPath, Partial: partialPath, Meta: metaPath}, nil
		}
		if fileExists(partialPath) || fileExists(metaPath) {
			return Paths{Final: finalPath, Partial: partialPath, Meta: metaPath}, nil
		}
		if !fileExists(finalPath) {
			return Paths{Final: finalPath, Partial: partialPath, Meta: metaPath}, nil
		}
	}
	return Paths{}, fmt.Errorf("could not resolve output paths")
}

// Finalize renames partial file to final and removes metadata.
func Finalize(paths Paths) error {
	if err := os.Rename(paths.Partial, paths.Final); err != nil {
		return fmt.Errorf("rename partial to final: %w", err)
	}
	_ = os.Remove(paths.Meta)
	return nil
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
