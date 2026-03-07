//go:build ignore

package main

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

func ensureWithinRoot(workdir, rel string) error {
	root, err := os.OpenRoot(workdir)
	if err != nil {
		return fmt.Errorf("open root %q: %w", workdir, err)
	}
	defer root.Close()

	candidate := rel
	for candidate != "" && candidate != "." {
		f, err := root.Open(candidate)
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				candidate = filepath.Dir(candidate)
				continue
			}
			return fmt.Errorf("path %q escapes workdir: %w", rel, err)
		}
		f.Close()
		break
	}
	return nil
}

func main() {
	err := ensureWithinRoot(".", "...")
	fmt.Println(err)
}
