package tools

import (
	"context"
	"encoding/json"
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
				return mcp.NewToolResultErrorFromErr("invalid parameters", merr), nil
			}
			raw = b
		}
		if err := json.Unmarshal(raw, &args); err != nil {
			return mcp.NewToolResultErrorFromErr("invalid parameters", err), nil
		}
		if strings.TrimSpace(args.Workspace) == "" {
			return mcp.NewToolResultError("workspace is required"), nil
		}

		cli, err := nvim.ConnectFromEnv(ctx)
		if err != nil {
			// Fallback to auto-discovery: find a Neovim whose cwd matches workspace
			cli, err = nvim.DiscoverAndConnectByCwd(ctx, args.Workspace)
			if err != nil {
				return mcp.NewToolResultErrorFromErr("failed to attach to Neovim", err), nil
			}
		}
		defer cli.Close()

		// Validate that the Neovim session cwd matches the requested workspace
		cwd, err := nvim.GetCwd(ctx, cli)
		if err != nil {
			return mcp.NewToolResultErrorFromErr("failed to read Neovim cwd", err), nil
		}
		if cwd != args.Workspace {
			return mcp.NewToolResultErrorf("nvim cwd mismatch: expected %s, got %s", args.Workspace, cwd), nil
		}

		out, err := nvim.CollectDiagnosticsJSON(ctx, cli)
		if err != nil {
			return mcp.NewToolResultErrorFromErr("failed to collect diagnostics", err), nil
		}
		if out == "" {
			logger.Warnf("no diagnostics returned from Neovim")
		}

		// Return JSON as text content
		return mcp.NewToolResultText(out), nil
	}
}
