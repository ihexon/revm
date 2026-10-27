//go:build (darwin && arm64) || (linux && (arm64 || amd64))

package filesystem

import (
	"fmt"
	"linuxvm/pkg/define"
	"path"
	"path/filepath"
	"strings"
)

// ParseMounts accepts HOST:GUEST[,ro]. Targets must be unique absolute guest paths.
func ParseMounts(values []string) ([]define.VirtioFSSpec, error) {
	mounts := make([]define.VirtioFSSpec, 0, len(values))
	targets := make(map[string]bool)
	for i, value := range values {
		source, guest, ok := strings.Cut(value, ":")
		target, option, hasOption := strings.Cut(guest, ",")
		if !ok || source == "" || !path.IsAbs(target) || strings.ContainsAny(value, "\x00") || strings.Contains(guest, ":") || (hasOption && option != "ro") {
			return nil, fmt.Errorf("invalid mount %q: expected HOST:/GUEST[,ro]", value)
		}
		target = path.Clean(target)
		if targets[target] {
			return nil, fmt.Errorf("duplicate mount target %q", target)
		}
		targets[target] = true
		source, err := filepath.Abs(source)
		if err != nil {
			return nil, fmt.Errorf("resolve mount source: %w", err)
		}
		mounts = append(mounts, define.VirtioFSSpec{Source: source, Target: target, Tag: fmt.Sprintf("mnt-%d", i), ReadOnly: hasOption})
	}
	return mounts, nil
}
