//go:build (darwin && arm64) || (linux && (arm64 || amd64))

package librevm

import (
	"fmt"
	"path/filepath"
)

func workspacePathForSession(name string) string {
	return fmt.Sprintf("/tmp/.revm-%s", name)
}

func ignitionSockPath(workspace string) string {
	return filepath.Clean(filepath.Join(workspace, "socks", "ign.sock"))
}
