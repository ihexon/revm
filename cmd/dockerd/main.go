//go:build (darwin && arm64) || (linux && (arm64 || amd64))

package main

import "linuxvm/internal/revmcmd"

func main() {
	revmcmd.Run(revmcmd.DockerdCommand())
}
