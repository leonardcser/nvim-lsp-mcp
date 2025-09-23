-- Refresh diagnostics for given files by loading/refreshing buffers and notifying LSP clients
-- Args: files (table of absolute file paths)
-- Returns: nil (side-effect only)

local files = ...

-- Local function to refresh a single buffer and notify LSP
local function refreshAndNotify(filepath, bufnr)
	-- Load or refresh the buffer from disk
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

-- Process each file
for _, filepath in ipairs(files) do
	local bufnr = vim.fn.bufnr(filepath, true)
	refreshAndNotify(filepath, bufnr)
end
