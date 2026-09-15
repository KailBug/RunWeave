// Package server runs the persistent HTTP control service.
package server

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"runweave/internal/config"
	"runweave/internal/identity"
	"runweave/internal/secretfile"
	"runweave/internal/store"
)

type authenticator interface {
	Authenticate(context.Context, string, identity.Role) (identity.Subject, error)
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func apiError(w http.ResponseWriter, status int, code string) {
	writeJSON(w, status, struct {
		Error string `json:"error"`
	}{code})
}

func newHandler(auth authenticator) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"ready": false, "reason": "execution_service_unavailable"})
	})
	authenticate := func(w http.ResponseWriter, r *http.Request, role identity.Role) (identity.Subject, bool) {
		values := r.Header.Values("Authorization")
		if len(values) != 1 {
			apiError(w, 401, "UNAUTHORIZED")
			return identity.Subject{}, false
		}
		scheme, token, ok := strings.Cut(values[0], " ")
		if !ok || !strings.EqualFold(scheme, "Bearer") {
			apiError(w, 401, "UNAUTHORIZED")
			return identity.Subject{}, false
		}
		subject, err := auth.Authenticate(r.Context(), token, role)
		if errors.Is(err, identity.ErrUnauthorized) {
			apiError(w, 401, "UNAUTHORIZED")
			return identity.Subject{}, false
		}
		if err != nil {
			apiError(w, 503, "STORAGE_UNAVAILABLE")
			return identity.Subject{}, false
		}
		return subject, true
	}
	mux.HandleFunc("GET /v1/principal", func(w http.ResponseWriter, r *http.Request) {
		if subject, ok := authenticate(w, r, identity.Principal); ok {
			writeJSON(w, 200, subject)
		}
	})
	mux.HandleFunc("GET /v1/nodes/{node_id}/identity", func(w http.ResponseWriter, r *http.Request) {
		subject, ok := authenticate(w, r, identity.Node)
		if !ok {
			return
		}
		if subject.ID != r.PathValue("node_id") {
			apiError(w, 403, "FORBIDDEN")
			return
		}
		writeJSON(w, 200, subject)
	})
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", "GET")
			apiError(w, 405, "METHOD_NOT_ALLOWED")
			return
		}
		if r.URL.RawQuery != "" || r.URL.ForceQuery || r.ContentLength != 0 {
			apiError(w, 400, "INVALID_REQUEST")
			return
		}
		mux.ServeHTTP(w, r)
	})
}

func serverTLS(c config.Server) (*tls.Config, error) {
	host, _, err := net.SplitHostPort(c.Listen)
	ip := net.ParseIP(host)
	if err != nil || ip == nil {
		return nil, errors.New("listener requires an IP literal and port")
	}
	if (c.TLSCertFile == "") != (c.TLSKeyFile == "") {
		return nil, errors.New("TLS certificate and key must be configured together")
	}
	if c.TLSCertFile == "" {
		if !ip.IsLoopback() {
			return nil, errors.New("non-loopback listener requires TLS")
		}
		return nil, nil
	}
	file, err := os.Open(c.TLSCertFile)
	if err != nil {
		return nil, errors.New("cannot read TLS certificate")
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		return nil, errors.New("cannot read TLS certificate")
	}
	cert, err := io.ReadAll(io.LimitReader(file, (128<<10)+1))
	if err != nil || len(cert) > 128<<10 {
		return nil, errors.New("cannot read TLS certificate")
	}
	key, err := secretfile.Read(c.TLSKeyFile, 64<<10)
	if err != nil {
		return nil, errors.New("cannot read private TLS key")
	}
	pair, err := tls.X509KeyPair(cert, key)
	if err != nil {
		return nil, errors.New("invalid TLS certificate or key pair")
	}
	return &tls.Config{MinVersion: tls.VersionTLS12, Certificates: []tls.Certificate{pair}}, nil
}

func Run(ctx context.Context, stateDir string, c config.Server, logger *slog.Logger) error {
	tlsConfig, err := serverTLS(c)
	if err != nil {
		return err
	}
	startup, cancel := context.WithTimeout(ctx, 10*time.Second)
	s, err := store.Open(startup, stateDir, store.Server)
	cancel()
	if err != nil {
		return err
	}
	defer s.Close()
	listener, err := net.Listen("tcp", c.Listen)
	if err != nil {
		return errors.New("cannot bind server listener")
	}
	defer listener.Close()
	if tlsConfig != nil {
		listener = tls.NewListener(listener, tlsConfig)
	}
	srv := &http.Server{
		Handler: newHandler(s), ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout: 10 * time.Second, WriteTimeout: 10 * time.Second, IdleTimeout: 30 * time.Second,
		MaxHeaderBytes: 16 << 10, ErrorLog: slog.NewLogLogger(logger.Handler(), slog.LevelWarn),
		BaseContext: func(net.Listener) context.Context { return ctx },
	}
	finished := make(chan error, 1)
	go func() { finished <- srv.Serve(listener) }()
	logger.Info("server listening", "address", listener.Addr().String(), "tls", tlsConfig != nil)
	select {
	case err := <-finished:
		_ = srv.Close()
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return errors.New("HTTP server stopped unexpectedly")
	case <-ctx.Done():
		shutdown, stop := context.WithTimeout(context.Background(), 5*time.Second)
		defer stop()
		if err := srv.Shutdown(shutdown); err != nil {
			_ = srv.Close()
		}
		<-finished
		logger.Info("server stopped")
		return s.Close()
	}
}
