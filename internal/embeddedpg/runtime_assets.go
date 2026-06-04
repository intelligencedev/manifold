package embeddedpg

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/klauspost/compress/zstd"

	pgassets "manifold/internal/embeddedpg/assets"
)

type preparedRuntimeAsset struct {
	runtimeID string
	pgMajor   int
	version   string
	root      string
}

func prepareRuntimeAsset(asset pgassets.RuntimeAsset, runtimeCacheDir string) (preparedRuntimeAsset, error) {
	if strings.TrimSpace(asset.RuntimeID) == "" {
		return preparedRuntimeAsset{}, errors.New("embedded postgres runtime asset missing runtime ID")
	}
	root := filepath.Join(runtimeCacheDir, asset.RuntimeID)
	prepared := preparedRuntimeAsset{
		runtimeID: asset.RuntimeID,
		pgMajor:   asset.PGMajor,
		version:   asset.Manifest.Postgres.Version,
		root:      root,
	}

	if err := verifyRuntimeFiles(root, asset.Manifest); err == nil {
		return prepared, nil
	}

	if err := os.MkdirAll(runtimeCacheDir, 0755); err != nil {
		return preparedRuntimeAsset{}, fmt.Errorf("create runtime cache directory: %w", err)
	}
	stage, err := os.MkdirTemp(runtimeCacheDir, ".extract-"+asset.RuntimeID+"-")
	if err != nil {
		return preparedRuntimeAsset{}, fmt.Errorf("create runtime extraction directory: %w", err)
	}
	defer func() { _ = os.RemoveAll(stage) }()

	if err := extractRuntimeArchive(asset.ArchiveName, asset.Archive, stage); err != nil {
		return preparedRuntimeAsset{}, fmt.Errorf("extract embedded postgres runtime %s: %w", asset.RuntimeID, err)
	}
	if err := verifyRuntimeFiles(stage, asset.Manifest); err != nil {
		return preparedRuntimeAsset{}, fmt.Errorf("verify embedded postgres runtime %s: %w", asset.RuntimeID, err)
	}

	if err := os.RemoveAll(root); err != nil {
		return preparedRuntimeAsset{}, fmt.Errorf("replace runtime directory: %w", err)
	}
	if err := os.Rename(stage, root); err != nil {
		return preparedRuntimeAsset{}, fmt.Errorf("install runtime directory: %w", err)
	}
	return prepared, nil
}

func verifyRuntimeFiles(root string, manifest pgassets.Manifest) error {
	if strings.TrimSpace(root) == "" {
		return errors.New("runtime root is empty")
	}
	for _, file := range manifest.Files {
		dest, err := safeRuntimePath(root, file.Path)
		if err != nil {
			return err
		}
		data, err := os.ReadFile(dest)
		if err != nil {
			return fmt.Errorf("read %s: %w", file.Path, err)
		}
		sum := sha256.Sum256(data)
		if !strings.EqualFold(hex.EncodeToString(sum[:]), strings.TrimSpace(file.SHA256)) {
			return fmt.Errorf("checksum mismatch for %s", file.Path)
		}
		if strings.TrimSpace(file.Mode) != "" && runtime.GOOS != "windows" {
			info, err := os.Stat(dest)
			if err != nil {
				return fmt.Errorf("stat %s: %w", file.Path, err)
			}
			gotMode := fmt.Sprintf("%04o", info.Mode().Perm())
			if gotMode != file.Mode {
				return fmt.Errorf("mode mismatch for %s: got %s want %s", file.Path, gotMode, file.Mode)
			}
		}
	}
	return nil
}

func extractRuntimeArchive(name string, data []byte, dest string) error {
	if len(data) == 0 {
		return errors.New("runtime archive is empty")
	}
	switch {
	case strings.HasSuffix(name, ".zip"):
		return extractRuntimeZip(data, dest)
	case strings.HasSuffix(name, ".tar.zst"):
		reader, err := zstd.NewReader(bytes.NewReader(data))
		if err != nil {
			return fmt.Errorf("open zstd stream: %w", err)
		}
		defer reader.Close()
		return extractRuntimeTar(reader, dest)
	case strings.HasSuffix(name, ".tar.gz"), strings.HasSuffix(name, ".tgz"):
		reader, err := gzip.NewReader(bytes.NewReader(data))
		if err != nil {
			return fmt.Errorf("open gzip stream: %w", err)
		}
		defer func() { _ = reader.Close() }()
		return extractRuntimeTar(reader, dest)
	default:
		return fmt.Errorf("unsupported runtime archive format %q", name)
	}
}

func extractRuntimeZip(data []byte, dest string) error {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return fmt.Errorf("open zip archive: %w", err)
	}
	for _, file := range zr.File {
		if file.FileInfo().IsDir() {
			if _, err := safeRuntimePath(dest, file.Name); err != nil {
				return err
			}
			continue
		}
		target, err := safeRuntimePath(dest, file.Name)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
			return err
		}
		in, err := file.Open()
		if err != nil {
			return err
		}
		err = writeRuntimeFile(target, in, file.Mode())
		closeErr := in.Close()
		if err != nil {
			return err
		}
		if closeErr != nil {
			return closeErr
		}
	}
	return nil
}

func extractRuntimeTar(reader io.Reader, dest string) error {
	tr := tar.NewReader(reader)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return fmt.Errorf("read tar archive: %w", err)
		}
		if isArchiveRootPath(hdr.Name) {
			continue
		}
		target, err := safeRuntimePath(dest, hdr.Name)
		if err != nil {
			return err
		}
		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, os.FileMode(hdr.Mode)); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
				return err
			}
			if err := writeRuntimeFile(target, tr, os.FileMode(hdr.Mode)); err != nil {
				return err
			}
		case tar.TypeSymlink:
			if err := ensureSafeLinkTarget(hdr.Linkname); err != nil {
				return err
			}
			if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
				return err
			}
			if err := os.Symlink(hdr.Linkname, target); err != nil && !os.IsExist(err) {
				return err
			}
		default:
			continue
		}
	}
}

func isArchiveRootPath(name string) bool {
	clean := path.Clean(strings.TrimPrefix(name, "./"))
	return clean == "."
}

func writeRuntimeFile(target string, reader io.Reader, mode os.FileMode) error {
	out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode.Perm())
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(out, reader)
	closeErr := out.Close()
	if copyErr != nil {
		return copyErr
	}
	return closeErr
}

func safeRuntimePath(root, rel string) (string, error) {
	if strings.TrimSpace(rel) == "" {
		return "", errors.New("runtime archive contains empty path")
	}
	if path.IsAbs(rel) || filepath.IsAbs(rel) {
		return "", fmt.Errorf("runtime archive contains absolute path %q", rel)
	}
	clean := path.Clean(strings.TrimPrefix(rel, "./"))
	if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") {
		return "", fmt.Errorf("runtime archive contains unsafe path %q", rel)
	}
	target := filepath.Join(root, filepath.FromSlash(clean))
	cleanRoot := filepath.Clean(root)
	cleanTarget := filepath.Clean(target)
	if cleanTarget != cleanRoot && !strings.HasPrefix(cleanTarget, cleanRoot+string(filepath.Separator)) {
		return "", fmt.Errorf("runtime archive path escapes root: %q", rel)
	}
	return target, nil
}

func ensureSafeLinkTarget(target string) error {
	if strings.TrimSpace(target) == "" {
		return errors.New("runtime archive contains symlink with empty target")
	}
	if path.IsAbs(target) || filepath.IsAbs(target) {
		return fmt.Errorf("runtime archive contains absolute symlink target %q", target)
	}
	clean := path.Clean(target)
	if clean == ".." || strings.HasPrefix(clean, "../") {
		return fmt.Errorf("runtime archive contains unsafe symlink target %q", target)
	}
	return nil
}
