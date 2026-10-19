package system

import "os"

// GetTerminalType returns the TERM environment variable or a default
func GetTerminalType() string {
	termEnv := os.Getenv("TERM")
	if termEnv == "" {
		termEnv = "xterm-256color"
	}
	return termEnv
}
