//go:build linux && (arm64 || amd64)

package system

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

type Info struct {
	ProductName         string
	ProductVersion      string
	ProductBuildVersion string
}

func GetOSVersion() (*Info, error) {
	f, err := os.Open("/etc/os-release")
	if err != nil {
		return nil, fmt.Errorf("failed to open /etc/os-release: %w", err)
	}
	defer f.Close()

	info := &Info{}
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		value = strings.Trim(value, "\"")
		switch key {
		case "NAME":
			info.ProductName = value
		case "VERSION_ID":
			info.ProductVersion = value
		case "VERSION":
			info.ProductBuildVersion = value
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("failed to read /etc/os-release: %w", err)
	}

	return info, nil
}
