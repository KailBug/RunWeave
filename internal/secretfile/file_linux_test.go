package secretfile

import (
	"os"
	"path/filepath"
	"testing"
)

func TestUnsafeModesAndSymlink(t *testing.T) {
	path := filepath.Join(t.TempDir(), "token")
	if err := Create(path, []byte("secret")); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := Read(path, 256); err == nil {
		t.Fatal("world-readable secret accepted")
	}
	if err := os.Chmod(path, 0600); err != nil {
		t.Fatal(err)
	}
	link := path + "-link"
	if err := os.Symlink(path, link); err != nil {
		t.Fatal(err)
	}
	if _, err := Read(link, 256); err == nil {
		t.Fatal("secret symlink followed")
	}
}
