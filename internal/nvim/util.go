package nvim

import "context"

// GetCwd returns the Neovim process current working directory.
func GetCwd(ctx context.Context, c *Client) (string, error) {
	var cwd string
	if err := c.NV.Eval("getcwd()", &cwd); err != nil {
		return "", err
	}
	return cwd, nil
}
