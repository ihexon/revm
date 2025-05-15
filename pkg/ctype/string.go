package ctype

/*
#include <stdlib.h>
#include <stdio.h>
*/
import "C"
import (
	"unsafe"
)

// slicePtr gives you an unsafe pointer to the start of a slice.
func slicePtr[T any](slc []T) unsafe.Pointer {
	return unsafe.Pointer(unsafe.SliceData(slc))
}
