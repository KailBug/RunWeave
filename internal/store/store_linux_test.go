package store

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestUnwritableDirectoryAndPrivateModes(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "private")
	s := openTestStore(t, dir, Node)
	s.Close()
	for _, name := range []string{"", "daemon.lock", "state.db"} {
		info, err := os.Stat(filepath.Join(dir, name))
		if err != nil || info.Mode().Perm()&0077 != 0 {
			t.Fatalf("state permissions not private: %s %v", name, err)
		}
	}
	if os.Geteuid() == 0 {
		t.Skip("root bypasses directory mode bits")
	}
	if err := os.Chmod(dir, 0500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(dir, 0700) })
	if opened, err := Open(context.Background(), dir, Node); err == nil {
		opened.Close()
		t.Fatal("unwritable directory accepted")
	}
}

func TestStateSymlinksRejected(t *testing.T) {
	for _, name := range []string{"daemon.lock", "state.db", "state.db-wal", "state.db-shm", "state.db-journal"} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			target := filepath.Join(t.TempDir(), "target")
			if err := os.WriteFile(target, []byte("unchanged"), 0600); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(target, filepath.Join(dir, name)); err != nil {
				t.Fatal(err)
			}
			if s, err := Open(context.Background(), dir, Node); err == nil {
				s.Close()
				t.Fatal("state symlink accepted")
			}
			data, err := os.ReadFile(target)
			if err != nil || string(data) != "unchanged" {
				t.Fatal("symlink target modified", err)
			}
		})
	}
}
