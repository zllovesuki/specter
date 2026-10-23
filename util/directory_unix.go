//go:build !windows

package util

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// MkdirAllDurable creates missing directories and syncs their parent entries.
// Existing ancestors need only traversal permission and are left unchanged.
func MkdirAllDurable(dir string, perm os.FileMode) error {
	dir = filepath.Clean(dir)
	info, err := os.Stat(dir)
	if err == nil {
		if !info.IsDir() {
			return fmt.Errorf("path %q is not a directory", dir)
		}
		return nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	parent := filepath.Dir(dir)
	if parent == dir {
		return err
	}
	if err := MkdirAllDurable(parent, perm); err != nil {
		return err
	}
	err = os.Mkdir(dir, perm)
	created := err == nil
	if err != nil && !errors.Is(err, os.ErrExist) {
		return err
	}
	if !created {
		info, err := os.Stat(dir)
		if err != nil {
			return err
		}
		if !info.IsDir() {
			return fmt.Errorf("path %q is not a directory", dir)
		}
	}
	if err := syncDirectory(parent); err != nil {
		if created {
			// A retry must not mistake a failed creation flush for an established
			// directory. Remove only the empty directory this call created.
			_ = os.Remove(dir)
		}
		return err
	}
	return nil
}

func syncDirectory(dir string) error {
	f, err := os.Open(dir)
	if err != nil {
		return fmt.Errorf("opening directory to sync: %w", err)
	}
	return errors.Join(f.Sync(), f.Close())
}
