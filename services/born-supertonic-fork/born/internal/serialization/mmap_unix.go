//go:build unix

package serialization

import (
	"os"
	"syscall"
)

// mmapFile memory-maps a file for reading (Unix implementation).
func mmapFile(f *os.File, size int64) ([]byte, error) {
	return syscall.Mmap(
		int(f.Fd()),
		0,
		int(size),
		syscall.PROT_READ,
		syscall.MAP_SHARED,
	)
}

// munmapFile unmaps a memory-mapped file (Unix implementation).
func munmapFile(data []byte) error {
	return syscall.Munmap(data)
}
