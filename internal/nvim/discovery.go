package nvim

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"

	nv "github.com/neovim/go-client/nvim"

	"github.com/leonardcser/nvim-lsp-mcp/internal/logger"
)

// discoverSocketCandidates returns possible Neovim socket paths without using nvr.
func discoverSocketCandidates() []string {
	candidates := make([]string, 0, 8)

	// Check NVIM_LISTEN_ADDRESS first if set
	if addr := os.Getenv("NVIM_LISTEN_ADDRESS"); addr != "" {
		candidates = append(candidates, addr)
	}

	// macOS TMPDIR (and general TMPDIR)
	tmp := os.Getenv("TMPDIR")
	if tmp == "" {
		tmp = os.TempDir()
	}
	if tmp != "" {
		// Newer Neovim uses per-user dirs like nvim.<user>/<rand>/nvim.<pid>.0
		if matches, _ := filepath.Glob(filepath.Join(tmp, "nvim.*", "*", "nvim.*.0")); len(matches) > 0 {
			candidates = append(candidates, matches...)
		}
		// Older/simple pattern fallback
		if matches, _ := filepath.Glob(filepath.Join(tmp, "nvim*", "0")); len(matches) > 0 {
			candidates = append(candidates, matches...)
		}
	}

	// Also check /tmp explicitly
	if matches, _ := filepath.Glob(filepath.Join("/tmp", "nvim.*", "*", "nvim.*.0")); len(matches) > 0 {
		candidates = append(candidates, matches...)
	}
	if matches, _ := filepath.Glob(filepath.Join("/tmp", "nvim*", "0")); len(matches) > 0 {
		candidates = append(candidates, matches...)
	}

	// XDG_RUNTIME_DIR (Linux style paths can exist on mac via some setups)
	if xdg := os.Getenv("XDG_RUNTIME_DIR"); xdg != "" {
		if matches, _ := filepath.Glob(filepath.Join(xdg, "nvim.*", "*")); len(matches) > 0 {
			candidates = append(candidates, matches...)
		}
	}

	// OS-specific broad scans
	switch runtime.GOOS {
	case "darwin":
		// Typical macOS temp roots (note nvim.<user>/<rand>/nvim.<pid>.0)
		if matches, _ := filepath.Glob("/var/folders/*/*/T/nvim.*/*/nvim.*.0"); len(matches) > 0 {
			candidates = append(candidates, matches...)
		}
		if matches, _ := filepath.Glob("/private/var/folders/*/*/T/nvim.*/*/nvim.*.0"); len(matches) > 0 {
			candidates = append(candidates, matches...)
		}
	case "linux":
		if matches, _ := filepath.Glob("/run/user/*/nvim.*/*"); len(matches) > 0 {
			candidates = append(candidates, matches...)
		}
	}

	if len(candidates) == 0 {
		logger.Warnf("nvim discovery: no socket candidates found (TMPDIR=%s, XDG_RUNTIME_DIR=%s)", tmp, os.Getenv("XDG_RUNTIME_DIR"))
	}

	return candidates
}

// DiscoverAndConnectByCwd tries all discovered sockets and returns the client whose cwd matches workspace.
func DiscoverAndConnectByCwd(ctx context.Context, workspace string) (*Client, error) {
	for _, addr := range discoverSocketCandidates() {
		logger.Infof("nvim discovery: trying %s", addr)
		n, err := nv.Dial(addr)
		if err != nil {
			logger.Warnf("nvim discovery: dial failed for %s: %v", addr, err)
			continue
		}
		cli := &Client{NV: n}
		cwd, err := GetCwd(ctx, cli)
		if err != nil {
			logger.Warnf("nvim discovery: failed to getcwd for %s: %v", addr, err)
			_ = n.Close()
			continue
		}
		if cwd == workspace {
			logger.Infof("nvim discovery: matched workspace cwd=%s at %s", cwd, addr)
			return cli, nil
		}
		_ = n.Close()
	}
	return nil, errors.New("no Neovim sessions found matching workspace cwd")
}
