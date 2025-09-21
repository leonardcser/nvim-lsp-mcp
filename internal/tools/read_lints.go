package tools

import (
	"context"
	"encoding/json"
	"fmt"
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

// Diagnostic represents a single diagnostic item
type Diagnostic struct {
	Severity int    `json:"severity"`
	Message  string `json:"message"`
	Source   string `json:"source"`
	Code     string `json:"code"`
	Lnum     int    `json:"lnum"`
	Col      int    `json:"col"`
	EndLnum  int    `json:"end_lnum"`
	EndCol   int    `json:"end_col"`
}

// FileDiagnostics represents diagnostics for a single file
type FileDiagnostics struct {
	File        string       `json:"file"`
	Diagnostics []Diagnostic `json:"diagnostics"`
}

// formatDiagnosticsAsText formats the JSON diagnostics into compiler-style text output
func formatDiagnosticsAsText(jsonOutput string) (string, error) {
	if strings.TrimSpace(jsonOutput) == "" {
		return "", nil
	}

	var fileDiags []FileDiagnostics
	if err := json.Unmarshal([]byte(jsonOutput), &fileDiags); err != nil {
		return "", fmt.Errorf("failed to parse diagnostics JSON: %w", err)
	}

	if len(fileDiags) == 0 {
		return "", nil
	}

	var output strings.Builder
	for i, fileDiag := range fileDiags {
		if i > 0 {
			output.WriteString("\n")
		}
		
		for _, diag := range fileDiag.Diagnostics {
			// Format: filename:line:column: severity: message
			var severity string
			switch diag.Severity {
			case 2:
				severity = "warning"
			case 3:
				severity = "info"
			case 4:
				severity = "hint"
			default:
				severity = "error"
			}
			
			output.WriteString(fmt.Sprintf("%s:%d:%d: %s: %s\n",
				fileDiag.File,
				diag.Lnum+1, // Convert 0-based to 1-based line numbers
				diag.Col+1,  // Convert 0-based to 1-based column numbers
				severity,
				diag.Message,
			))
		}
	}

	return output.String(), nil
}

// ReadLintsHandler returns the MCP tool handler for the "read-lints" tool.
// This uses the recommended structured handler pattern from mcp-go.
func ReadLintsHandler(ctx context.Context, req mcp.CallToolRequest, args ReadLintsArgs) (*mcp.CallToolResult, error) {
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

	jsonOutput, err := nvim.CollectDiagnosticsJSON(ctx, cli, args.Files)
	if err != nil {
		return mcp.NewToolResultErrorFromErr("failed to collect diagnostics", err), nil
	}
	if jsonOutput == "" {
		logger.Warnf("no diagnostics returned from Neovim")
		return mcp.NewToolResultText(""), nil
	}

	// Format diagnostics as compiler-style text output
	formattedOutput, err := formatDiagnosticsAsText(jsonOutput)
	if err != nil {
		return mcp.NewToolResultErrorFromErr("failed to format diagnostics", err), nil
	}

	return mcp.NewToolResultText(formattedOutput), nil
}
