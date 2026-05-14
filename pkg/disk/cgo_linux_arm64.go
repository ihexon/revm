//go:build linux && arm64

package disk

/*
#cgo LDFLAGS: /usr/lib/aarch64-linux-gnu/libext2fs.a /usr/lib/aarch64-linux-gnu/libcom_err.a /usr/lib/aarch64-linux-gnu/libblkid.a /usr/lib/aarch64-linux-gnu/libuuid.a
*/
import "C"
