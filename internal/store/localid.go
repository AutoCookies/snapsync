// Package store provides tiny local persistence helpers.
package store

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// LoadOrCreatePeerID loads a persisted peer ID or writes a new one.
func LoadOrCreatePeerID(generate func() (string, error)) (string, error) {
	path, err := peerIDPath()
	if err != nil {
		return "", fmt.Errorf("resolve peer id path: %w", err)
	}
	if data, readErr := os.ReadFile(path); readErr == nil {
		id := strings.TrimSpace(string(data))
		if id != "" {
			return id, nil
		}
	} else if !os.IsNotExist(readErr) {
		return "", fmt.Errorf("read peer id file: %w", readErr)
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", fmt.Errorf("create peer id directory: %w", err)
	}
	id, err := generate()
	if err != nil {
		return "", fmt.Errorf("generate peer id: %w", err)
	}
	if err := os.WriteFile(path, []byte(id+"\n"), 0o600); err != nil {
		return "", fmt.Errorf("write peer id file: %w", err)
	}
	return id, nil
}

func peerIDPath() (string, error) {
	if runtime.GOOS == "windows" {
		appData := os.Getenv("APPDATA")
		if appData == "" {
			return "", fmt.Errorf("APPDATA is not set")
		}
		return filepath.Join(appData, "SnapSync", "peer_id"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve user home dir: %w", err)
	}
	return filepath.Join(home, ".config", "snapsync", "peer_id"), nil
}
