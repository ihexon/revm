// Package librevm provides a Go library for declaring, building, and running
// Linux microVMs using libkrun.
//
// It supports three usage patterns:
//
//  1. TOML/JSON configuration file:
//
//     cfg, _ := librevm.LoadFile("vm.toml")
//     vm, _  := librevm.New(ctx, cfg)
//     defer vm.Close()
//     vm.Run(ctx)
//
//  2. Struct literal:
//
//     vm, _ := librevm.New(ctx, &librevm.Config{
//     CPUs:     4,
//     MemoryMB: 2048,
//     Command:  []string{"/bin/sh", "-c", "echo hello"},
//     })
//     defer vm.Close()
//     vm.Run(ctx)
//
//  3. Chain (fluent) API:
//
//     cfg := librevm.DefaultConfig().
//     WithName("dev").
//     WithCPUs(4).
//     WithMemory(4096).
//     WithProxy(true)
//     vm, _ := librevm.New(ctx, cfg)
//     defer vm.Close()
//     vm.Run(ctx)
//
// Mode inference: if Config.Command is non-empty the VM runs in rootfs mode;
// otherwise it runs in container mode.
//
// Close must always be called to release resources (flock, workspace), even if
// Start was never called.
package librevm
