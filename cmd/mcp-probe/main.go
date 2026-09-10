// mcp-probe validates the SDK and stdio transport before the execution backend exists.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const toolName = "runweave_probe"

type probeInput struct {
	Nonce string `json:"nonce" jsonschema:"a caller supplied value to echo for transport verification"`
}

type probeOutput struct {
	Nonce string `json:"nonce"`
	Stage string `json:"stage"`
}

func newServer() *mcp.Server {
	server := mcp.NewServer(&mcp.Implementation{Name: "runweave-p0-probe", Version: "0.0.1-dev"}, nil)
	mcp.AddTool(server, &mcp.Tool{
		Name:        toolName,
		Description: "P0 transport probe only. Echoes a nonce; does not access files, nodes, or run commands.",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
	}, func(_ context.Context, _ *mcp.CallToolRequest, in probeInput) (*mcp.CallToolResult, probeOutput, error) {
		return nil, probeOutput{Nonce: in.Nonce, Stage: "p0-probe"}, nil
	})
	return server
}

// check uses a real subprocess and pipes, not an in-memory transport.
func check(ctx context.Context, executable string) error {
	client := mcp.NewClient(&mcp.Implementation{Name: "runweave-p0-check", Version: "0.0.1-dev"}, nil)
	cmd := exec.CommandContext(ctx, executable, "serve")
	cmd.Stderr = os.Stderr
	session, err := client.Connect(ctx, &mcp.CommandTransport{Command: cmd}, nil)
	if err != nil {
		return fmt.Errorf("connect: %w", err)
	}
	defer session.Close()
	listed, err := session.ListTools(ctx, nil)
	if err != nil {
		return fmt.Errorf("list tools: %w", err)
	}
	if len(listed.Tools) != 1 || listed.Tools[0].Name != toolName || listed.Tools[0].InputSchema == nil || listed.Tools[0].OutputSchema == nil {
		return fmt.Errorf("unexpected tools or missing schemas: %+v", listed.Tools)
	}
	const nonce = "runweave-协议-check"
	result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: toolName, Arguments: map[string]any{"nonce": nonce}})
	if err != nil {
		return fmt.Errorf("call tool: %w", err)
	}
	if result.IsError {
		return fmt.Errorf("probe returned a tool error")
	}
	raw, err := json.Marshal(result.StructuredContent)
	if err != nil {
		return err
	}
	var output probeOutput
	if err := json.Unmarshal(raw, &output); err != nil {
		return err
	}
	if output.Nonce != nonce || output.Stage != "p0-probe" {
		return fmt.Errorf("unexpected structured result: %s", raw)
	}
	if len(result.Content) == 0 {
		return fmt.Errorf("missing text compatibility content")
	}
	text, ok := result.Content[0].(*mcp.TextContent)
	if !ok || text.Text == "" {
		return fmt.Errorf("missing nonempty text compatibility content")
	}
	bad, badErr := session.CallTool(ctx, &mcp.CallToolParams{Name: toolName, Arguments: map[string]any{}})
	if badErr == nil && (bad == nil || !bad.IsError) {
		return fmt.Errorf("missing required nonce was accepted")
	}
	// A rejected input must not break the session.
	if _, err := session.ListTools(ctx, nil); err != nil {
		return fmt.Errorf("session unusable after invalid input: %w", err)
	}
	fmt.Printf("PASS protocol=%s: stdio discovery, schemas, UTF-8 structured result, invalid input rejection\n", session.InitializeResult().ProtocolVersion)
	return session.Close()
}

func run() error {
	if len(os.Args) != 2 {
		return fmt.Errorf("usage: mcp-probe serve|check")
	}
	switch os.Args[1] {
	case "serve":
		return newServer().Run(context.Background(), &mcp.StdioTransport{})
	case "check":
		executable, err := os.Executable()
		if err != nil {
			return err
		}
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		return check(ctx, executable)
	default:
		return fmt.Errorf("unknown command %q; usage: mcp-probe serve|check", os.Args[1])
	}
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
