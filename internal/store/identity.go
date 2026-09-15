package store

import (
	"context"
	"database/sql"
	"encoding/hex"
	"errors"
	"path/filepath"
	"time"

	"runweave/internal/identity"
)

// IdentityAdmin permits only local, short identity transactions. It neither
// migrates a database nor owns a daemon lock. State-directory OS access is its
// authority. All users of it must stop before upgrades or backup/restore.
type IdentityAdmin struct{ db *sql.DB }

func OpenIdentityAdmin(ctx context.Context, dir string) (_ *IdentityAdmin, err error) {
	path, err := filepath.Abs(filepath.Join(dir, "state.db"))
	if err != nil {
		return nil, err
	}
	if err := regularOrAbsent(path); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", databaseURI(path)) // mode=rw: never creates a missing DB.
	if err != nil {
		return nil, err
	}
	defer func() {
		if err != nil {
			db.Close()
		}
	}()
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	migrations, err := loadMigrations(Server)
	if err != nil {
		return nil, err
	}
	count, err := migrationCount(ctx, tx, migrations)
	if err != nil {
		return nil, err
	}
	if count != len(migrations) {
		return nil, errors.New("initialize or upgrade server state before identity administration")
	}
	var role, instanceID string
	if err := tx.QueryRowContext(ctx, "SELECT role,instance_id FROM store_identity WHERE singleton=1").Scan(&role, &instanceID); err != nil {
		return nil, err
	}
	rawID, decodeErr := hex.DecodeString(instanceID)
	if role != string(Server) || decodeErr != nil || len(rawID) != 16 {
		return nil, errors.New("identity administration requires server state")
	}
	return &IdentityAdmin{db: db}, nil
}

func (a *IdentityAdmin) Close() error { return a.db.Close() }

func identityTable(role identity.Role) (table, column string) {
	switch role {
	case identity.Principal:
		return "principals", "principal_id"
	case identity.Node:
		return "node_identities", "node_id"
	}
	return "", ""
}

func (a *IdentityAdmin) Create(ctx context.Context, subject identity.Subject, token string) error {
	if !subject.Valid() {
		return errors.New("invalid identity")
	}
	digest, err := identity.Digest(token, subject.Role)
	if err != nil {
		return err
	}
	table, column := identityTable(subject.Role) // Static SQL identifiers only.
	tx, err := a.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := tx.ExecContext(ctx, "INSERT INTO "+table+"(id,created_at) VALUES(?,?)", subject.ID, now); err != nil {
		return errors.New("cannot create identity; ID may already exist or storage is unavailable")
	}
	if _, err := tx.ExecContext(ctx, "INSERT INTO token_records(role,"+column+",digest,created_at) VALUES(?,?,?,?)", subject.Role, subject.ID, digest, now); err != nil {
		return errors.New("cannot persist credential")
	}
	return tx.Commit()
}

func (a *IdentityAdmin) Revoke(ctx context.Context, subject identity.Subject) error {
	if !subject.Valid() {
		return errors.New("invalid identity")
	}
	table, column := identityTable(subject.Role)
	tx, err := a.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	result, err := tx.ExecContext(ctx, "UPDATE "+table+" SET revoked_at=COALESCE(revoked_at,?) WHERE id=?", now, subject.ID)
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count != 1 {
		return errors.New("identity not found")
	}
	if _, err := tx.ExecContext(ctx, "UPDATE token_records SET revoked_at=COALESCE(revoked_at,?) WHERE "+column+"=?", now, subject.ID); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) Authenticate(ctx context.Context, token string, role identity.Role) (identity.Subject, error) {
	var subject identity.Subject
	if s.info.Role != Server {
		return subject, identity.ErrUnauthorized
	}
	digest, err := identity.Digest(token, role)
	if err != nil {
		return subject, identity.ErrUnauthorized
	}
	table, column := identityTable(role)
	err = s.db.QueryRowContext(ctx, "SELECT t.role, i.id FROM token_records t JOIN "+table+" i ON i.id=t."+column+" WHERE t.digest=? AND t.role=? AND t.revoked_at IS NULL AND i.revoked_at IS NULL", digest, role).Scan(&subject.Role, &subject.ID)
	if errors.Is(err, sql.ErrNoRows) {
		return identity.Subject{}, identity.ErrUnauthorized
	}
	if err != nil {
		return identity.Subject{}, err
	}
	if !subject.Valid() {
		return identity.Subject{}, identity.ErrUnauthorized
	}
	return subject, nil
}
