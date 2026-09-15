package store

import (
	"bufio"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func openTestStore(t *testing.T, dir string, role Role) *Store {
	t.Helper()
	s, err := Open(context.Background(), dir, role)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := s.Close(); err != nil {
			t.Error(err)
		}
	})
	return s
}

func execSQL(t *testing.T, db *sql.DB, stmt string) {
	t.Helper()
	if _, err := db.Exec(stmt); err != nil {
		t.Fatal(err)
	}
}

func TestReopenBackupAndRoleIsolation(t *testing.T) {
	for _, role := range []Role{Server, Node} {
		t.Run(string(role), func(t *testing.T) {
			// Reserved URI characters must be part of the path, not driver options.
			dir := filepath.Join(t.TempDir(), "中文 # state %25")
			s := openTestStore(t, dir, role)
			original := s.Info()
			wantVersion := 1
			if role == Server {
				wantVersion = 2
			}
			if original.Role != role || len(original.InstanceID) != 32 || original.SchemaVersion != wantVersion {
				t.Fatalf("bad info: %+v", original)
			}
			if err := s.Close(); err != nil {
				t.Fatal(err)
			}
			s = openTestStore(t, dir, role)
			if s.Info() != original {
				t.Fatal("identity changed on reopen")
			}
			if err := s.Close(); err != nil {
				t.Fatal(err)
			}
			other := Server
			if role == Server {
				other = Node
			}
			if bad, err := Open(context.Background(), dir, other); err == nil {
				bad.Close()
				t.Fatal("state accepted another role")
			}
			// A failed Open must release the process lock too.
			s = openTestStore(t, dir, role)
			if s.Info() != original {
				t.Fatal("failed role switch modified identity")
			}
			s.Close()
			backup := t.TempDir()
			entries, err := os.ReadDir(dir)
			if err != nil {
				t.Fatal(err)
			}
			for _, entry := range entries {
				data, err := os.ReadFile(filepath.Join(dir, entry.Name()))
				if err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(backup, entry.Name()), data, 0600); err != nil {
					t.Fatal(err)
				}
			}
			restored := openTestStore(t, backup, role)
			if restored.Info() != original {
				t.Fatal("backup did not retain identity and schema")
			}
		})
	}
}

func TestConnectionPragmasAndConstraints(t *testing.T) {
	s := openTestStore(t, t.TempDir(), Server)
	// Force replacement connections: connection-local settings cannot be one-off.
	s.db.SetMaxIdleConns(0)
	for i := 0; i < 2; i++ {
		for query, want := range map[string]string{
			"PRAGMA journal_mode": "wal",
			"PRAGMA foreign_keys": "1",
			"PRAGMA busy_timeout": "1000",
			"PRAGMA synchronous":  "2",
		} {
			var got string
			if err := s.db.QueryRow(query).Scan(&got); err != nil || got != want {
				t.Fatalf("%s: %s, %v", query, got, err)
			}
		}
	}
	var sqliteVersion string
	if err := s.db.QueryRow("SELECT sqlite_version()").Scan(&sqliteVersion); err != nil {
		t.Fatal(err)
	}
	t.Logf("SQLite %s; replacement connections retain all PRAGMAs", sqliteVersion)
	execSQL(t, s.db, "CREATE TABLE test_parent(id TEXT PRIMARY KEY)")
	execSQL(t, s.db, "CREATE TABLE test_child(parent TEXT REFERENCES test_parent(id))")
	if _, err := s.db.Exec("INSERT INTO test_child VALUES('missing')"); err == nil {
		t.Fatal("foreign key check disabled")
	}
}

func TestMigrationRollbackAndUpgrade(t *testing.T) {
	dir := t.TempDir()
	s := openTestStore(t, dir, Node)
	original := s.Info()
	migrations, err := loadMigrations(Node)
	if err != nil {
		t.Fatal(err)
	}
	failed := migration{2, "002_test.sql", "CREATE TABLE migration_test(value TEXT); INSERT INTO table_does_not_exist VALUES(1);", "test-only"}
	if err := migrate(context.Background(), s.db, Node, append(migrations, failed)); err == nil {
		t.Fatal("invalid migration succeeded")
	}
	var count int
	if err := s.db.QueryRow("SELECT count(*) FROM sqlite_master WHERE name='migration_test'").Scan(&count); err != nil || count != 0 {
		t.Fatalf("DDL did not roll back: %d %v", count, err)
	}
	if err := s.db.QueryRow("SELECT count(*) FROM schema_migrations").Scan(&count); err != nil || count != 1 {
		t.Fatalf("failed migration advanced history: %d %v", count, err)
	}
	good := migration{2, "002_test.sql", "CREATE TABLE migration_test(value TEXT); INSERT INTO migration_test VALUES('durable-fact');", "test-only"}
	if err := migrate(context.Background(), s.db, Node, append(migrations, good)); err != nil {
		t.Fatal(err)
	}
	var fact, identity string
	if err := s.db.QueryRow("SELECT value FROM migration_test").Scan(&fact); err != nil || fact != "durable-fact" {
		t.Fatal("successful migration lost fact", err)
	}
	if err := s.db.QueryRow("SELECT instance_id FROM store_identity").Scan(&identity); err != nil || identity != original.InstanceID {
		t.Fatal("upgrade replaced identity", err)
	}
	s.Close()
	if old, err := Open(context.Background(), dir, Node); err == nil {
		old.Close()
		t.Fatal("old binary accepted upgraded schema")
	}
	// Read through a separate SQLite handle to prove rejected downgrade preserved
	// the committed migration and payload after closing/reopening the database.
	db, err := sql.Open("sqlite", databaseURI(filepath.Join(dir, "state.db")))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := db.QueryRow("SELECT value FROM migration_test").Scan(&fact); err != nil || fact != "durable-fact" {
		t.Fatal("downgrade attempt lost durable fact", err)
	}
}

func TestInitialMigrationRollbackAndRetry(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.db")
	if err := os.WriteFile(path, nil, 0600); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", databaseURI(path))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	migrations, err := loadMigrations(Server)
	if err != nil {
		t.Fatal(err)
	}
	migrations = append(migrations, migration{3, "003_failure.sql", "CREATE TABLE partial(value TEXT); INSERT INTO missing VALUES(1);", "test-only"})
	if err := migrate(context.Background(), db, Server, migrations); err == nil {
		t.Fatal("invalid initial migration accepted")
	}
	var count int
	if err := db.QueryRow("SELECT count(*) FROM sqlite_master WHERE type='table'").Scan(&count); err != nil || count != 0 {
		t.Fatalf("initial schema/history not rolled back: %d %v", count, err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	openTestStore(t, dir, Server)
}

func TestInvalidHistoryAndIdentityFailClosed(t *testing.T) {
	for name, stmt := range map[string]string{
		"future":       "INSERT INTO schema_migrations VALUES(3,'003_future.sql','x','now')",
		"gap":          "DELETE FROM schema_migrations WHERE version=1",
		"checksum":     "UPDATE schema_migrations SET checksum='changed'",
		"name":         "UPDATE schema_migrations SET name='changed.sql'",
		"empty":        "DELETE FROM schema_migrations",
		"no-history":   "DROP TABLE schema_migrations",
		"no-identity":  "DELETE FROM store_identity",
		"bad-identity": "UPDATE store_identity SET instance_id='invalid'",
	} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			s := openTestStore(t, dir, Server)
			execSQL(t, s.db, stmt)
			s.Close()
			if bad, err := Open(context.Background(), dir, Server); err == nil {
				bad.Close()
				t.Fatal("invalid store accepted")
			}
			lock, err := acquireLock(dir)
			if err != nil {
				t.Fatalf("failed open leaked lock: %v", err)
			}
			lock.Close()
		})
	}
}

func TestOpenFailureReleasesLock(t *testing.T) {
	dir := t.TempDir()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if s, err := Open(ctx, dir, Node); err == nil {
		s.Close()
		t.Fatal("cancelled context accepted")
	}
	openTestStore(t, dir, Node)
	for _, name := range []string{"daemon.lock", "state.db", "state.db-wal", "state.db-shm", "state.db-journal"} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.Mkdir(filepath.Join(dir, name), 0700); err != nil {
				t.Fatal(err)
			}
			if s, err := Open(context.Background(), dir, Node); err == nil {
				s.Close()
				t.Fatal("directory accepted as a state file")
			}
		})
	}
}

func TestReadOnlyDatabaseRejected(t *testing.T) {
	dir := t.TempDir()
	s := openTestStore(t, dir, Server)
	s.Close()
	path := filepath.Join(dir, "state.db")
	if err := os.Chmod(path, 0400); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(path, 0600) })
	// Root/CAP_DAC_OVERRIDE can bypass POSIX mode bits; do not call that a pass.
	probe, probeErr := os.OpenFile(path, os.O_RDWR, 0)
	if probeErr == nil {
		probe.Close()
		t.Skip("OS identity bypasses read-only mode bits")
	}
	if s, err := Open(context.Background(), dir, Server); err == nil {
		s.Close()
		t.Fatal("read-only database accepted")
	}
	if err := os.Chmod(path, 0600); err != nil {
		t.Fatal(err)
	}
	openTestStore(t, dir, Server)
}

// This helper runs only in a real subprocess, holding the Store until its stdin
// closes. Parent tests also kill it to check kernel lock release after a crash.
func TestLockOwnerHelper(t *testing.T) {
	dir := os.Getenv("RUNWEAVE_LOCK_HELPER")
	if dir == "" {
		return
	}
	s, err := Open(context.Background(), dir, Node)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.NewEncoder(os.Stdout).Encode(s.Info()); err != nil {
		t.Fatal(err)
	}
	_, _ = io.Copy(io.Discard, os.Stdin)
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestProcessLockAndCrashRelease(t *testing.T) {
	for _, terminate := range []bool{false, true} {
		name := "normal-exit"
		if terminate {
			name = "forced-exit"
		}
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			cmd := exec.Command(os.Args[0], "-test.run=^TestLockOwnerHelper$")
			cmd.Env = append(os.Environ(), "RUNWEAVE_LOCK_HELPER="+dir)
			input, err := cmd.StdinPipe()
			if err != nil {
				t.Fatal(err)
			}
			output, err := cmd.StdoutPipe()
			if err != nil {
				t.Fatal(err)
			}
			var stderr strings.Builder
			cmd.Stderr = &stderr
			if err := cmd.Start(); err != nil {
				t.Fatal(err)
			}
			exited := make(chan struct{})
			var waitErr error
			go func() { waitErr = cmd.Wait(); close(exited) }()
			t.Cleanup(func() {
				input.Close()
				_ = cmd.Process.Kill()
				<-exited
			})
			ready := make(chan string, 1)
			go func() {
				line, _ := bufio.NewReader(output).ReadString('\n')
				ready <- line
			}()
			var info Info
			select {
			case line := <-ready:
				if err := json.Unmarshal([]byte(line), &info); err != nil {
					t.Fatalf("helper failed to report ready: %q %v", line, err)
				}
			case <-time.After(15 * time.Second):
				t.Fatal("helper startup timed out")
			}
			if s, err := Open(context.Background(), dir, Node); !errors.Is(err, ErrLocked) {
				if s != nil {
					s.Close()
				}
				t.Fatalf("second process bypassed lock: %v", err)
			}
			if terminate {
				if err := cmd.Process.Kill(); err != nil {
					t.Fatal(err)
				}
			} else {
				input.Close()
			}
			select {
			case <-exited:
				if !terminate && waitErr != nil {
					t.Fatalf("helper exit: %v %s", waitErr, &stderr)
				}
			case <-time.After(15 * time.Second):
				t.Fatal("helper exit timed out")
			}
			s := openTestStore(t, dir, Node)
			if s.Info() != info {
				t.Fatal("crash/reopen changed durable identity")
			}
			if _, err := os.Stat(filepath.Join(dir, "daemon.lock")); err != nil {
				t.Fatal("lock file removed", err)
			}
		})
	}
}

func TestDatabasePathWithQuestionMark(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("question marks are not valid Windows filenames")
	}
	s := openTestStore(t, filepath.Join(t.TempDir(), "state?mode=ro"), Server)
	if s.Info().SchemaVersion != 2 {
		t.Fatal("URI interpreted path as options")
	}
}
