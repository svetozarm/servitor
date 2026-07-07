//go:build windows

package update

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

func replaceBinary(archivePath, binaryPath string) error {
	if !strings.HasSuffix(archivePath, ".zip") {
		return fmt.Errorf("%w: expected .zip archive", ErrReplaceFailed)
	}

	zr, err := zip.OpenReader(archivePath)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrReplaceFailed, err)
	}
	defer zr.Close()

	for _, f := range zr.File {
		if filepath.Base(f.Name) != "servitor.exe" {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return fmt.Errorf("%w: %v", ErrReplaceFailed, err)
		}
		defer rc.Close()

		// On Windows, rename running exe to .old then write new
		oldPath := binaryPath + ".old"
		os.Remove(oldPath)
		if err := os.Rename(binaryPath, oldPath); err != nil {
			return fmt.Errorf("%w: %v", ErrReplaceFailed, err)
		}

		out, err := os.OpenFile(binaryPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0755)
		if err != nil {
			os.Rename(oldPath, binaryPath)
			return fmt.Errorf("%w: %v", ErrReplaceFailed, err)
		}
		if _, err := io.Copy(out, rc); err != nil {
			out.Close()
			os.Remove(binaryPath)
			os.Rename(oldPath, binaryPath)
			return fmt.Errorf("%w: %v", ErrReplaceFailed, err)
		}
		out.Close()
		os.Remove(oldPath)
		return nil
	}
	return fmt.Errorf("%w: binary not found in archive", ErrReplaceFailed)
}
