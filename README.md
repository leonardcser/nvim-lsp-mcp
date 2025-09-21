# Neovim LSP MCP

A Model Context Protocol (MCP) server that provides access to linter diagnostics
from a workspace via Neovim's LSP.

## Tools

### `read-lints`

Read diagnostics from the current workspace via Neovim LSP.

This tool attaches to an existing Neovim session. It prefers
`NVIM_LISTEN_ADDRESS` if set; otherwise it will auto-discover running Neovim
sockets and attach to the one whose `getcwd()` matches the requested
`workspace`. It does not spawn new Neovim instances.

**Parameters:**

- `workspace` (string, required): Absolute path to the workspace. The Neovim
  session's cwd must equal this path.

**Behavior:**

- Connects to the Neovim session specified by `NVIM_LISTEN_ADDRESS`, or
  auto-discovers an appropriate session by cwd match.
- Validates that `getcwd()` in Neovim equals `workspace`. If not, returns an
  error.
- Collects diagnostics for loaded buffers using `vim.diagnostic.get(bufnr)` and
  returns them grouped by file as JSON text.

## Installation

```bash
go build -o nvim-lsp-mcp ./cmd/server
```

## Usage

Run the server:

```bash
./nvim-lsp-mcp
```

Ensure you have a Neovim instance running with LSP diagnostics in the same
workspace, and either:

- Export `NVIM_LISTEN_ADDRESS` from that Neovim, or
- Rely on auto-discovery (the server searches typical socket locations and
  selects the session whose `cwd` equals `workspace`).

## MCP Configuration

To use this server with Claude Desktop or other MCP clients, add it to your MCP
configuration file.

```json
{
  "mcpServers": {
    "nvim-lsp": {
      "command": "/path/to/nvim-lsp-mcp",
      "args": []
    }
  }
}
```

Replace `/path/to/nvim-lsp-mcp` with the actual path to your compiled binary.

### Other MCP Clients

For other MCP clients, configure the server with:

- **Command**: Path to the `nvim-lsp-mcp` binary
- **Args**: Empty array `[]`
- **Transport**: STDIO

## Custom Cursor Rules

You can create custom rules in Cursor to make the agent proactively use the
`read-lints` tool. Create a `.cursor/rules/lint.mdc` file in your project root
with rules like this:

```markdown
---
description: Lint workspace files
globs:
alwaysApply: true
---

- IMPORTANT: After creating or editing ANY file, run the read-lints tool to
  check for workspace lint errors and address them proactively.
- If lint warnings/errors appear from files you did not create/edit, ask the
  user if they want you to fix those files at the end of the tasks you were
  given.
- If you fixed a lint error and recheck with the read-lints tool and get the
  same error, tell the user to reload the file in their nvim client.
- When the user asks to run lint checks or run lint tool, do not use the
  read-lints tool. However if the user asks to fix the lint errors, or fix the
  lint errors in a file, then use the read-lints tool.
```

This rule ensures the agent automatically checks for lint errors after making
changes and proactively addresses them.

## Configuration

- Set log path with `NVIM_LSP_MCP_LOG` (defaults near the executable)
- Logging is written to a single file; rotate externally if needed

## Requirements

- Go 1.25.1+
- Neovim running with active LSP diagnostics in the target workspace
- Socket accessibility via `NVIM_LISTEN_ADDRESS` or auto-discovery

## Troubleshooting

- "failed to attach to Neovim": ensure a Neovim instance is running and either
  export `NVIM_LISTEN_ADDRESS` or open Neovim in the same `workspace` so
  auto-discovery can match by cwd.
- "nvim cwd mismatch": open Neovim with `:cd /absolute/path/to/project` (or
  start Neovim from that directory) to align with `workspace`.
- Empty results: diagnostics are only returned for buffers with diagnostics;
  ensure your LSP is configured and diagnostics exist.

## License

MIT
