package resume

import (
	"fmt"
	"os"
	"strconv"
	"time"

	apperrors "snapsync/internal/errors"
)

// FileLock represents an acquired target lock.
type FileLock struct {
	path string
	file *os.File
}

// AcquireLock acquires a target lock file exclusively.
func AcquireLock(path, sessionID, peer string, breakLock bool) (*FileLock, error) {
	if breakLock {
		_ = os.Remove(path)
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		if os.IsExist(err) {
			return nil, fmt.Errorf("output target is locked: %w", apperrors.ErrLockBusy)
		}
		return nil, fmt.Errorf("create lock file: %w", err)
	}
	body := "pid=" + strconv.Itoa(os.Getpid()) + "\n" +
		"time=" + time.Now().UTC().Format(time.RFC3339Nano) + "\n" +
		"session=" + sessionID + "\n" +
		"peer=" + peer + "\n"
	_, _ = f.WriteString(body)
	_ = f.Sync()
	return &FileLock{path: path, file: f}, nil
}

// Release frees an acquired lock.
func (l *FileLock) Release() {
	if l == nil {
		return
	}
	if l.file != nil {
		_ = l.file.Close()
	}
	if l.path != "" {
		_ = os.Remove(l.path)
	}
}
