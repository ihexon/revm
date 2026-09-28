//go:build linux && (arm64 || amd64)

package libkrun

/*
#cgo CFLAGS: -I ../../include
#cgo LDFLAGS: -L/tmp/.deps/libkrun/lib64 -lkrun -lkrun_init
*/
import "C"
