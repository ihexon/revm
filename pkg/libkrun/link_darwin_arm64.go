//go:build darwin && arm64

package libkrun

/*
#cgo LDFLAGS: /tmp/.deps/libkrun/lib/libkrun.a
#cgo LDFLAGS: /tmp/.deps/libkrun/lib/libvirglrenderer.a /tmp/.deps/libkrun/lib/libepoxy.a
#cgo LDFLAGS: /tmp/.deps/libkrun/lib/libMoltenVK.a /tmp/.deps/libkrun/lib/libSPIRVCross.a /tmp/.deps/libkrun/lib/libSPIRVTools.a
#cgo LDFLAGS: -L/tmp/.deps/libkrunfw/lib -lkrunfw
#cgo LDFLAGS: -framework Hypervisor -framework Metal -framework Foundation -framework QuartzCore
#cgo LDFLAGS: -framework CoreGraphics -framework IOSurface -framework IOKit -framework AppKit
#cgo LDFLAGS: -lc++ -lobjc -liconv
*/
import "C"
