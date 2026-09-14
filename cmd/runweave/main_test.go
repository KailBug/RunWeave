package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"runweave/internal/store"
)

func TestCommandsAndOutputIsolation(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "server.yaml")
	if err := os.WriteFile(path, []byte("version: 1\nrole: server\nstate_dir: state\nserver: {}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	var firstID string
	for _, args := range [][]string{{"version"}, {"config", "check", "--config", path}, {"state", "init", "--config", path}, {"state", "init", "--config", path}} {
		var out, logs bytes.Buffer
		if code := run(context.Background(), args, &out, &logs); code != 0 {
			t.Fatalf("code %d: %s", code, &logs)
		}
		if !json.Valid(out.Bytes()) {
			t.Fatalf("stdout is not exactly one JSON value: %s", &out)
		}
		if logs.Len() != 0 && !json.Valid(logs.Bytes()) {
			t.Fatalf("stderr is not JSON diagnostic: %s", &logs)
		}
		if args[0] == "config" {
			if _, err := os.Stat(filepath.Join(dir, "state")); !os.IsNotExist(err) {
				t.Fatal("config check mutated state")
			}
		}
		if args[0] == "state" {
			var info store.Info
			if err := json.Unmarshal(out.Bytes(), &info); err != nil || info.Role != store.Server || info.InstanceID == "" {
				t.Fatalf("invalid result %s: %v", &out, err)
			}
			if firstID != "" && firstID != info.InstanceID {
				t.Fatal("state init replaced durable identity")
			}
			firstID = info.InstanceID
		}
	}
	s, err := store.Open(context.Background(), filepath.Join(dir, "state"), store.Server)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	var out, logs bytes.Buffer
	if code := run(context.Background(), []string{"state", "init", "--config", path}, &out, &logs); code != 1 || out.Len() != 0 || !strings.Contains(logs.String(), "locked") {
		t.Fatalf("init ignored active owner: %d %s %s", code, &out, &logs)
	}
}

func TestErrorsDoNotEchoConfigurationOrArguments(t *testing.T) {
	path := filepath.Join(t.TempDir(), "invalid.yaml")
	if err := os.WriteFile(path, []byte("version: 1\nrole: secret-canary\n"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{nil, {"server", "secret-canary"}, {"mcp"}, {"config", "check", "--config", path}, {"version", "secret-canary"}} {
		var out, logs bytes.Buffer
		if code := run(context.Background(), args, &out, &logs); code != 1 || out.Len() != 0 || !json.Valid(logs.Bytes()) || strings.Contains(logs.String(), "secret-canary") {
			t.Fatalf("bad error output: code=%d stdout=%s stderr=%s", code, &out, &logs)
		}
	}
}
