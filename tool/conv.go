package tool

import "unsafe"

// BytesToString converts a byte slice to a string without allocating memory.
// The returned string should be used within the same scope as the original byte slice.
func BytesToString(b []byte) string {
	return *(*string)(unsafe.Pointer(&b))
}

// StringToBytes converts a string to a byte slice without allocating memory.
// The returned slice should not be modified.
func StringToBytes(s string) (b []byte) {
	return unsafe.Slice(unsafe.StringData(s), len(s))
}
