// runweave is the product CLI. P0 probes remain separate binaries.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"runweave/internal/config"
	"runweave/internal/controlclient"
	"runweave/internal/identity"
	"runweave/internal/secretfile"
	"runweave/internal/server"
	"runweave/internal/store"
)

const version = "0.0.1-dev"

func run(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	level := new(slog.LevelVar)
	logger := slog.New(slog.NewJSONHandler(stderr, &slog.HandlerOptions{Level: level}))
	fail := func(err error) int {
		logger.Error("command failed", "error", err.Error())
		return 1
	}
	write := func(value any) int {
		if err := json.NewEncoder(stdout).Encode(value); err != nil {
			return fail(errors.New("cannot write command result"))
		}
		return 0
	}
	if len(args) == 1 && args[0] == "version" {
		return write(struct {
			Version string `json:"version"`
			Stage   string `json:"stage"`
		}{version, "p1-identity"})
	}
	action, options, err := parseCommand(args)
	if err != nil {
		return fail(err)
	}
	c, err := config.Load(options["--config"])
	if err != nil {
		return fail(err)
	}
	levels := map[string]slog.Level{"debug": slog.LevelDebug, "info": slog.LevelInfo, "warn": slog.LevelWarn, "error": slog.LevelError}
	level.Set(levels[c.LogLevel])
	if action == "config check" {
		logger.Info("configuration validated", "role", c.Role)
		return write(struct {
			Valid   bool   `json:"valid"`
			Version int    `json:"config_version"`
			Role    string `json:"role"`
		}{true, c.Version, c.Role})
	}
	if action == "server serve" {
		if c.Role != "server" {
			return fail(errors.New("server serve requires server configuration"))
		}
		if err := server.Run(ctx, c.StateDir, *c.Server, logger); err != nil {
			return fail(err)
		}
		return 0
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if action == "auth check" {
		var subject identity.Subject
		var err error
		switch c.Role {
		case "node":
			subject, err = controlclient.Check(ctx, c.Node.ServerURL, c.Node.TokenFile, c.Node.TLSCAFile, identity.Subject{Role: identity.Node, ID: c.Node.NodeID})
		case "mcp":
			subject, err = controlclient.Check(ctx, c.MCP.ServerURL, c.MCP.TokenFile, c.MCP.TLSCAFile, identity.Subject{Role: identity.Principal})
		default:
			return fail(errors.New("auth check requires node or mcp configuration"))
		}
		if err != nil {
			return fail(err)
		}
		return write(subject)
	}
	if action == "identity create" || action == "identity revoke" {
		if c.Role != "server" {
			return fail(errors.New("identity administration requires server configuration"))
		}
		subject := identity.Subject{Role: identity.Role(options["--role"]), ID: options["--id"]}
		if !subject.Valid() {
			return fail(errors.New("invalid identity role or ID"))
		}
		admin, err := store.OpenIdentityAdmin(ctx, c.StateDir)
		if err != nil {
			return fail(err)
		}
		defer admin.Close()
		if action == "identity revoke" {
			if err := admin.Revoke(ctx, subject); err != nil {
				return fail(err)
			}
			return write(struct {
				identity.Subject
				Revoked bool `json:"revoked"`
			}{subject, true})
		}
		token, err := identity.NewToken(subject.Role)
		if err != nil {
			return fail(errors.New("cannot generate credential"))
		}
		if err := secretfile.Create(options["--token-file"], []byte(token+"\n")); err != nil {
			return fail(err)
		}
		if err := admin.Create(ctx, subject, token); err != nil {
			return fail(errors.New("identity creation failed; private credential file retained for inspection"))
		}
		return write(subject)
	}
	if c.Role == "mcp" {
		return fail(errors.New("mcp bridge has no local daemon state"))
	}
	s, err := store.Open(ctx, c.StateDir, store.Role(c.Role))
	if err != nil {
		return fail(err)
	}
	info := s.Info()
	if err := s.Close(); err != nil {
		return fail(err)
	}
	logger.Info("state initialized and write check committed", "role", info.Role, "schema_version", info.SchemaVersion)
	return write(info)
}

func parseCommand(args []string) (string, map[string]string, error) {
	invalid := errors.New("usage: runweave version | config check | state init | server serve | auth check | identity create/revoke; use --config FILE; identity requires --role principal|node --id ID, create also requires --token-file FILE; node daemon and MCP serving are not implemented yet")
	if len(args) < 2 || len(args)%2 != 0 {
		return "", nil, invalid
	}
	action := args[0] + " " + args[1]
	required := []string{"--config"}
	switch action {
	case "config check", "state init", "server serve", "auth check":
	case "identity create":
		required = append(required, "--role", "--id", "--token-file")
	case "identity revoke":
		required = append(required, "--role", "--id")
	default:
		return "", nil, invalid
	}
	allowed := make(map[string]bool)
	for _, flag := range required {
		allowed[flag] = true
	}
	options := make(map[string]string)
	for i := 2; i < len(args); i += 2 {
		if !allowed[args[i]] || options[args[i]] != "" || args[i+1] == "" {
			return "", nil, invalid
		}
		options[args[i]] = args[i+1]
	}
	if len(options) != len(required) {
		return "", nil, invalid
	}
	return action, options, nil
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	code := run(ctx, os.Args[1:], os.Stdout, os.Stderr)
	stop()
	os.Exit(code)
}
