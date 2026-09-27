package mcp

import (
	"context"
	"errors"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// RunStdio starts the public MCP server over the process's standard streams.
// The SDK alone writes protocol frames to stdout; all command diagnostics stay
// in cmd/mindrail on stderr.
func RunStdio(ctx context.Context, root string) error {
	server, err := New(ctx, root)
	if err != nil {
		return err
	}
	runErr := server.SDK().Run(ctx, &sdk.StdioTransport{})
	shutdownErr := server.Close(context.Background())
	return errors.Join(runErr, shutdownErr)
}
