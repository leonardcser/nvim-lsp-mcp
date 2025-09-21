package nvim

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/leonardcser/nvim-lsp-mcp/internal/logger"
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

// CollectDiagnosticsJSON collects diagnostics for all listed buffers as JSON, using the injected Lua function.
func CollectDiagnosticsJSON(ctx context.Context, c *Client) (string, error) {
	// Minimal context
	if cwd, err := GetCwd(ctx, c); err == nil {
		logger.Infof("nvim: cwd=%s", cwd)
	}

	// Use RPC for buffer list and buffer metadata
	var bufs []int
	if err := c.NV.Call("nvim_list_bufs", &bufs); err != nil {
		return "", err
	}
	logger.Infof("nvim: buffers_total=%d", len(bufs))
	if len(bufs) == 0 {
		logger.Warnf("nvim: no buffers returned by nvim_list_bufs")
	}

	type fileDiags struct {
		File        string           `json:"file"`
		Diagnostics []map[string]any `json:"diagnostics"`
	}
	results := make([]fileDiags, 0, len(bufs))

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

		// Fetch diagnostics directly from vim.diagnostic.get
		items, err := fetchBufferDiagnostics(c, bnr)
		if err != nil {
			logger.Errorf("nvim: diagnostic.get(%d) error: %v", bnr, err)
			continue
		}
		if len(items) == 0 {
			continue
		}
		results = append(results, fileDiags{File: name, Diagnostics: items})
	}

	b, err := json.Marshal(results)
	if err != nil {
		return "", err
	}
	logger.Infof("nvim: files_with_diagnostics=%d", len(results))
	return string(b), nil
}
