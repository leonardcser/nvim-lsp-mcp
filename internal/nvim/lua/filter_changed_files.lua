-- Filter changed files by LSP supported filetypes
-- Args: workspace (string), maxFiles (int)
-- Returns: JSON {filtered: [paths], origCount: int, filteredCount: int}

local workspace, maxFiles = ...

-- Get changed files via git diff
local origCwd = vim.fn.getcwd()
vim.fn.chdir(workspace)
local gitOut = vim.fn.system("git diff --name-only HEAD")
vim.fn.chdir(origCwd)

local relFiles = vim.fn.split(vim.trim(gitOut), "\n")
local origCount = 0
for _, rel in ipairs(relFiles) do
	if rel ~= "" then
		origCount = origCount + 1
	end
end

-- Tables for results and caching
local absFiles = {}
local detectedFTs = {}
local skipExts = { "conf" }

-- Local function to detect filetype for a path
local function detectFiletype(path)
	local ext = vim.fn.fnamemodify(path, ":e")
	if detectedFTs[ext] ~= nil then
		return detectedFTs[ext]
	end

	local ft = vim.filetype.match({ filename = path })
	if not ft then
		-- Fallback: create temp buffer to detect filetype
		local tmpBuf = vim.api.nvim_create_buf(false, true)
		local lines = vim.fn.readfile(path)
		vim.api.nvim_buf_set_lines(tmpBuf, 0, -1, false, lines)
		ft = vim.filetype.match({ buf = tmpBuf }) or ""
		pcall(vim.api.nvim_buf_delete, tmpBuf, { force = true })
	end

	-- Cache unless extension is problematic
	if not vim.tbl_contains(skipExts, ext) then
		detectedFTs[ext] = ft
	end

	return ft
end

-- Process each relative file path
for _, rel in ipairs(relFiles) do
	if rel == "" then
		goto continue
	end

	local abs = vim.fs.joinpath(workspace, rel)
	abs = vim.fn.fnamemodify(abs, ":p")

	if vim.fn.filereadable(abs) == 0 then
		goto continue
	end

	local ft = detectFiletype(abs)
	table.insert(absFiles, { path = abs, ft = ft })

	::continue::
end

-- Get supported filetypes from all active LSP clients (global)
local clients = vim.lsp.get_clients()
local supportedFTs = {}
for _, cl in ipairs(clients) do
	if cl.config then
		local filetypes = cl.config.filetypes or {}
		for _, sft in ipairs(filetypes) do
			supportedFTs[sft] = true
		end
	end
end
local supportedCount = 0
for _ in pairs(supportedFTs) do
	supportedCount = supportedCount + 1
end

-- Filter to relevant files, capping at maxFiles
local filtered = {}
local matched = 0
local unsupported = 0
for _, f in ipairs(absFiles) do
	if not supportedFTs[f.ft] then
		unsupported = unsupported + 1
	elseif #filtered < maxFiles then
		table.insert(filtered, f.path)
		matched = matched + 1
	else
		-- Cap exceeded
	end
end

-- Return JSON result
return vim.json.encode({
	filtered = filtered,
	origCount = origCount,
	filteredCount = #filtered,
})
