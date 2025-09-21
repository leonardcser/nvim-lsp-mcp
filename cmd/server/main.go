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
			"- Uses Neovim RPC to query LSP diagnostics",
			"- Returns diagnostics grouped by file",
		)),
		// Structured input schema using Go struct (see mcp-go docs): https://mcp-go.dev/servers/tools
		mcp.WithInputSchema[tools.ReadLintsArgs](),
	)
	s.AddTool(toolReadLints, tools.ReadLintsHandler())
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
