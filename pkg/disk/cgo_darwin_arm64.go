//go:build darwin && arm64

package disk

/*
#cgo pkg-config: ext2fs blkid uuid com_err
*/
import "C"
