//go:build linux && (arm64 || amd64)

package libkrun

/*
#cgo LDFLAGS: /tmp/.deps/libkrun/lib64/libkrun.a -L/tmp/.deps/libkrunfw/lib -lkrunfw
*/
import "C"
