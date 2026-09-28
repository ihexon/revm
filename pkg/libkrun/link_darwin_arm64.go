//go:build darwin && arm64

package libkrun

/*
#cgo CFLAGS: -I ../../include
#cgo LDFLAGS: -L/tmp/.deps/libkrun/lib -lkrun -lkrun_init
#cgo LDFLAGS: -framework Hypervisor -framework Metal -framework Foundation -framework QuartzCore
#cgo LDFLAGS: -framework CoreGraphics -framework IOSurface -framework IOKit -framework AppKit
#cgo LDFLAGS: -lc++ -lobjc -liconv
*/
import "C"
