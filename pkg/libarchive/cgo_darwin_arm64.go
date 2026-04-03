//go:build darwin && arm64

package libarchive_go

/*
#cgo CFLAGS: -I/opt/homebrew/opt/libarchive/include
#cgo LDFLAGS: /opt/homebrew/opt/libarchive/lib/libarchive.a
#cgo LDFLAGS: /opt/homebrew/opt/xz/lib/liblzma.a
#cgo LDFLAGS: /opt/homebrew/opt/zstd/lib/libzstd.a
#cgo LDFLAGS: /opt/homebrew/opt/lz4/lib/liblz4.a
#cgo LDFLAGS: /opt/homebrew/opt/libb2/lib/libb2.a
#cgo LDFLAGS: /opt/homebrew/opt/bzip2/lib/libbz2.a
#cgo LDFLAGS: /opt/homebrew/opt/zlib/lib/libz.a
#cgo LDFLAGS: -liconv -lexpat
*/
import "C"
