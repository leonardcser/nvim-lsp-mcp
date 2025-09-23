package nvim

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/leonardcser/nvim-lsp-mcp/internal/logger"
)

const (
	// MaxFilesToReload is the maximum number of files to reload for diagnostics
	// If the number of files exceeds this limit, reloading is disabled
	MaxFilesToReload = 100
)

// fetchBufferDiagnostics tries to fetch diagnostics for a given buffer.
// It first asks Lua for the count, then attempts to decode the table directly.
// If decoding yields fewer items than Lua reports, it falls back to JSON encoding in Lua.
func fetchBufferDiagnostics(c *Client, bufnr int) ([]map[string]any, error) {
	// Encode in Lua and unmarshal in Go for stability
	var jsonStr string
	codeJSON := fmt.Sprintf("return vim.json.encode(vim.diagnostic.get(%d))", bufnr)
	if err := c.NV.ExecLua(codeJSON, &jsonStr); err != nil {
		return nil, err
	}
	if jsonStr == "" || jsonStr == "null" {
		return nil, nil
	}
	var items []map[string]any
	if err := json.Unmarshal([]byte(jsonStr), &items); err != nil {
		return nil, err
	}
	return items, nil
}

// refreshWorkspaceDiagnostics forces a refresh of workspace diagnostics for specific files
func refreshWorkspaceDiagnostics(c *Client, files []string, workspace string) error {
	var filesToProcess []string

	if len(files) == 0 {
		// If no files specified, use git diff to get changed files (staged and unstaged)
		cmd := exec.Command("git", "diff", "--name-only", "HEAD")
		cmd.Dir = workspace
		output, err := cmd.Output()
		if err != nil {
			return fmt.Errorf("failed to run git diff --name-only: %w", err)
		}

		gitFiles := strings.SplitSeq(strings.TrimSpace(string(output)), "\n")
		for file := range gitFiles {
			if file != "" {
				fullPath := filepath.Join(workspace, file)
				filesToProcess = append(filesToProcess, fullPath)
			}
		}
	} else {
		filesToProcess = files
	}

	// Check if we have too many files to reload
	if len(filesToProcess) > MaxFilesToReload {
		logger.Warnf("nvim: too many files to reload (%d > %d), skipping reload", len(filesToProcess), MaxFilesToReload)
		return nil
	}

	// Refresh diagnostics for files by sending textDocument/didSave notifications
	// Use ExecLua with args to properly pass the file list to Lua
	code := `
		local files = ...
		for _, filepath in ipairs(files) do
			local bufnr = vim.fn.bufnr(filepath, true)

			if not vim.api.nvim_buf_is_loaded(bufnr) then
				-- Use nvim_buf_call to safely load the buffer
				vim.api.nvim_buf_call(bufnr, function()
					vim.cmd("silent! edit")
				end)
			else
				-- Buffer is already loaded, refresh it from disk
				vim.api.nvim_buf_call(bufnr, function()
					vim.cmd("silent! checktime")
				end)
			end

			-- Small delay to ensure the buffer is fully loaded/refreshed
			vim.schedule(function()
				-- Send LSP notifications after buffer is reloaded
				for _, client in ipairs(vim.lsp.get_clients({ bufnr = bufnr })) do
					if client:supports_method("textDocument/didSave") then
						client:notify("textDocument/didSave", {
							textDocument = { uri = vim.uri_from_fname(filepath) },
						})
					end
				end
			end)
		end
	`

	return c.NV.ExecLua(code, nil, filesToProcess)
}

// CollectDiagnosticsJSON collects diagnostics for all listed buffers as JSON, using the injected Lua function.
func CollectDiagnostics(ctx context.Context, c *Client, files []string) (string, error) {
	// Minimal context
	if cwd, err := GetCwd(ctx, c); err == nil {
		logger.Infof("nvim: cwd=%s", cwd)
	}

	// Get workspace directory
	workspace, err := GetCwd(ctx, c)
	if err != nil {
		return "", fmt.Errorf("failed to get workspace: %w", err)
	}

	// Validate file paths are within workspace
	if len(files) > 0 {
		validatedFiles := make([]string, 0, len(files))
		for _, file := range files {
			// Check if file is absolute and within workspace
			if !strings.HasPrefix(file, workspace) {
				logger.Warnf("nvim: file %s is outside workspace %s, skipping", file, workspace)
				continue
			}
			validatedFiles = append(validatedFiles, file)
		}
		files = validatedFiles
	}

	// Refresh workspace diagnostics before collecting
	if len(files) == 0 {
		logger.Infof("nvim: refreshing workspace diagnostics for changed files")
	} else {
		logger.Infof("nvim: refreshing workspace diagnostics for %d files", len(files))
	}
	if err := refreshWorkspaceDiagnostics(c, files, workspace); err != nil {
		logger.Warnf("nvim: failed to refresh workspace diagnostics: %v", err)
		// Continue anyway - diagnostics might still be available
	}

	// Give LSP servers a moment to process the refresh notifications
	logger.Infof("nvim: waiting for LSP to reload diagnostics...")
	time.Sleep(3 * time.Second)

	// Use RPC for buffer list and buffer metadata
	var bufs []int
	if err := c.NV.Call("nvim_list_bufs", &bufs); err != nil {
		return "", err
	}
	logger.Infof("nvim: buffers_total=%d", len(bufs))
	if len(bufs) == 0 {
		logger.Warnf("nvim: no buffers returned by nvim_list_bufs")
	}

	var lines []string

	for _, bnr := range bufs {
		var valid bool
		if err := c.NV.Call("nvim_buf_is_valid", &valid, bnr); err != nil {
			logger.Errorf("nvim: nvim_buf_is_valid(%d) error: %v", bnr, err)
			continue
		}
		if !valid {
			continue
		}
		var name string
		if err := c.NV.Call("nvim_buf_get_name", &name, bnr); err != nil {
			logger.Errorf("nvim: nvim_buf_get_name(%d) error: %v", bnr, err)
			continue
		}
		if name == "" {
			// Skip unnamed buffers
			continue
		}

		// If specific files were requested, only include diagnostics for those files
		if len(files) > 0 {
			if !slices.Contains(files, name) {
				continue
			}
		}

		// Fetch diagnostics directly from vim.diagnostic.get
		items, err := fetchBufferDiagnostics(c, bnr)
		if err != nil {
			logger.Errorf("nvim: diagnostic.get(%d) error: %v", bnr, err)
			continue
		}
		if len(items) == 0 {
			continue
		}
		for _, item := range items {
			severityRaw, ok := item["severity"].(float64)
			if !ok {
				continue
			}
			severityInt := int(severityRaw)
			var severityStr string
			switch severityInt {
			case 1:
				severityStr = "error"
			case 2:
				severityStr = "warning"
			case 3:
				severityStr = "info"
			case 4:
				severityStr = "hint"
			default:
				severityStr = "unknown"
			}

			lnumRaw, ok := item["lnum"].(float64)
			if !ok {
				continue
			}
			line := int(lnumRaw) + 1

			colRaw, ok := item["col"].(float64)
			col := 1
			if ok {
				col = int(colRaw) + 1
			}

			msg, ok := item["message"].(string)
			if !ok || msg == "" {
				continue
			}

			source, _ := item["source"].(string)
			codeRaw := item["code"]
			var codeStr string
			if codeRaw != nil {
				codeStr = fmt.Sprintf("%v", codeRaw)
			}

			formatted := fmt.Sprintf("%s:%d:%d: %s: %s", name, line, col, strings.ToUpper(severityStr), msg)
			if source != "" {
				formatted += fmt.Sprintf(" (%s)", source)
			}
			if codeStr != "" {
				formatted += fmt.Sprintf(" [%s]", codeStr)
			}
			lines = append(lines, formatted)
		}
	}

	logger.Infof("nvim: diagnostics_total=%d", len(lines))
	return strings.Join(lines, "\n"), nil
}
