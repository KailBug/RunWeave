package server

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"runweave/internal/config"
	"runweave/internal/controlclient"
	"runweave/internal/identity"
	"runweave/internal/secretfile"
	"runweave/internal/store"
	"runweave/tests/testcert"
)

type startupWriter struct{ address chan string }

func (w startupWriter) Write(p []byte) (int, error) {
	var event struct{ Msg, Address string }
	if json.Unmarshal(p, &event) == nil && event.Msg == "server listening" {
		w.address <- event.Address
	}
	return len(p), nil
}

func startDaemon(t *testing.T, dir string, c config.Server) (string, func()) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	address := make(chan string, 1)
	done := make(chan struct{})
	var runErr error
	go func() {
		runErr = Run(ctx, dir, c, slog.New(slog.NewJSONHandler(startupWriter{address}, nil)))
		close(done)
	}()
	stop := func() {
		cancel()
		select {
		case <-done:
			if runErr != nil {
				t.Error(runErr)
			}
		case <-time.After(10 * time.Second):
			t.Error("daemon shutdown timed out")
		}
	}
	t.Cleanup(stop)
	select {
	case addr := <-address:
		return addr, stop
	case <-done:
		t.Fatalf("daemon startup: %v", runErr)
	case <-time.After(15 * time.Second):
		t.Fatal("daemon startup timed out")
	}
	return "", stop
}

func TestDaemonAuthenticationAndLiveRevocation(t *testing.T) {
	dir := t.TempDir()
	addr, stop := startDaemon(t, dir, config.Server{Listen: "127.0.0.1:0"})
	ctx := context.Background()
	if second, err := store.Open(ctx, dir, store.Server); !errors.Is(err, store.ErrLocked) {
		if second != nil {
			second.Close()
		}
		t.Fatal("daemon did not retain state lock", err)
	}
	a, err := store.OpenIdentityAdmin(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	p, _ := identity.NewToken(identity.Principal)
	n, _ := identity.NewToken(identity.Node)
	for _, pair := range []struct {
		role      identity.Role
		id, token string
	}{{identity.Principal, "principal-1", p}, {identity.Node, "node-1", n}} {
		if err := a.Create(ctx, identity.Subject{Role: pair.role, ID: pair.id}, pair.token); err != nil {
			t.Fatal(err)
		}
	}
	client := &http.Client{Timeout: 3 * time.Second}
	request := func(path, method, token string, duplicate bool) (int, string) {
		t.Helper()
		r, _ := http.NewRequest(method, "http://"+addr+path, nil)
		if token != "" {
			r.Header.Set("Authorization", "Bearer "+token)
		}
		if duplicate {
			r.Header.Add("Authorization", "Bearer "+token)
		}
		r.Header.Set("X-Principal-ID", "forged")
		resp, err := client.Do(r)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		data, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(data), p) || strings.Contains(string(data), n) {
			t.Fatal("credential in response")
		}
		return resp.StatusCode, string(data)
	}
	for _, tc := range []struct {
		path, method, token string
		duplicate           bool
		status              int
	}{
		{"/healthz", "GET", "", false, 200}, {"/readyz", "GET", "", false, 503},
		{"/v1/principal", "GET", p, false, 200}, {"/v1/principal", "GET", n, false, 401},
		{"/v1/principal", "GET", "", false, 401}, {"/v1/principal", "GET", "bad-token", false, 401},
		{"/v1/principal", "GET", p, true, 401}, {"/v1/nodes/node-1/identity", "GET", n, false, 200},
		{"/v1/nodes/node-2/identity", "GET", n, false, 403}, {"/v1/nodes/node-1/identity", "GET", p, false, 401},
		{"/v1/principal?token=secret-canary", "GET", p, false, 400}, {"/v1/principal", "POST", p, false, 405},
	} {
		status, body := request(tc.path, tc.method, tc.token, tc.duplicate)
		if status != tc.status {
			t.Fatalf("%s: status %d want %d", tc.path, status, tc.status)
		}
		if tc.path == "/v1/principal" && status == 200 && !strings.Contains(body, `"id":"principal-1"`) {
			t.Fatal("caller identity override accepted")
		}
	}
	if err := a.Revoke(ctx, identity.Subject{Role: identity.Node, ID: "node-1"}); err != nil {
		t.Fatal(err)
	}
	if status, _ := request("/v1/nodes/node-1/identity", "GET", n, false); status != 401 {
		t.Fatal("live revoke ineffective")
	}
	a.Close()
	stop()
	s, err := store.Open(ctx, dir, store.Server)
	if err != nil {
		t.Fatal("stop did not release lock", err)
	}
	s.Close()
}

func TestDaemonTLSAndClientIdentityCheck(t *testing.T) {
	for _, validHostname := range []bool{true, false} {
		name := "valid-host"
		if !validHostname {
			name = "wrong-host"
		}
		t.Run(name, func(t *testing.T) {
			files := testcert.Make(t, validHostname)
			dir := t.TempDir()
			addr, _ := startDaemon(t, dir, config.Server{Listen: "127.0.0.1:0", TLSCertFile: files.CertFile, TLSKeyFile: files.KeyFile})
			a, err := store.OpenIdentityAdmin(context.Background(), dir)
			if err != nil {
				t.Fatal(err)
			}
			defer a.Close()
			token, _ := identity.NewToken(identity.Principal)
			subject := identity.Subject{Role: identity.Principal, ID: "tls-principal"}
			if err := a.Create(context.Background(), subject, token); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(t.TempDir(), "token")
			if err := secretfile.Create(path, []byte(token+"\n")); err != nil {
				t.Fatal(err)
			}
			got, err := controlclient.Check(context.Background(), "https://"+addr, path, files.CertFile, identity.Subject{Role: identity.Principal})
			if validHostname && (err != nil || got != subject) {
				t.Fatal("trusted TLS failed", err)
			}
			if !validHostname && err == nil {
				t.Fatal("hostname verification bypassed")
			}
			if _, err := controlclient.Check(context.Background(), "https://"+addr, path, "", identity.Subject{Role: identity.Principal}); err == nil {
				t.Fatal("untrusted self-signed certificate accepted")
			}
			if err != nil && strings.Contains(err.Error(), token) {
				t.Fatal("client error leaked token")
			}
		})
	}
}

func TestBadTLSFailsBeforeStateAndListen(t *testing.T) {
	for _, c := range []config.Server{{Listen: "0.0.0.0:0"}, {Listen: "127.0.0.1:0", TLSCertFile: "missing"}, {Listen: "127.0.0.1:0", TLSCertFile: "missing", TLSKeyFile: "missing"}} {
		dir := filepath.Join(t.TempDir(), "state")
		if err := Run(context.Background(), dir, c, slog.New(slog.NewJSONHandler(io.Discard, nil))); err == nil {
			t.Fatal("invalid TLS accepted")
		}
		if _, err := os.Stat(dir); !os.IsNotExist(err) {
			t.Fatal("TLS failure mutated state")
		}
	}
}

type failedAuth struct{}

func (failedAuth) Authenticate(context.Context, string, identity.Role) (identity.Subject, error) {
	return identity.Subject{}, errors.New("storage failed")
}

func TestAuthenticationStorageFailureIsUnavailable(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/v1/principal", nil)
	req.Header.Set("Authorization", "Bearer value")
	response := httptest.NewRecorder()
	newHandler(failedAuth{}).ServeHTTP(response, req)
	if response.Code != 503 || !strings.Contains(response.Body.String(), "STORAGE_UNAVAILABLE") {
		t.Fatal("storage failure treated as successful or invalid authentication")
	}
}

func TestBindFailureReleasesStore(t *testing.T) {
	addr, _ := startDaemon(t, t.TempDir(), config.Server{Listen: "127.0.0.1:0"})
	dir := t.TempDir()
	if err := Run(context.Background(), dir, config.Server{Listen: addr}, slog.New(slog.NewJSONHandler(io.Discard, nil))); err == nil {
		t.Fatal("occupied listener bound")
	}
	s, err := store.Open(context.Background(), dir, store.Server)
	if err != nil {
		t.Fatal("bind failure leaked lock", err)
	}
	s.Close()
}
