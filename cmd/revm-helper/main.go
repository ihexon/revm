package main

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

// Linux wrapper that exec's .revm via bundled ld-linux
// to avoid system library dependencies.

func main() {
	exe, err := os.Executable()
	if err != nil {
		fmt.Fprintf(os.Stderr, "revm: cannot resolve own path: %v\n", err)
		os.Exit(1)
	}

	selfDir := filepath.Dir(exe)
	ldLinux := filepath.Join(selfDir, "..", "helper", "ld-linux-aarch64.so.1")
	libDir := filepath.Join(selfDir, "..", "lib")
	revm := filepath.Join(selfDir, ".revm")

	args := append([]string{"ld-linux-aarch64.so.1", "--library-path", libDir, revm}, os.Args[1:]...)

	err = syscall.Exec(ldLinux, args, os.Environ())
	fmt.Fprintf(os.Stderr, "revm: exec failed: %v\n", err)
	os.Exit(1)
}
