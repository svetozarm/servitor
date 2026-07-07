package update

import "os"

func writable(dir string) bool {
	f, err := os.CreateTemp(dir, ".servitor-write-check-*")
	if err != nil {
		return false
	}
	name := f.Name()
	f.Close()
	os.Remove(name)
	return true
}
