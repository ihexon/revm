//go:build linux && amd64

package disk

/*
#cgo LDFLAGS: /usr/lib/x86_64-linux-gnu/libext2fs.a /usr/lib/x86_64-linux-gnu/libcom_err.a /usr/lib/x86_64-linux-gnu/libblkid.a /usr/lib/x86_64-linux-gnu/libuuid.a
*/
import "C"
