package store

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

var ErrLocked = errors.New("state directory is locked by another process")

// The file must never be unlinked while it could have an owner: an unlinked lock
// permits a second process to lock a new inode while the first still owns the old.
func acquireLock(dir string) (*os.File, error) {
	path := filepath.Join(dir, "daemon.lock")
	if err := regularOrAbsent(path); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, fmt.Errorf("open state lock: %w", err)
	}
	if err := lockFile(f); err != nil {
		_ = f.Close()
		return nil, err
	}
	return f, nil
}

func regularOrAbsent(path string) error {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return errors.New("state files must be regular files, not directories or symbolic links")
	}
	return nil
}
