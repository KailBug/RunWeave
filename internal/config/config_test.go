package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadResolvesPathsWithoutReadingCredentials(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "node.yaml")
	input := "version: 1\nrole: node\nstate_dir: ./state\nnode:\n  node_id: node-1\n  server_url: http://127.0.0.1:7788\n  token_file: ./secrets/missing-token\n"
	if err := os.WriteFile(path, []byte(input), 0600); err != nil {
		t.Fatal(err)
	}
	c, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if c.StateDir != filepath.Join(dir, "state") || c.Node.TokenFile != filepath.Join(dir, "secrets", "missing-token") || c.LogLevel != "info" {
		t.Fatalf("bad defaults or path resolution: %+v", c)
	}
	if _, err := os.Stat(c.StateDir); !os.IsNotExist(err) {
		t.Fatal("config check created state")
	}
}

func TestStrictConfig(t *testing.T) {
	server := "version: 1\nrole: server\nstate_dir: ./state\nserver: {}\n"
	client := func(url string) string {
		return "version: 1\nrole: mcp\nmcp:\n  server_url: " + url + "\n  token_file: ./token\n"
	}
	for _, tc := range []struct {
		name, input string
		valid       bool
	}{
		{"defaults", server, true},
		{"ipv6-loopback", client("http://[::1]:7788"), true},
		{"remote-tls", client("https://server.example:443/"), true},
		{"tls-listener", strings.Replace(server, "server: {}", "server:\n  listen: 0.0.0.0:7788\n  tls_cert_file: cert.pem\n  tls_key_file: key.pem", 1), true},
		{"unknown", server + "token: secret-canary\n", false},
		{"unknown-nested", strings.Replace(server, "{}", "{token: secret-canary}", 1), false},
		{"duplicate", server + "role: node\n", false},
		{"nested-duplicate", strings.Replace(server, "{}", "{listen: '127.0.0.1:7788', listen: '0.0.0.0:7788'}", 1), false},
		{"anchor", strings.Replace(server, "{}", "&settings {}", 1), false},
		{"merge", strings.Replace(server, "{}", "{<<: {listen: '127.0.0.1:7788'}}", 1), false},
		{"alias", strings.Replace(server, "{}", "*settings", 1), false},
		{"null", strings.Replace(server, "{}", "null", 1), false},
		{"null-optional", server + "log_level: null\n", false},
		{"typed-string", strings.Replace(server, "./state", "123", 1), false},
		{"quoted-version", strings.Replace(server, "version: 1", "version: '1'", 1), false},
		{"custom-tag", strings.Replace(server, "./state", "!path ./state", 1), false},
		{"unsupported-version", strings.Replace(server, "version: 1", "version: 2", 1), false},
		{"missing-version", strings.Replace(server, "version: 1\n", "", 1), false},
		{"extra-doc", server + "---\n" + server, false},
		{"trailing-empty-doc", server + "---\n", false},
		{"malformed", "password: [secret-canary", false},
		{"empty", "", false},
		{"scalar", "secret-canary", false},
		{"list", "[secret-canary]", false},
		{"mixed-role", server + "mcp: {}\n", false},
		{"missing-state", strings.Replace(server, "state_dir: ./state\n", "", 1), false},
		{"bad-log-level", server + "log_level: secret-canary\n", false},
		{"any-address-http", strings.Replace(server, "{}", "{listen: '0.0.0.0:7788'}", 1), false},
		{"empty-host", strings.Replace(server, "{}", "{listen: ':7788'}", 1), false},
		{"listener-hostname", strings.Replace(server, "{}", "{listen: 'localhost:7788'}", 1), false},
		{"port-zero", strings.Replace(server, "{}", "{listen: '127.0.0.1:0'}", 1), false},
		{"half-tls", strings.Replace(server, "{}", "{tls_cert_file: cert.pem}", 1), false},
		{"remote-http", client("http://server.example:7788"), false},
		{"http-localhost", client("http://localhost:7788"), false},
		{"userinfo", client("https://user:secret-canary@server.example"), false},
		{"query", client("https://server.example/?token=secret-canary"), false},
		{"path", client("https://server.example/secret-canary"), false},
		{"fragment", client("https://server.example/#secret-canary"), false},
		{"empty-query", client("https://server.example/?"), false},
		{"high-port", client("https://server.example:65536"), false},
		{"empty-port", client("https://server.example:"), false},
		{"no-token", strings.Replace(client("https://server.example"), "  token_file: ./token\n", "", 1), false},
		{"mcp-state", client("https://server.example") + "state_dir: ./state\n", false},
		{"size", server + "#" + strings.Repeat("x", MaxBytes), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, err := decode([]byte(tc.input), t.TempDir())
			if (err == nil) != tc.valid {
				t.Fatalf("valid=%v, err=%v", tc.valid, err)
			}
			if err != nil && strings.Contains(err.Error(), "secret-canary") {
				t.Fatal("error leaked input")
			}
			if tc.name == "defaults" && c.Server.Listen != "127.0.0.1:7788" {
				t.Fatal("missing default listener")
			}
		})
	}
}

func TestLoadBoundaries(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "big.yaml")
	if err := os.WriteFile(path, []byte(strings.Repeat("x", MaxBytes+1)), 0600); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{path, dir, filepath.Join(dir, "missing")} {
		if _, err := Load(path); err == nil {
			t.Fatalf("accepted %s", path)
		}
	}
}

func TestDocumentedExamples(t *testing.T) {
	for _, role := range []string{"server", "node", "mcp"} {
		path := filepath.Join("..", "..", "docs", "资源", "P1配置", role+".yaml")
		c, err := Load(path)
		if err != nil || c.Role != role {
			t.Fatalf("documented %s config: %v", role, err)
		}
	}
}
