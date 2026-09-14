// Package store owns local SQLite state and its process-lifetime directory lock.
package store

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

type Role string

const (
	Server Role = "server"
	Node   Role = "node"
)

//go:embed migrations/*/*.sql
var migrationFiles embed.FS

type Info struct {
	Role          Role   `json:"role"`
	InstanceID    string `json:"instance_id"`
	SchemaVersion int    `json:"schema_version"`
}

type Store struct {
	db        *sql.DB
	lock      *os.File
	info      Info
	closeOnce sync.Once
	closeErr  error
}

// Open performs startup validation and a committed write before returning. The
// caller must retain the Store for the entire daemon lifetime and then Close it.
func Open(ctx context.Context, dir string, role Role) (_ *Store, err error) {
	if role != Server && role != Node {
		return nil, errors.New("store role must be server or node")
	}
	if dir == "" {
		return nil, errors.New("state directory is required")
	}
	dir, err = filepath.Abs(dir)
	if err != nil {
		return nil, err
	}
	if err = os.MkdirAll(dir, 0700); err != nil {
		return nil, fmt.Errorf("create state directory: %w", err)
	}
	lock, err := acquireLock(dir)
	if err != nil {
		return nil, err
	}
	s := &Store{lock: lock}
	defer func() {
		if err != nil {
			err = errors.Join(err, s.Close())
		}
	}()
	path := filepath.Join(dir, "state.db")
	for _, name := range []string{path, path + "-wal", path + "-shm", path + "-journal"} {
		if err = regularOrAbsent(name); err != nil {
			return nil, err
		}
	}
	// Pre-create with private POSIX permissions; on Windows the parent's ACL is
	// authoritative. Existing permissions are never silently changed.
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, fmt.Errorf("open state database for writing: %w", err)
	}
	if err = f.Close(); err != nil {
		return nil, err
	}
	s.db, err = sql.Open("sqlite", databaseURI(path))
	if err != nil {
		return nil, err
	}
	s.db.SetMaxOpenConns(1)
	s.db.SetMaxIdleConns(1)
	var mode string
	if err = s.db.QueryRowContext(ctx, "PRAGMA journal_mode=WAL").Scan(&mode); err != nil {
		return nil, fmt.Errorf("enable WAL: %w", err)
	}
	if mode != "wal" {
		return nil, errors.New("state database did not enable WAL")
	}
	migrations, err := loadMigrations(role)
	if err != nil {
		return nil, err
	}
	if err = migrate(ctx, s.db, role, migrations); err != nil {
		return nil, err
	}
	if err = s.db.QueryRowContext(ctx, "SELECT role, instance_id FROM store_identity WHERE singleton=1").Scan(&s.info.Role, &s.info.InstanceID); err != nil {
		return nil, err
	}
	s.info.SchemaVersion = len(migrations)
	// A read-only open is not sufficient for readiness. Force a durable write
	// even when the schema is current. Business readiness is checked separately.
	if _, err = s.db.ExecContext(ctx, "UPDATE store_identity SET last_checked_at=? WHERE singleton=1", time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
		return nil, fmt.Errorf("state database is not writable: %w", err)
	}
	return s, nil
}

// databaseURI escapes reserved path characters before appending driver options.
func databaseURI(path string) string {
	p := filepath.ToSlash(path)
	if filepath.VolumeName(path) != "" && p[0] != '/' {
		p = "/" + p
	}
	u := url.URL{Scheme: "file", Path: p}
	q := url.Values{"mode": {"rw"}, "_pragma": {"foreign_keys(1)", "busy_timeout(1000)", "synchronous(FULL)"}}
	u.RawQuery = q.Encode()
	return u.String()
}

func (s *Store) Info() Info { return s.info }

func (s *Store) Close() error {
	s.closeOnce.Do(func() {
		if s.db != nil {
			s.closeErr = s.db.Close()
		}
		if s.lock != nil {
			s.closeErr = errors.Join(s.closeErr, s.lock.Close())
		}
	})
	return s.closeErr
}
