package store

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"runweave/internal/identity"
)

func TestIdentityAdminAndRevocationWhileDaemonOwnsStore(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	s := openTestStore(t, dir, Server)
	admin, err := OpenIdentityAdmin(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close()
	var savedToken string
	for _, role := range []identity.Role{identity.Principal, identity.Node} {
		subject := identity.Subject{Role: role, ID: "same-id"}
		token, err := identity.NewToken(role)
		if err != nil {
			t.Fatal(err)
		}
		if err := admin.Create(ctx, subject, token); err != nil {
			t.Fatal(err)
		}
		got, err := s.Authenticate(ctx, token, role)
		if err != nil || got != subject {
			t.Fatalf("authenticate: %+v %v", got, err)
		}
		other := identity.Principal
		if role == identity.Principal {
			other = identity.Node
		}
		if _, err := s.Authenticate(ctx, token, other); !errors.Is(err, identity.ErrUnauthorized) {
			t.Fatal("role confusion", err)
		}
		random, _ := identity.NewToken(role)
		if _, err := s.Authenticate(ctx, random, role); !errors.Is(err, identity.ErrUnauthorized) {
			t.Fatal("unknown credential accepted", err)
		}
		if err := admin.Create(ctx, subject, random); err == nil {
			t.Fatal("existing identity overwritten")
		}
		// Reusing the digest for a new identity must roll back the identity insert.
		if err := admin.Create(ctx, identity.Subject{Role: role, ID: "duplicate-token"}, token); err == nil {
			t.Fatal("digest reused")
		}
		table, _ := identityTable(role)
		var count int
		if err := s.db.QueryRow("SELECT count(*) FROM " + table + " WHERE id='duplicate-token'").Scan(&count); err != nil || count != 0 {
			t.Fatal("partial identity committed", err)
		}
		var stored []byte
		if err := s.db.QueryRow("SELECT digest FROM token_records WHERE role=?", role).Scan(&stored); err != nil || len(stored) != 32 || string(stored) == token {
			t.Fatal("not digest-only storage", err)
		}
		if err := admin.Revoke(ctx, subject); err != nil {
			t.Fatal(err)
		}
		if err := admin.Revoke(ctx, subject); err != nil {
			t.Fatal("revoke not idempotent", err)
		}
		if _, err := s.Authenticate(ctx, token, role); !errors.Is(err, identity.ErrUnauthorized) {
			t.Fatal("revocation not visible to daemon", err)
		}
		if err := admin.Create(ctx, subject, random); err == nil {
			t.Fatal("revoked identity reused")
		}
		if role == identity.Node {
			savedToken = token
		}
	}
	admin.Close()
	s.Close()
	s = openTestStore(t, dir, Server)
	if _, err := s.Authenticate(ctx, savedToken, identity.Node); !errors.Is(err, identity.ErrUnauthorized) {
		t.Fatal("restart forgot revocation", err)
	}
}

func TestRevocationRollback(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	s := openTestStore(t, dir, Server)
	a, err := OpenIdentityAdmin(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	subject := identity.Subject{Role: identity.Principal, ID: "p1"}
	token, _ := identity.NewToken(subject.Role)
	if err := a.Create(ctx, subject, token); err != nil {
		t.Fatal(err)
	}
	execSQL(t, s.db, "CREATE TRIGGER fail_revoke BEFORE UPDATE ON token_records BEGIN SELECT RAISE(ABORT,'injected storage failure'); END")
	if err := a.Revoke(ctx, subject); err == nil {
		t.Fatal("injected failure ignored")
	}
	if _, err := s.Authenticate(ctx, token, subject.Role); err != nil {
		t.Fatal("partial revocation committed", err)
	}
}

func TestV1UpgradeAndAdminValidation(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	if a, err := OpenIdentityAdmin(ctx, dir); err == nil {
		a.Close()
		t.Fatal("admin initialized missing DB")
	}
	path := filepath.Join(dir, "state.db")
	if err := os.WriteFile(path, nil, 0600); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", databaseURI(path))
	if err != nil {
		t.Fatal(err)
	}
	migrations, err := loadMigrations(Server)
	if err != nil {
		t.Fatal(err)
	}
	if err := migrate(ctx, db, Server, migrations[:1]); err != nil {
		t.Fatal(err)
	}
	var original string
	if err := db.QueryRow("SELECT instance_id FROM store_identity").Scan(&original); err != nil {
		t.Fatal(err)
	}
	db.Close()
	if a, err := OpenIdentityAdmin(ctx, dir); err == nil {
		a.Close()
		t.Fatal("admin accepted old schema")
	}
	s := openTestStore(t, dir, Server)
	if s.Info().InstanceID != original || s.Info().SchemaVersion != 2 {
		t.Fatal("v1 upgrade lost identity")
	}
	a, err := OpenIdentityAdmin(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	a.Close()
	nodeDir := t.TempDir()
	openTestStore(t, nodeDir, Node)
	if a, err := OpenIdentityAdmin(ctx, nodeDir); err == nil {
		a.Close()
		t.Fatal("admin accepted Node DB")
	}
}
