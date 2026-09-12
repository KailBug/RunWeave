// mcp-contract-probe validates six business tool schemas using synthetic data only.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"runweave/internal/contractprobe"
	"runweave/internal/mcpcontract"
)

func run() error {
	if len(os.Args) < 2 {
		return fmt.Errorf("usage: mcp-contract-probe serve [--audit path]|check|schemas")
	}
	switch os.Args[1] {
	case "serve":
		fs := flag.NewFlagSet("serve", flag.ContinueOnError)
		path := fs.String("audit", "", "write fixture protocol audit JSONL")
		if err := fs.Parse(os.Args[2:]); err != nil {
			return err
		}
		var audit *os.File
		if *path != "" {
			var err error
			audit, err = os.OpenFile(*path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
			if err != nil {
				return err
			}
			defer audit.Close()
			return contractprobe.NewServer(audit).Run(context.Background(), &mcp.StdioTransport{})
		}
		return contractprobe.NewServer(nil).Run(context.Background(), &mcp.StdioTransport{})
	case "schemas":
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(mcpcontract.Tools())
	case "check":
		exe, err := os.Executable()
		if err != nil {
			return err
		}
		ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, exe, "serve")
		cmd.Stderr = os.Stderr
		client := mcp.NewClient(&mcp.Implementation{Name: "runweave-contract-check", Version: "0.0.1-dev"}, nil)
		session, err := client.Connect(ctx, &mcp.CommandTransport{Command: cmd}, nil)
		if err != nil {
			return err
		}
		defer session.Close()
		if err = contractprobe.Verify(ctx, session); err != nil {
			return err
		}
		fmt.Printf("PASS protocol=%s: six schemas, three operations, idempotency/conflict, approval, cancel intent/confirmation, bounded artifact, strict errors and text/structured equality\n", session.InitializeResult().ProtocolVersion)
		return session.Close()
	}
	return fmt.Errorf("unknown command")
}
func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
