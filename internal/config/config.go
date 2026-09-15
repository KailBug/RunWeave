// Package config loads versioned startup configuration without reading secrets.
package config

import (
	"bytes"
	"errors"
	"io"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"

	"go.yaml.in/yaml/v3"
	"runweave/internal/contract"
)

const MaxBytes = 64 << 10

type Config struct {
	Version  int     `yaml:"version"`
	Role     string  `yaml:"role"`
	StateDir string  `yaml:"state_dir"`
	LogLevel string  `yaml:"log_level"`
	Server   *Server `yaml:"server"`
	Node     *Node   `yaml:"node"`
	MCP      *Client `yaml:"mcp"`
}

type Server struct {
	Listen      string `yaml:"listen"`
	TLSCertFile string `yaml:"tls_cert_file"`
	TLSKeyFile  string `yaml:"tls_key_file"`
}

type Node struct {
	NodeID    string `yaml:"node_id"`
	ServerURL string `yaml:"server_url"`
	TokenFile string `yaml:"token_file"`
	TLSCAFile string `yaml:"tls_ca_file"`
}

type Client struct {
	ServerURL string `yaml:"server_url"`
	TokenFile string `yaml:"token_file"`
	TLSCAFile string `yaml:"tls_ca_file"`
}

// Load resolves relative paths against the configuration's directory, never cwd.
// Errors intentionally omit source text, URLs, and YAML parser diagnostics, which
// may contain accidentally pasted credentials.
func Load(path string) (Config, error) {
	var c Config
	abs, err := filepath.Abs(path)
	if err != nil {
		return c, errors.New("cannot resolve config path")
	}
	f, err := os.Open(abs)
	if err != nil {
		return c, errors.New("cannot open config file")
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() {
		return c, errors.New("config must be a regular file")
	}
	data, err := io.ReadAll(io.LimitReader(f, MaxBytes+1))
	if err != nil || len(data) > MaxBytes {
		return c, errors.New("config cannot be read or exceeds 64 KiB")
	}
	return decode(data, filepath.Dir(abs))
}

func decode(data []byte, base string) (Config, error) {
	var c Config
	if len(data) > MaxBytes {
		return c, errors.New("config exceeds 64 KiB")
	}
	d := yaml.NewDecoder(bytes.NewReader(data))
	var doc yaml.Node
	if err := d.Decode(&doc); err != nil || len(doc.Content) != 1 {
		return c, errors.New("invalid YAML configuration")
	}
	var extra yaml.Node
	if err := d.Decode(&extra); !errors.Is(err, io.EOF) {
		return c, errors.New("config requires exactly one YAML document")
	}
	if !validShape(doc.Content[0], reflect.TypeOf(c)) {
		return c, errors.New("invalid config field, type, duplicate key, or YAML alias")
	}
	if err := doc.Decode(&c); err != nil {
		return Config{}, errors.New("invalid YAML configuration")
	}
	if err := c.validate(); err != nil {
		return Config{}, err
	}
	resolve := func(p *string) {
		if *p != "" && !filepath.IsAbs(*p) {
			*p = filepath.Join(base, *p)
		}
		if *p != "" {
			*p = filepath.Clean(*p)
		}
	}
	resolve(&c.StateDir)
	if c.Server != nil {
		resolve(&c.Server.TLSCertFile)
		resolve(&c.Server.TLSKeyFile)
	}
	if c.Node != nil {
		resolve(&c.Node.TokenFile)
		resolve(&c.Node.TLSCAFile)
	}
	if c.MCP != nil {
		resolve(&c.MCP.TokenFile)
		resolve(&c.MCP.TLSCAFile)
	}
	return c, nil
}

// yaml.v3 intentionally coerces some scalar types. Check the AST first so that
// numbers cannot silently become strings and aliases cannot hide duplicate keys.
func validShape(n *yaml.Node, typ reflect.Type) bool {
	if n.Anchor != "" || n.Alias != nil || n.Kind == yaml.AliasNode {
		return false
	}
	if typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
	}
	switch typ.Kind() {
	case reflect.Struct:
		if n.Kind != yaml.MappingNode || n.Tag != "!!map" {
			return false
		}
		fields := make(map[string]reflect.Type)
		for i := 0; i < typ.NumField(); i++ {
			f := typ.Field(i)
			fields[f.Tag.Get("yaml")] = f.Type
		}
		seen := make(map[string]bool)
		for i := 0; i < len(n.Content); i += 2 {
			key, value := n.Content[i], n.Content[i+1]
			field, ok := fields[key.Value]
			if !ok || seen[key.Value] || !validShape(key, reflect.TypeOf("")) || !validShape(value, field) {
				return false
			}
			seen[key.Value] = true
		}
		return true
	case reflect.String:
		return n.Kind == yaml.ScalarNode && n.Tag == "!!str" && !strings.ContainsRune(n.Value, 0)
	case reflect.Int:
		return n.Kind == yaml.ScalarNode && n.Tag == "!!int"
	}
	return false
}

func (c *Config) validate() error {
	if c.Version != 1 {
		return errors.New("unsupported config version: expected version 1")
	}
	if c.LogLevel == "" {
		c.LogLevel = "info"
	}
	switch c.LogLevel {
	case "debug", "info", "warn", "error":
	default:
		return errors.New("invalid log_level")
	}
	if strings.TrimSpace(c.StateDir) != c.StateDir {
		return errors.New("state_dir cannot have surrounding whitespace")
	}
	switch c.Role {
	case "server":
		if c.StateDir == "" || c.Server == nil || c.Node != nil || c.MCP != nil {
			return errors.New("server requires state_dir and only a server block")
		}
		s := c.Server
		if s.Listen == "" {
			s.Listen = "127.0.0.1:7788"
		}
		host, port, err := net.SplitHostPort(s.Listen)
		ip := net.ParseIP(host)
		if err != nil || ip == nil || !validPort(port) {
			return errors.New("server.listen requires an IP literal and port 1..65535")
		}
		if (s.TLSCertFile == "") != (s.TLSKeyFile == "") {
			return errors.New("TLS certificate and key must be configured together")
		}
		if !ip.IsLoopback() && s.TLSCertFile == "" {
			return errors.New("non-loopback listener requires TLS certificate and key")
		}
	case "node":
		if c.StateDir == "" || c.Node == nil || c.Server != nil || c.MCP != nil {
			return errors.New("node requires state_dir and only a node block")
		}
		if !contract.ValidIdentifier(c.Node.NodeID) {
			return errors.New("invalid node.node_id")
		}
		if err := validateClient(c.Node.ServerURL, c.Node.TokenFile); err != nil {
			return err
		}
	case "mcp":
		if c.StateDir != "" || c.MCP == nil || c.Server != nil || c.Node != nil {
			return errors.New("mcp requires only an mcp block and no state_dir")
		}
		if err := validateClient(c.MCP.ServerURL, c.MCP.TokenFile); err != nil {
			return err
		}
	default:
		return errors.New("role must be server, node, or mcp")
	}
	return nil
}

func validPort(port string) bool {
	n, err := strconv.Atoi(port)
	return err == nil && n >= 1 && n <= 65535
}

func validateClient(rawURL, tokenFile string) error {
	if strings.TrimSpace(tokenFile) == "" {
		return errors.New("token_file is required; inline tokens are not supported")
	}
	u, err := url.Parse(rawURL)
	if err != nil || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || u.Opaque != "" || (u.Path != "" && u.Path != "/") || u.RawPath != "" {
		return errors.New("server_url requires an origin URL without credentials, query, fragment, or path")
	}
	if u.Port() != "" && !validPort(u.Port()) || strings.HasSuffix(u.Host, ":") {
		return errors.New("invalid server_url port")
	}
	if u.Scheme != "https" {
		ip := net.ParseIP(u.Hostname())
		if u.Scheme != "http" || ip == nil || !ip.IsLoopback() {
			return errors.New("server_url requires HTTPS except for HTTP on a loopback IP literal")
		}
	}
	return nil
}

// ValidateClientOrigin applies the same fail-closed URL rules to runtime callers.
func ValidateClientOrigin(rawURL string) error {
	return validateClient(rawURL, "runtime-file-reference")
}
