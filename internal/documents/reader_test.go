package documents

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestFileReaderFiltersBinaries(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "text.txt"), []byte("hello"), 0644)
	os.WriteFile(filepath.Join(dir, "nul.bin"), []byte("a\x00b"), 0644)
	os.WriteFile(filepath.Join(dir, "elf"), []byte("\x7fELF"), 0644)
	os.WriteFile(filepath.Join(dir, "img.jpg"), []byte("\xFF\xD8\xFF"), 0644)

	r := NewDirReader(dir)
	ch := make(chan FileData, 10)
	if err := r.Stream(context.Background(), ch); err != nil {
		t.Fatal(err)
	}
	close(ch)
	var paths []string
	for f := range ch {
		paths = append(paths, f.Path)
	}
	if len(paths) != 1 || paths[0] != "text.txt" {
		t.Fatalf("unexpected files %v", paths)
	}
}
