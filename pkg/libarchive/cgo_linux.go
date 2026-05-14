//go:build linux && (arm64 || amd64)

package libarchive_go

/*
#cgo linux,arm64 LDFLAGS: /usr/lib/aarch64-linux-gnu/libarchive.a /usr/lib/aarch64-linux-gnu/libxml2.a /usr/lib/aarch64-linux-gnu/libnettle.a /usr/lib/aarch64-linux-gnu/libacl.a /usr/lib/aarch64-linux-gnu/libzstd.a /usr/lib/aarch64-linux-gnu/liblz4.a /usr/lib/aarch64-linux-gnu/libbz2.a /usr/lib/aarch64-linux-gnu/libz.a /usr/lib/aarch64-linux-gnu/liblzma.a /usr/lib/aarch64-linux-gnu/libxxhash.a /usr/lib/aarch64-linux-gnu/libicuuc.a /usr/lib/aarch64-linux-gnu/libicudata.a -lstdc++ /usr/lib/aarch64-linux-gnu/libm.a
#cgo linux,amd64 LDFLAGS: /usr/lib/x86_64-linux-gnu/libarchive.a /usr/lib/x86_64-linux-gnu/libxml2.a /usr/lib/x86_64-linux-gnu/libnettle.a /usr/lib/x86_64-linux-gnu/libacl.a /usr/lib/x86_64-linux-gnu/libzstd.a /usr/lib/x86_64-linux-gnu/liblz4.a /usr/lib/x86_64-linux-gnu/libbz2.a /usr/lib/x86_64-linux-gnu/libz.a /usr/lib/x86_64-linux-gnu/liblzma.a /usr/lib/x86_64-linux-gnu/libxxhash.a /usr/lib/x86_64-linux-gnu/libicuuc.a /usr/lib/x86_64-linux-gnu/libicudata.a -lstdc++ -lm
*/
import "C"
