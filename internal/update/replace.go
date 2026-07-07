//go:build !windows

package update

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

func replaceBinary(archivePath, binaryPath string) error {
	if !strings.HasSuffix(archivePath, ".tar.gz") {
		return fmt.Errorf("%w: expected .tar.gz archive", ErrReplaceFailed)
	}

	f, err := os.Open(archivePath)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrReplaceFailed, err)
	}
	defer f.Close()

	gr, err := gzip.NewReader(f)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrReplaceFailed, err)
	}
	defer gr.Close()

	tr := tar.NewReader(gr)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			return fmt.Errorf("%w: binary not found in archive", ErrReplaceFailed)
		}
		if err != nil {
			return fmt.Errorf("%w: %v", ErrReplaceFailed, err)
		}
		if hdr.Typeflag != tar.TypeReg {
			continue
		}
		if filepath.Base(hdr.Name) != "servitor" {
			continue
		}
		return extractAndReplace(tr, binaryPath)
	}
}

func extractAndReplace(r io.Reader, binaryPath string) error {
	dir := filepath.Dir(binaryPath)
	tmp, err := os.CreateTemp(dir, ".servitor-update-*")
	if err != nil {
		return fmt.Errorf("%w: %v", ErrReplaceFailed, err)
	}
	tmpPath := tmp.Name()

	if _, err = io.Copy(tmp, r); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("%w: %v", ErrReplaceFailed, err)
	}
	if err = tmp.Close(); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("%w: %v", ErrReplaceFailed, err)
	}
	if err = os.Chmod(tmpPath, 0755); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("%w: %v", ErrReplaceFailed, err)
	}
	if err = os.Rename(tmpPath, binaryPath); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("%w: %v", ErrReplaceFailed, err)
	}
	return nil
}
