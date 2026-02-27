//go:build linux && arm64

package disk

/*
#cgo pkg-config: ext2fs blkid uuid
*/
import "C"
