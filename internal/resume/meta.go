// Package resume handles persistent resume metadata and file naming.
package resume

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// MetaVersion is resume metadata schema version.
const MetaVersion uint16 = 1

// Meta stores crash-safe transfer progress for one partial file.
type Meta struct {
	Version        uint16 `json:"version"`
	ExpectedSize   uint64 `json:"expected_size"`
	ReceivedOffset uint64 `json:"received_offset"`
	OriginalName   string `json:"original_name"`
	SessionID      string `json:"session_id"`
}

// LoadMeta loads a metadata file.
func LoadMeta(path string) (Meta, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Meta{}, fmt.Errorf("read meta file: %w", err)
	}
	var meta Meta
	if err := json.Unmarshal(data, &meta); err != nil {
		return Meta{}, fmt.Errorf("decode meta file: %w", err)
	}
	if meta.Version != MetaVersion {
		return Meta{}, fmt.Errorf("unsupported meta version %d", meta.Version)
	}
	return meta, nil
}

// SaveMetaAtomic writes metadata atomically to target path.
func SaveMetaAtomic(path string, meta Meta) error {
	meta.Version = MetaVersion
	data, err := json.Marshal(meta)
	if err != nil {
		return fmt.Errorf("encode meta file: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create meta directory: %w", err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".snapsync-meta-*.tmp")
	if err != nil {
		return fmt.Errorf("create meta temp file: %w", err)
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write meta temp file: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("sync meta temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close meta temp file: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("rename meta temp file: %w", err)
	}
	return nil
}
