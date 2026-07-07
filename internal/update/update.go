package update

import (
	"context"
	"os"
	"path/filepath"
)

var executablePath = os.Executable

type Result struct {
	OldVersion string
	NewVersion string
	UpToDate   bool
}

func Run(ctx context.Context, currentVersion string, checker ReleaseChecker) (*Result, error) {
	if currentVersion == "" {
		return nil, ErrDevBuild
	}

	release, err := checker.LatestRelease(ctx)
	if err != nil {
		return nil, err
	}

	if !isNewer(release.Tag, currentVersion) {
		return &Result{UpToDate: true}, nil
	}

	assetName := deriveAssetName(release.Tag)
	var assetURL, checksumsURL string
	for _, a := range release.Assets {
		switch a.Name {
		case assetName:
			assetURL = a.URL
		case "checksums.txt":
			checksumsURL = a.URL
		}
	}
	if assetURL == "" || checksumsURL == "" {
		return nil, ErrAssetNotFound
	}

	binaryPath, err := executablePath()
	if err != nil {
		return nil, ErrPermission
	}
	dir := filepath.Dir(binaryPath)
	if !writable(dir) {
		return nil, ErrPermission
	}

	archiveDest := filepath.Join(dir, assetName)
	checksumsDest := filepath.Join(dir, "checksums.txt")
	cleanup := func() {
		os.Remove(archiveDest)
		os.Remove(checksumsDest)
	}

	if err := checker.DownloadAsset(ctx, assetURL, archiveDest); err != nil {
		cleanup()
		return nil, err
	}
	if err := checker.DownloadAsset(ctx, checksumsURL, checksumsDest); err != nil {
		cleanup()
		return nil, err
	}

	if err := verifyChecksum(archiveDest, assetName, checksumsDest); err != nil {
		cleanup()
		return nil, err
	}
	if err := replaceBinary(archiveDest, binaryPath); err != nil {
		cleanup()
		return nil, err
	}

	cleanup()
	return &Result{OldVersion: currentVersion, NewVersion: release.Tag}, nil
}
