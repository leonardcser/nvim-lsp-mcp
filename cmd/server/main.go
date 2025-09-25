package main

import (
	"os"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/leonardcser/nvim-lsp-mcp/internal/logger"
	tools "github.com/leonardcser/nvim-lsp-mcp/internal/tools"
)

func main() {
	if err := logger.InitFromEnv(); err != nil {
		panic(err)
	}
	defer logger.Close()

	logger.Infof("Starting Neovim LSP MCP server")

	s := server.NewMCPServer(
		"Neovim LSP MCP",
		"0.1.0",
		server.WithRecovery(),
		server.WithToolCapabilities(false),
	)
	logger.Infof("Created MCP server instance")

	toolReadLints := mcp.NewTool("read-lints",
		mcp.WithDescription(multiline(
			"Reads linter diagnostics from the current workspace via Neovim LSP",
			"\nFunctionality:",
			"- Uses Neovim to query LSP diagnostics",
			"- Returns diagnostics",
			"\nUsage notes:",
			"- IMPORTANT: ALWAYS run this tool immediately after creating or editing ANY file, without exception, passing the files you created/edited. This is mandatory for all file operations.",
			"- This tool checks for workspace lint warnings/errors and allows you to address them proactively.",
			"- If lint warnings/errors appear from files you did not create/edit, ask the user if they want you to fix those files at the end of the tasks you were given.",
			"- If you fixed a lint error and recheck with the read-lints tool and get the same error, tell the user to reload the file in their nvim client.",
			"- When the user asks to run lint checks or run lint tool, do not use this tool. However if the user asks to fix the lint errors, or fix the lint errors in a file, then use this tool.",
		)),
		// Structured input schema using Go struct (see mcp-go docs): https://mcp-go.dev/servers/tools
		mcp.WithInputSchema[tools.ReadLintsArgs](),
	)
	s.AddTool(toolReadLints, tools.ReadLintsHandler)
	logger.Infof("Registered read-lints tool")

	logger.Infof("Starting MCP server on stdio")
	if err := server.ServeStdio(s); err != nil {
		logger.Errorf("server error: %v", err)
	}
}

// multiline joins lines with newlines for tool descriptions.
func multiline(lines ...string) string { return strings.Join(lines, "\n") }

// placeholder to avoid unused import during boilerplate stage
var _ = os.Environ
