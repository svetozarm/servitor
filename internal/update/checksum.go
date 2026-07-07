package update

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"strings"
)

func verifyChecksum(archivePath, archiveName, checksumsPath string) error {
	checksums, err := parseChecksums(checksumsPath)
	if err != nil {
		return err
	}
	expected, ok := checksums[archiveName]
	if !ok {
		return fmt.Errorf("%w: no entry for %s", ErrChecksumMismatch, archiveName)
	}
	actual, err := computeSHA256(archivePath)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrChecksumMismatch, err)
	}
	if !strings.EqualFold(actual, expected) {
		return fmt.Errorf("%w: expected %s, got %s", ErrChecksumMismatch, expected, actual)
	}
	return nil
}

func parseChecksums(path string) (map[string]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrChecksumMismatch, err)
	}
	defer f.Close()

	result := make(map[string]string)
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		parts := strings.Fields(line)
		if len(parts) != 2 || len(parts[0]) != 64 {
			continue
		}
		result[parts[1]] = parts[0]
	}
	return result, nil
}

func computeSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
