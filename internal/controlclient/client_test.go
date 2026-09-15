package controlclient

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"runweave/internal/identity"
	"runweave/internal/secretfile"
)

func TestNoRedirectAndBoundedResponses(t *testing.T) {
	token, _ := identity.NewToken(identity.Principal)
	path := filepath.Join(t.TempDir(), "token")
	if err := secretfile.Create(path, []byte(token)); err != nil {
		t.Fatal(err)
	}
	var leaked atomic.Int32
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { leaked.Add(1) }))
	defer target.Close()
	for _, mode := range []string{"redirect", "oversize", "wrong-role", "wrong-id", "malformed", "duplicate", "unknown", "ok"} {
		t.Run(mode, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("Authorization") != "Bearer "+token {
					t.Error("missing bearer credential")
				}
				switch mode {
				case "redirect":
					http.Redirect(w, r, target.URL, http.StatusTemporaryRedirect)
				case "oversize":
					fmt.Fprint(w, strings.Repeat("x", 4097))
				case "wrong-role":
					fmt.Fprint(w, `{"role":"node","id":"p1"}`)
				case "wrong-id":
					fmt.Fprint(w, `{"role":"principal","id":"p2"}`)
				case "malformed":
					fmt.Fprint(w, `[]`)
				case "duplicate":
					fmt.Fprint(w, `{"role":"principal","id":"p2","id":"p1"}`)
				case "unknown":
					fmt.Fprint(w, `{"role":"principal","id":"p1","extra":true}`)
				case "ok":
					fmt.Fprint(w, `{"role":"principal","id":"p1"}`)
				}
			}))
			defer srv.Close()
			_, err := Check(context.Background(), srv.URL, path, "", identity.Subject{Role: identity.Principal, ID: "p1"})
			if (err == nil) != (mode == "ok") {
				t.Fatalf("%s: %v", mode, err)
			}
			if leaked.Load() != 0 {
				t.Fatal("followed redirect")
			}
		})
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := Check(ctx, target.URL, path, "", identity.Subject{Role: identity.Principal}); err == nil {
		t.Fatal("cancelled request succeeded")
	}
	if _, err := Check(context.Background(), "http://remote.example", path, "", identity.Subject{Role: identity.Principal}); err == nil {
		t.Fatal("remote plaintext accepted")
	}
}
