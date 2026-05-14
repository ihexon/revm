//go:build darwin && arm64

package libkrun

/*
#cgo LDFLAGS: -L /tmp/.deps/libkrun/lib/ -L/tmp/.deps/libkrunfw/lib -lkrun -lkrunfw
*/
import "C"
