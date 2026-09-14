//go:build !windows && !linux

package store

import (
	"errors"
	"os"
)

func lockFile(*os.File) error {
	return errors.New("state locking is supported only on Windows and Linux")
}
