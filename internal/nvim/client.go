package nvim

import (
	"context"
	"errors"
	"os"

	nv "github.com/neovim/go-client/nvim"
)

// Client wraps a Neovim RPC client.
type Client struct {
	NV *nv.Nvim
}

// ConnectFromEnv attaches to an existing Neovim via NVIM_LISTEN_ADDRESS only.
func ConnectFromEnv(ctx context.Context) (*Client, error) {
	addr := os.Getenv("NVIM_LISTEN_ADDRESS")
	if addr == "" {
		return nil, errors.New("NVIM_LISTEN_ADDRESS is not set")
	}
	n, err := nv.Dial(addr)
	if err != nil {
		return nil, err
	}
	return &Client{NV: n}, nil
}

// Close closes the underlying Neovim client.
func (c *Client) Close() {
	if c != nil && c.NV != nil {
		_ = c.NV.Close()
	}
}
