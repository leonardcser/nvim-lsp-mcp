package nvim

import (
	"context"
)

// GetCwd returns the Neovim process current working directory.
func GetCwd(ctx context.Context, c *Client) (string, error) {
	cwdCh := make(chan string, 1)
	errCh := make(chan error, 1)

	go func() {
		var cwd string
		if err := c.NV.Eval("getcwd()", &cwd); err != nil {
			errCh <- err
			return
		}
		cwdCh <- cwd
	}()

	select {
	case cwd := <-cwdCh:
		return cwd, nil
	case err := <-errCh:
		return "", err
	case <-ctx.Done():
		return "", ctx.Err()
	}
}
