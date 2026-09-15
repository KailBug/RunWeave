// Package controlclient provides bounded, authenticated control HTTP calls.
package controlclient

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"runweave/internal/config"
	"runweave/internal/identity"
	"runweave/internal/secretfile"
	"runweave/internal/strictjson"
)

// Check authenticates one principal or verifies the configured Node ID. It does
// not register a Node or imply execution readiness. No credentials are cached.
func Check(ctx context.Context, origin, tokenFile, caFile string, expected identity.Subject) (identity.Subject, error) {
	if err := config.ValidateClientOrigin(origin); err != nil {
		return identity.Subject{}, err
	}
	if expected.Role != identity.Principal && !expected.Valid() {
		return identity.Subject{}, errors.New("invalid expected identity")
	}
	data, err := secretfile.Read(tokenFile, 256)
	if err != nil {
		return identity.Subject{}, err
	}
	token := strings.TrimSuffix(strings.TrimSuffix(string(data), "\n"), "\r")
	if _, err := identity.Digest(token, expected.Role); err != nil {
		return identity.Subject{}, errors.New("invalid credential file")
	}
	tlsConfig := &tls.Config{MinVersion: tls.VersionTLS12}
	if caFile != "" {
		file, err := os.Open(caFile)
		if err != nil {
			return identity.Subject{}, errors.New("cannot read TLS CA file")
		}
		defer file.Close()
		info, err := file.Stat()
		if err != nil || !info.Mode().IsRegular() {
			return identity.Subject{}, errors.New("TLS CA must be a regular file")
		}
		pem, err := io.ReadAll(io.LimitReader(file, (128<<10)+1))
		if err != nil || len(pem) > 128<<10 {
			return identity.Subject{}, errors.New("cannot read bounded TLS CA file")
		}
		roots, err := x509.SystemCertPool()
		if err != nil {
			roots = x509.NewCertPool()
		}
		if !roots.AppendCertsFromPEM(pem) {
			return identity.Subject{}, errors.New("invalid TLS CA file")
		}
		tlsConfig.RootCAs = roots
	}
	transport := &http.Transport{Proxy: nil, TLSClientConfig: tlsConfig, TLSHandshakeTimeout: 5 * time.Second, ResponseHeaderTimeout: 5 * time.Second}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: 10 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	path := "/v1/principal"
	if expected.Role == identity.Node {
		path = "/v1/nodes/" + url.PathEscape(expected.ID) + "/identity"
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimSuffix(origin, "/")+path, nil)
	if err != nil {
		return identity.Subject{}, errors.New("cannot create authentication request")
	}
	req.Header.Set("Authorization", "Bearer "+token)
	response, err := client.Do(req)
	if err != nil {
		return identity.Subject{}, errors.New("authentication connection failed; verify address and TLS trust")
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return identity.Subject{}, errors.New("authentication rejected or service unavailable")
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, 4097))
	if err != nil || len(body) > 4096 {
		return identity.Subject{}, errors.New("invalid authentication response")
	}
	var subject identity.Subject
	if err := strictjson.Decode(body, &subject); err != nil || !subject.Valid() || subject.Role != expected.Role || expected.ID != "" && subject.ID != expected.ID {
		return identity.Subject{}, errors.New("authentication identity mismatch")
	}
	return subject, nil
}
