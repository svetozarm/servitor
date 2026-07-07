package update

import (
	"strconv"
	"strings"
)

func isNewer(remote, current string) bool {
	rv := parseSemver(remote)
	cv := parseSemver(current)
	for i := 0; i < 3; i++ {
		if rv[i] > cv[i] {
			return true
		}
		if rv[i] < cv[i] {
			return false
		}
	}
	return false
}

func parseSemver(s string) [3]int {
	s = strings.TrimPrefix(s, "v")
	parts := strings.SplitN(s, ".", 3)
	var v [3]int
	for i := 0; i < 3 && i < len(parts); i++ {
		n, err := strconv.Atoi(parts[i])
		if err != nil {
			return v
		}
		v[i] = n
	}
	return v
}
