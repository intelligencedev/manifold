package main

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

func main() {
	root := "."
	dirs := map[string]struct{}{}
	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			// skip vendor and .git
			if d.Name() == "vendor" || strings.HasPrefix(path, "./.git") || strings.HasPrefix(path, "./.github") {
				return filepath.SkipDir
			}
			// consider directories that contain .go files
			hasGo := false
			entries, _ := os.ReadDir(path)
			for _, e := range entries {
				if strings.HasSuffix(e.Name(), ".go") {
					hasGo = true
					break
				}
			}
			if hasGo {
				dirs[path] = struct{}{}
			}
		}
		return nil
	})

	for d := range dirs {
		// count _test.go files
		count := 0
		entries, _ := os.ReadDir(d)
		for _, e := range entries {
			if strings.HasSuffix(e.Name(), "_test.go") {
				count++
			}
		}
		rel := strings.TrimPrefix(d, "./")
		if rel == "" {
			rel = "."
		}
		fmt.Printf("%s %d\n", rel, count)
	}
}
