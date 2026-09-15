//go:build !windows && !linux

package secretfile

import "os"

func openPrivate(string, bool) (*os.File, error) { return nil, ErrUnsafe }
func validatePrivate(*os.File) error             { return ErrUnsafe }
