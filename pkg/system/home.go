package system

import (
	"fmt"
	"os"
	"path/filepath"
)

// RevmHome is the single root for revm-managed runtime and build data.
func RevmHome() (string, error) {
	root := os.Getenv("REVM_HOME")
	if root == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		root = filepath.Join(home, ".revm")
	}
	if !filepath.IsAbs(root) {
		return "", fmt.Errorf("REVM_HOME must be an absolute path: %s", root)
	}
	root = filepath.Clean(root)
	resolved, _ := filepath.EvalSymlinks(root)
	if root == string(filepath.Separator) || resolved == string(filepath.Separator) {
		return "", fmt.Errorf("REVM_HOME must be a dedicated directory, not the filesystem root")
	}
	return root, nil
}
