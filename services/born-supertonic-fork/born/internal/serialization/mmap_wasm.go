//go:build wasm

package serialization

import (
	"errors"
	"os"
)

// mmapFile is not supported on WASM — memory-mapped files require OS syscalls unavailable in the browser sandbox.
func mmapFile(_ *os.File, _ int64) ([]byte, error) {
	return nil, errors.New("mmap is not supported on WASM; use NewReader for file loading")
}

// munmapFile is not supported on WASM — memory-mapped files require OS syscalls unavailable in the browser sandbox.
func munmapFile(_ []byte) error {
	return errors.New("munmap is not supported on WASM; use NewReader for file loading")
}
