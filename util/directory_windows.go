package util

import "os"

// MkdirAllDurable creates missing directories. Windows directory handles do not
// support os.File.Sync; callers retain responsibility for file durability.
func MkdirAllDurable(dir string, perm os.FileMode) error {
	return os.MkdirAll(dir, perm)
}
