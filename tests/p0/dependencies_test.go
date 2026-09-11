package p0

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"go.yaml.in/yaml/v3"
	_ "modernc.org/sqlite"
)

func TestSQLiteTransactionsAndReopen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "probe.db")
	open := func() *sql.DB {
		t.Helper()
		db, err := sql.Open("sqlite", path)
		if err != nil {
			t.Fatal(err)
		}
		db.SetMaxOpenConns(1)
		t.Cleanup(func() { db.Close() })
		for _, pragma := range []string{"PRAGMA journal_mode=WAL", "PRAGMA foreign_keys=ON", "PRAGMA busy_timeout=1000"} {
			if _, err := db.Exec(pragma); err != nil {
				t.Fatal(err)
			}
		}
		return db
	}
	db := open()
	var mode, version string
	if err := db.QueryRow("PRAGMA journal_mode").Scan(&mode); err != nil || mode != "wal" {
		t.Fatalf("WAL: %q %v", mode, err)
	}
	if err := db.QueryRow("SELECT sqlite_version()").Scan(&version); err != nil {
		t.Fatal(err)
	}
	t.Logf("SQLite %s; journal_mode=%s", version, mode)
	var timeout int
	if err := db.QueryRow("PRAGMA busy_timeout").Scan(&timeout); err != nil || timeout != 1000 {
		t.Fatalf("busy_timeout: %d %v", timeout, err)
	}
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	for _, stmt := range []string{
		"CREATE TABLE schema_migrations(version INTEGER PRIMARY KEY)",
		"CREATE TABLE executions(id TEXT PRIMARY KEY, request_id TEXT NOT NULL UNIQUE)",
		"CREATE TABLE events(execution_id TEXT NOT NULL REFERENCES executions(id))",
		"INSERT INTO schema_migrations VALUES(1)",
		"INSERT INTO executions VALUES('e1','r1')",
		"INSERT INTO events VALUES('e1')",
	} {
		if _, err := tx.Exec(stmt); err != nil {
			t.Fatal(err)
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("INSERT INTO events VALUES('missing')"); err == nil {
		t.Fatal("foreign key accepted")
	}
	if _, err := db.Exec("INSERT INTO executions VALUES('e2','r1')"); err == nil {
		t.Fatal("duplicate request accepted")
	}
	tx, err = db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if _, err := tx.Exec("CREATE TABLE failed_migration(id INTEGER)"); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec("INSERT INTO schema_migrations VALUES(1)"); err == nil {
		t.Fatal("migration should fail")
	}
	if err := tx.Rollback(); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	db = open()
	for query, want := range map[string]int{
		"SELECT count(*) FROM executions WHERE id='e1' AND request_id='r1'": 1,
		"SELECT count(*) FROM events WHERE execution_id='e1'":               1,
		"SELECT max(version) FROM schema_migrations":                        1,
		"SELECT count(*) FROM sqlite_master WHERE name='failed_migration'":  0,
	} {
		var got int
		if err := db.QueryRow(query).Scan(&got); err != nil || got != want {
			t.Fatalf("%s: got %d want %d: %v", query, got, want, err)
		}
	}
}

func TestWebSocketBoundaries(t *testing.T) {
	for _, scenario := range []string{"echo", "oversize", "disconnect", "deadline"} {
		t.Run(scenario, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			serverResult := make(chan error, 1)
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				c, err := websocket.Accept(w, r, nil)
				if err != nil {
					serverResult <- err
					return
				}
				defer c.CloseNow()
				c.SetReadLimit(1 << 20)
				if scenario == "disconnect" {
					serverResult <- c.CloseNow()
					return
				}
				if scenario == "deadline" {
					<-ctx.Done()
					serverResult <- nil
					return
				}
				typ, data, err := c.Read(ctx)
				if err == nil && scenario == "echo" {
					err = c.Write(ctx, typ, data)
				}
				serverResult <- err
			}))
			defer srv.Close()
			c, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(srv.URL, "http"), nil)
			if err != nil {
				t.Fatal(err)
			}
			defer c.CloseNow()
			switch scenario {
			case "echo":
				want := []byte("RunWeave-中文")
				if err := c.Write(ctx, websocket.MessageText, want); err != nil {
					t.Fatal(err)
				}
				typ, got, err := c.Read(ctx)
				if err != nil || typ != websocket.MessageText || !bytes.Equal(got, want) {
					t.Fatalf("echo: %q %v", got, err)
				}
			case "oversize":
				// The peer may close before Write completes; Read must observe 1009.
				_ = c.Write(ctx, websocket.MessageBinary, bytes.Repeat([]byte("x"), (1<<20)+1))
				_, _, err := c.Read(ctx)
				if websocket.CloseStatus(err) != websocket.StatusMessageTooBig {
					t.Fatalf("oversize status: %v", err)
				}
			case "disconnect":
				if _, _, err := c.Read(ctx); err == nil {
					t.Fatal("disconnect not observed")
				}
			case "deadline":
				readCtx, stop := context.WithTimeout(ctx, 50*time.Millisecond)
				defer stop()
				if _, _, err := c.Read(readCtx); !errors.Is(err, context.DeadlineExceeded) {
					t.Fatalf("deadline: %v", err)
				}
				cancel()
			}
			select {
			case err := <-serverResult:
				if scenario == "oversize" {
					if err == nil {
						t.Fatal("server accepted oversized message")
					}
				} else if err != nil {
					t.Fatal(err)
				}
			case <-time.After(6 * time.Second):
				t.Fatal("server handler did not finish")
			}
		})
	}
}

// This is an experiment configuration, not the product configuration schema.
func decodeProbeConfig(input string) error {
	var config struct {
		Version int    `yaml:"version"`
		Name    string `yaml:"name"`
	}
	d := yaml.NewDecoder(strings.NewReader(input))
	d.KnownFields(true)
	if err := d.Decode(&config); err != nil {
		return err
	}
	if config.Version != 1 || config.Name == "" {
		return fmt.Errorf("invalid probe configuration")
	}
	var extra any
	if err := d.Decode(&extra); !errors.Is(err, io.EOF) {
		return fmt.Errorf("expected exactly one YAML document")
	}
	return nil
}

func TestYAMLStrictConfig(t *testing.T) {
	for _, tc := range []struct {
		name, input string
		valid       bool
	}{
		{"valid", "version: 1\nname: 中文\n", true},
		{"unknown", "version: 1\nname: x\nsecret: x\n", false},
		{"duplicate", "version: 1\nname: x\nname: y\n", false},
		{"multiple", "version: 1\nname: x\n---\nversion: 1\nname: y\n", false},
		{"empty-trailing-document", "version: 1\nname: x\n---\n", false},
		{"version", "version: 2\nname: x\n", false},
		{"missing", "name: x\n", false},
		{"malformed", "version: [\n", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := decodeProbeConfig(tc.input); (err == nil) != tc.valid {
				t.Fatalf("valid=%v: %v", tc.valid, err)
			}
		})
	}
}
