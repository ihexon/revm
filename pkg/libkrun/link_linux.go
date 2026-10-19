//go:build linux && (arm64 || amd64)

package libkrun

/*
#cgo CFLAGS: -I ../../include
#cgo LDFLAGS: -lkrun -lkrun_init -Wl,-rpath,$ORIGIN/../lib
*/
import "C"
