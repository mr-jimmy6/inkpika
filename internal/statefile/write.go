// Package statefile persists local state without exposing partially written files.
package statefile

import (
	"os"
	"path/filepath"
)

// Write replaces filename only after the new contents have been flushed and
// closed. Callers serialize logical updates; unique temporary files prevent
// stale .tmp files or concurrent writers from sharing a file descriptor.
// The temporary file is on the same filesystem as the destination for rename.
func Write(filename string, data []byte) error {
	f, err := os.CreateTemp(filepath.Dir(filename), "."+filepath.Base(filename)+"-*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err = f.Write(data); err == nil {
		err = f.Sync()
	}
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	return os.Rename(f.Name(), filename)
}
