package config

import (
	"os"
	"path/filepath"
)

// BinPath is where pm is installed on this machine and what hooks, agents and the service unit run:
// ${PM_BIN_DIR:-$HOME/.local/bin}/pm (the pm-go page, Distribution). install.sh and pm init put the binary there.
func BinPath() (string, error) {
	if dir := os.Getenv("PM_BIN_DIR"); dir != "" {
		return filepath.Join(dir, "pm"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".local/bin/pm"), nil
}
