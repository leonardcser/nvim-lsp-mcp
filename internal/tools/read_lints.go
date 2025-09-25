package tools

import (
	"context"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/leonardcser/nvim-lsp-mcp/internal/logger"
	"github.com/leonardcser/nvim-lsp-mcp/internal/nvim"
)

// ReadLintsArgs defines the structured input schema for the read-lints tool.
// Only an existing Neovim session is used; NVIM_LISTEN_ADDRESS must be set.
type ReadLintsArgs struct {
	Workspace string   `json:"workspace" jsonschema_description:"Absolute workspace path" jsonschema:"required"`
	Files     []string `json:"files,omitempty" jsonschema_description:"List of absolute file paths to refresh diagnostics for, if empty, fallsback to refreshing changed files (staged and unstaged) via git diff."`
}

// ReadLintsHandler returns the MCP tool handler for the "read-lints" tool.
// This uses the recommended structured handler pattern from mcp-go.
func ReadLintsHandler(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var args ReadLintsArgs
	if err := req.BindArguments(&args); err != nil {
		return mcp.NewToolResultError(err.Error()), nil
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

	output, err := nvim.CollectDiagnostics(ctx, cli, args.Files)
	if err != nil {
		return mcp.NewToolResultErrorFromErr("failed to collect diagnostics", err), nil
	}
	if output == "" {
		logger.Warnf("no diagnostics returned from Neovim")
		return mcp.NewToolResultText(""), nil
	}

	return mcp.NewToolResultText(output), nil
}
