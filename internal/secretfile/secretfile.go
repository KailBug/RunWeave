// Package secretfile reads and creates bounded, owner-restricted secret files.
package secretfile

import (
	"errors"
	"io"
	"os"
	"path/filepath"
)

var ErrUnsafe = errors.New("secret file must be a private regular file owned by the current OS user")

func Read(path string, limit int64) ([]byte, error) {
	if limit < 1 {
		return nil, errors.New("invalid secret size limit")
	}
	f, err := openPrivate(path, false)
	if err != nil {
		return nil, errors.New("cannot open private secret file")
	}
	defer f.Close()
	if err := validatePrivate(f); err != nil {
		return nil, ErrUnsafe
	}
	data, err := io.ReadAll(io.LimitReader(f, limit+1))
	if err != nil || len(data) == 0 || int64(len(data)) > limit {
		return nil, errors.New("secret file is empty, unreadable, or too large")
	}
	return data, nil
}

// Create never overwrites a file. A failure after creation leaves the file for
// explicit inspection: it must not delete a possibly durable sole credential.
func Create(path string, data []byte) error {
	if len(data) == 0 || len(data) > 64<<10 {
		return errors.New("invalid secret size")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return errors.New("cannot create secret directory")
	}
	f, err := openPrivate(path, true)
	if err != nil {
		return errors.New("cannot create private secret file; existing files are never overwritten")
	}
	defer f.Close()
	if err := validatePrivate(f); err != nil {
		return ErrUnsafe
	}
	if _, err := f.Write(data); err != nil {
		return errors.New("cannot write secret file; inspect the reserved file")
	}
	if err := f.Sync(); err != nil {
		return errors.New("cannot sync secret file; inspect the reserved file")
	}
	if err := f.Close(); err != nil {
		return errors.New("cannot close secret file")
	}
	return nil
}
