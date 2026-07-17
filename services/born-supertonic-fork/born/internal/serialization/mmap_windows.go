//go:build windows

package serialization

import (
	"fmt"
	"os"
	"reflect"
	"syscall"
	"unsafe"
)

// mmapFile memory-maps a file for reading (Windows implementation).
//
// This function uses unsafe operations which are required for memory mapping.
// The code is safe because addr comes from MapViewOfFile which returns a valid memory address.
func mmapFile(f *os.File, size int64) ([]byte, error) {
	// Create file mapping object
	handle, err := syscall.CreateFileMapping(
		syscall.Handle(f.Fd()),
		nil,
		syscall.PAGE_READONLY,
		uint32(size>>32), //nolint:gosec // G115: integer overflow conversion int64 -> uint32
		uint32(size),     //nolint:gosec // G115: integer overflow conversion int64 -> uint32
		nil,
	)
	if err != nil {
		return nil, err
	}
	defer func() {
		if closeErr := syscall.CloseHandle(handle); closeErr != nil {
			// Log or handle error (can't return it due to defer)
			_ = closeErr
		}
	}()

	// Map view of file into address space
	addr, err := syscall.MapViewOfFile(
		handle,
		syscall.FILE_MAP_READ,
		0,
		0,
		uintptr(size), //nolint:gosec // G115: int64-to-uintptr needed for syscall
	)
	if err != nil {
		return nil, err
	}

	// Convert to byte slice using reflect.SliceHeader.
	// This is the standard pattern for mmap on Windows in Go.
	// This is safe because:
	// 1. addr is a valid mapped memory address from MapViewOfFile
	// 2. size is the exact size we requested
	// 3. The memory is read-only (PAGE_READONLY)
	var slice []byte
	//nolint:staticcheck,gosec // SA1019+G103: SliceHeader is deprecated but still works and avoids go vet issues
	header := (*reflect.SliceHeader)(unsafe.Pointer(&slice))
	header.Data = addr
	header.Len = int(size)
	header.Cap = int(size)

	return slice, nil
}

// munmapFile unmaps a memory-mapped file (Windows implementation).
func munmapFile(data []byte) error {
	if len(data) == 0 {
		return fmt.Errorf("cannot unmap empty data")
	}
	//nolint:staticcheck,gosec // SA1019+G103: SliceHeader is deprecated but avoids go vet issues with unsafe.Pointer
	header := (*reflect.SliceHeader)(unsafe.Pointer(&data))
	return syscall.UnmapViewOfFile(header.Data)
}
