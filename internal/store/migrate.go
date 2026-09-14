package store

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"strconv"
	"strings"
	"time"
)

type migration struct {
	version  int
	name     string
	sql      string
	checksum string
}

func loadMigrations(role Role) ([]migration, error) {
	entries, err := fs.ReadDir(migrationFiles, "migrations/"+string(role))
	if err != nil {
		return nil, err
	}
	var out []migration
	for i, entry := range entries {
		name := entry.Name()
		prefix, _, ok := strings.Cut(name, "_")
		version, err := strconv.Atoi(prefix)
		if err != nil || !ok || version != i+1 || !strings.HasSuffix(name, ".sql") || entry.IsDir() {
			return nil, errors.New("invalid embedded migration sequence")
		}
		data, err := migrationFiles.ReadFile("migrations/" + string(role) + "/" + name)
		if err != nil {
			return nil, err
		}
		// Git line-ending conversion must not invalidate a released migration.
		content := strings.ReplaceAll(string(data), "\r\n", "\n")
		digest := sha256.Sum256([]byte(content))
		out = append(out, migration{version, name, content, hex.EncodeToString(digest[:])})
	}
	return out, nil
}

func migrate(ctx context.Context, db *sql.DB, role Role, migrations []migration) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var tables int
	if err := tx.QueryRowContext(ctx, "SELECT count(*) FROM sqlite_master WHERE type='table' AND name NOT LIKE 'sqlite_%'").Scan(&tables); err != nil {
		return err
	}
	// A nonempty database without our history is not an empty install.
	var hasHistory int
	if err := tx.QueryRowContext(ctx, "SELECT count(*) FROM sqlite_master WHERE type='table' AND name='schema_migrations'").Scan(&hasHistory); err != nil {
		return err
	}
	if tables != 0 && hasHistory == 0 {
		return errors.New("unrecognized state database without migration history")
	}
	if _, err := tx.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (
		version INTEGER PRIMARY KEY CHECK(version > 0),
		name TEXT NOT NULL,
		checksum TEXT NOT NULL,
		applied_at TEXT NOT NULL
	)`); err != nil {
		return err
	}
	rows, err := tx.QueryContext(ctx, "SELECT version, name, checksum FROM schema_migrations ORDER BY version")
	if err != nil {
		return err
	}
	count := 0
	for rows.Next() {
		var version int
		var name, checksum string
		if err := rows.Scan(&version, &name, &checksum); err != nil {
			rows.Close()
			return err
		}
		if count >= len(migrations) || version != count+1 || name != migrations[count].name || checksum != migrations[count].checksum {
			rows.Close()
			return errors.New("unsupported or modified migration history; automatic downgrade is forbidden")
		}
		count++
	}
	err = errors.Join(rows.Err(), rows.Close())
	if err != nil {
		return err
	}
	if count == 0 && tables != 0 {
		return errors.New("existing state database has empty migration history")
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	for _, m := range migrations[count:] {
		if _, err := tx.ExecContext(ctx, m.sql); err != nil {
			return fmt.Errorf("migration %d failed: %w", m.version, err)
		}
		if _, err := tx.ExecContext(ctx, "INSERT INTO schema_migrations(version,name,checksum,applied_at) VALUES(?,?,?,?)", m.version, m.name, m.checksum, now); err != nil {
			return err
		}
	}
	if count == 0 {
		var id [16]byte
		if _, err := rand.Read(id[:]); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, "INSERT INTO store_identity(singleton,role,instance_id,created_at,last_checked_at) VALUES(1,?,?,?,?)", role, hex.EncodeToString(id[:]), now, now); err != nil {
			return err
		}
	}
	var storedRole Role
	var instanceID string
	if err := tx.QueryRowContext(ctx, "SELECT role, instance_id FROM store_identity WHERE singleton=1").Scan(&storedRole, &instanceID); err != nil {
		return err
	}
	decoded, err := hex.DecodeString(instanceID)
	if storedRole != role || err != nil || len(decoded) != 16 {
		return errors.New("state role or instance identity is invalid")
	}
	return tx.Commit()
}
