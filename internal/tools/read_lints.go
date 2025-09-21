package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/leonardcser/nvim-lsp-mcp/internal/logger"
	"github.com/leonardcser/nvim-lsp-mcp/internal/nvim"
)

// ReadLintsArgs defines the structured input schema for the read-lints tool.
// Only an existing Neovim session is used; NVIM_LISTEN_ADDRESS must be set.
type ReadLintsArgs struct {
	Workspace string `json:"workspace" jsonschema_description:"Absolute workspace path" jsonschema:"required"`
}

// ReadLintsHandler returns the MCP tool handler for the "read-lints" tool.
func ReadLintsHandler() func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var args ReadLintsArgs
		raw, ok := req.Params.Arguments.([]byte)
		if !ok {
			// Fallback: try to marshal then unmarshal if it's a map[string]any
			b, merr := json.Marshal(req.Params.Arguments)
			if merr != nil {
				return nil, fmt.Errorf("invalid parameters: %v", merr)
			}
			raw = b
		}
		if err := json.Unmarshal(raw, &args); err != nil {
			return nil, fmt.Errorf("invalid parameters: %w", err)
		}
		if strings.TrimSpace(args.Workspace) == "" {
			return nil, errors.New("workspace is required")
		}

		cli, err := nvim.ConnectFromEnv(ctx)
		if err != nil {
			// Fallback to auto-discovery: find a Neovim whose cwd matches workspace
			cli, err = nvim.DiscoverAndConnectByCwd(ctx, args.Workspace)
			if err != nil {
				return nil, fmt.Errorf("failed to attach to Neovim: %w", err)
			}
		}
		defer cli.Close()

		// Validate that the Neovim session cwd matches the requested workspace
		cwd, err := nvim.GetCwd(ctx, cli)
		if err != nil {
			return nil, fmt.Errorf("failed to read Neovim cwd: %w", err)
		}
		if cwd != args.Workspace {
			return nil, fmt.Errorf("nvim cwd mismatch: expected %s, got %s", args.Workspace, cwd)
		}

		out, err := nvim.CollectDiagnosticsJSON(ctx, cli)
		if err != nil {
			return nil, fmt.Errorf("failed to collect diagnostics: %w", err)
		}
		if out == "" {
			logger.Warnf("no diagnostics returned from Neovim")
		}

		// Return JSON as text content
		return mcp.NewToolResultText(out), nil
	}
}
