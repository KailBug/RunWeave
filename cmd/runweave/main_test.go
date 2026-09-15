package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

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

func TestServerProcessHelper(t *testing.T) {
	path := os.Getenv("RUNWEAVE_SERVER_TEST_CONFIG")
	if path == "" {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _, _ = io.Copy(io.Discard, os.Stdin); cancel() }()
	var stdout bytes.Buffer
	if code := run(ctx, []string{"server", "serve", "--config", path}, &stdout, os.Stderr); code != 0 || stdout.Len() != 0 {
		t.Fatalf("server code=%d stdout bytes=%d", code, stdout.Len())
	}
}

func TestCLIServerIdentityLifecycleAcrossProcesses(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "server.yaml")
	// Reserve an available port for the product-config validator (which forbids 0).
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	address := listener.Addr().String()
	listener.Close()
	content := "version: 1\nrole: server\nstate_dir: state\nserver:\n  listen: " + address + "\n"
	if err := os.WriteFile(configPath, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(os.Args[0], "-test.run=^TestServerProcessHelper$")
	cmd.Env = append(os.Environ(), "RUNWEAVE_SERVER_TEST_CONFIG="+configPath)
	input, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	logs, err := cmd.StderrPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	var waitErr error
	go func() { waitErr = cmd.Wait(); close(done) }()
	t.Cleanup(func() { input.Close(); _ = cmd.Process.Kill(); <-done })
	ready := make(chan bool, 1)
	go func() {
		scanner := bufio.NewScanner(logs)
		sent := false
		for scanner.Scan() {
			if !sent && strings.Contains(scanner.Text(), `"msg":"server listening"`) {
				ready <- true
				sent = true
			}
		}
		if !sent {
			ready <- false
		}
	}()
	select {
	case ok := <-ready:
		if !ok {
			t.Fatal("daemon failed before ready")
		}
	case <-time.After(15 * time.Second):
		t.Fatal("daemon startup timed out")
	}
	call := func(want int, args ...string) string {
		t.Helper()
		var out, log bytes.Buffer
		if code := run(context.Background(), args, &out, &log); code != want {
			t.Fatalf("command %s: code=%d logs=%s", args[0], code, &log)
		}
		if want != 0 && out.Len() != 0 {
			t.Fatal("error wrote stdout")
		}
		if strings.Contains(out.String(), "rw_p_") || strings.Contains(out.String(), "rw_n_") || strings.Contains(log.String(), "rw_p_") || strings.Contains(log.String(), "rw_n_") {
			t.Fatal("CLI leaked token")
		}
		return out.String()
	}
	call(1, "server", "serve", "--config", configPath)
	for _, role := range []string{"principal", "node"} {
		id := role + "-1"
		tokenPath := filepath.Join(dir, id+".token")
		call(0, "identity", "create", "--config", configPath, "--role", role, "--id", id, "--token-file", tokenPath)
		original, err := os.ReadFile(tokenPath)
		if err != nil {
			t.Fatal(err)
		}
		call(1, "identity", "create", "--config", configPath, "--role", role, "--id", id, "--token-file", tokenPath)
		after, _ := os.ReadFile(tokenPath)
		if !bytes.Equal(original, after) {
			t.Fatal("existing token overwritten")
		}
		clientPath := filepath.Join(dir, id+".yaml")
		clientRole, block := "mcp", "mcp:\n"
		if role == "node" {
			clientRole = "node"
			block = "state_dir: node-state\nnode:\n  node_id: " + id + "\n"
		}
		body := "version: 1\nrole: " + clientRole + "\n" + block + "  server_url: http://" + address + "\n  token_file: " + id + ".token\n"
		if err := os.WriteFile(clientPath, []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
		out := call(0, "auth", "check", "--config", clientPath)
		if !strings.Contains(out, `"id":"`+id+`"`) {
			t.Fatal("wrong identity returned")
		}
		call(0, "identity", "revoke", "--config", configPath, "--role", role, "--id", id)
		call(1, "auth", "check", "--config", clientPath)
	}
	input.Close()
	select {
	case <-done:
		if waitErr != nil {
			t.Fatal(waitErr)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("daemon did not stop")
	}
	s, err := store.Open(context.Background(), filepath.Join(dir, "state"), store.Server)
	if err != nil {
		t.Fatal("daemon exit leaked store", err)
	}
	s.Close()
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
