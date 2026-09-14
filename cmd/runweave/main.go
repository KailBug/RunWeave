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
		}{version, "p1-foundation"})
	}
	if len(args) != 4 || args[2] != "--config" || args[3] == "" ||
		!((args[0] == "config" && args[1] == "check") || (args[0] == "state" && args[1] == "init")) {
		return fail(errors.New("usage: runweave version | config check --config FILE | state init --config FILE; daemon and mcp serving are not implemented yet"))
	}
	c, err := config.Load(args[3])
	if err != nil {
		return fail(err)
	}
	levels := map[string]slog.Level{"debug": slog.LevelDebug, "info": slog.LevelInfo, "warn": slog.LevelWarn, "error": slog.LevelError}
	level.Set(levels[c.LogLevel])
	if args[0] == "config" {
		logger.Info("configuration validated", "role", c.Role)
		return write(struct {
			Valid   bool   `json:"valid"`
			Version int    `json:"config_version"`
			Role    string `json:"role"`
		}{true, c.Version, c.Role})
	}
	if c.Role == "mcp" {
		return fail(errors.New("mcp bridge has no local daemon state"))
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
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

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	code := run(ctx, os.Args[1:], os.Stdout, os.Stderr)
	stop()
	os.Exit(code)
}
