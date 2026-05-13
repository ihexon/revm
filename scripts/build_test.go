package main

import (
	"reflect"
	"testing"
)

func TestParseLDDLibraryPaths(t *testing.T) {
	output := `
	linux-vdso.so.1 (0x0000eaeb6aaaf000)
	libext2fs.so.2 => /mnt/revm/out/chroot/bin/../lib/libext2fs.so.2 (0x0000eaeb6a9c0000)
	libcom_err.so.2 => /lib/aarch64-linux-gnu/libcom_err.so.2 (0x0000eaeb6a1b0000)
	/lib/ld-linux-aarch64.so.1 (0x0000eaeb6aa60000)
	libmissing.so.1 => not found
`

	got := parseLDDLibraryPaths(output)
	want := []string{
		"/mnt/revm/out/chroot/bin/../lib/libext2fs.so.2",
		"/lib/aarch64-linux-gnu/libcom_err.so.2",
		"/lib/ld-linux-aarch64.so.1",
	}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("parseLDDLibraryPaths() = %#v, want %#v", got, want)
	}
}
