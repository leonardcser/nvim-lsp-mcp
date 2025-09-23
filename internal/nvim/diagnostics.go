package nvim

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
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

type luaFilterResult struct {
	Filtered      []string `json:"filtered"`
	OrigCount     int      `json:"origCount"`
	FilteredCount int      `json:"filteredCount"`
}

//go:embed lua/filter_changed_files.lua
var filterLua string

//go:embed lua/refresh_diagnostics.lua
var refreshLua string

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
func refreshWorkspaceDiagnostics(c *Client, files []string, workspace string, maxFiles int) error {
	var filesToProcess []string

	if len(files) > 0 {
		filesToProcess = files
		if len(filesToProcess) > maxFiles {
			filesToProcess = filesToProcess[:maxFiles]
			logger.Warnf("nvim: capped user-specified files to %d", maxFiles)
		}
	} else {
		// Lua-based filtering for changed files
		luaCode := filterLua
		var jsonStr string
		err := c.NV.ExecLua(luaCode, &jsonStr, workspace, maxFiles)
		if err != nil {
			logger.Errorf("nvim: Lua filtering failed: %v, skipping refresh", err)
			return nil
		}
		if jsonStr == "" || jsonStr == "null" {
			logger.Errorf("nvim: Lua filtering returned empty result, skipping refresh")
			return nil
		}
		var result luaFilterResult
		if err := json.Unmarshal([]byte(jsonStr), &result); err != nil {
			logger.Errorf("nvim: Invalid JSON from Lua filtering: %v, skipping refresh", err)
			return nil
		}
		filesToProcess = result.Filtered
		logger.Infof("nvim: Lua filtered %d changed files to %d relevant (max %d)", result.OrigCount, result.FilteredCount, maxFiles)
		if len(filesToProcess) > maxFiles {
			filesToProcess = filesToProcess[:maxFiles]
			logger.Warnf("nvim: Capped post-Lua files to %d", maxFiles)
		}
	}

	if len(filesToProcess) == 0 {
		return nil
	}

	// Refresh diagnostics for files by sending textDocument/didSave notifications
	// Use ExecLua with args to properly pass the file list to Lua
	code := refreshLua

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
	if err := refreshWorkspaceDiagnostics(c, files, workspace, MaxFilesToReload); err != nil {
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
