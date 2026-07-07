package update

import (
	"runtime"
	"strings"
)

func deriveAssetName(tag string) string {
	version := strings.TrimPrefix(tag, "v")
	ext := ".tar.gz"
	if runtime.GOOS == "windows" {
		ext = ".zip"
	}
	return "servitor_" + version + "_" + runtime.GOOS + "_" + runtime.GOARCH + ext
}
