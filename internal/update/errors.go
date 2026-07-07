package update

import "errors"

var (
	ErrUpdateAPI        = errors.New("update: GitHub API error")
	ErrChecksumMismatch = errors.New("update: checksum mismatch")
	ErrAssetNotFound    = errors.New("update: platform asset not found in release")
	ErrPermission       = errors.New("update: binary path not writable")
	ErrReplaceFailed    = errors.New("update: failed to replace binary")
	ErrDevBuild         = errors.New("update: no version embedded (development build)")
)
