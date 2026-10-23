//go:build !windows

package server

import (
	"fmt"
	"os"
	"path/filepath"
)

func prepareKVProviderDirectory(dir string) error {
	dir = filepath.Clean(dir)
	info, err := os.Stat(dir)
	if err == nil {
		if !info.IsDir() {
			return fmt.Errorf("data path %q is not a directory", dir)
		}
		return nil
	}
	if !os.IsNotExist(err) {
		return err
	}
	parent := filepath.Dir(dir)
	if parent == dir {
		return err
	}
	if err := prepareKVProviderDirectory(parent); err != nil {
		return err
	}
	err = os.Mkdir(dir, 0750)
	created := err == nil
	if err != nil && !os.IsExist(err) {
		return err
	}
	if !created {
		info, err := os.Stat(dir)
		if err != nil {
			return err
		}
		if !info.IsDir() {
			return fmt.Errorf("data path %q is not a directory", dir)
		}
	}
	// Persist each new directory's name before creating a child or marker.
	// Existing ancestors need only traversal permission and are left alone.
	if err := syncKVProviderIdentity(parent); err != nil {
		if created {
			// Only remove this initialization's empty directory. A retry must
			// not mistake a failed creation flush for an established directory.
			_ = os.Remove(dir)
		}
		return err
	}
	return nil
}

func publishKVProviderMarker(tempPath, dir string) error {
	// A hard link publishes the complete, synced marker atomically and never
	// overwrites an identity established by a concurrent startup.
	if err := os.Link(tempPath, filepath.Join(dir, kvProviderMarker)); err != nil {
		return err
	}
	if err := os.Remove(tempPath); err != nil {
		return fmt.Errorf("removing temporary marker: %w", err)
	}
	return syncKVProviderIdentity(dir)
}

func syncKVProviderIdentity(dir string) error {
	d, err := os.Open(dir)
	if err != nil {
		return fmt.Errorf("opening directory to sync storage identity: %w", err)
	}
	syncErr := d.Sync()
	closeErr := d.Close()
	if syncErr != nil {
		return fmt.Errorf("syncing directory for storage identity: %w", syncErr)
	}
	if closeErr != nil {
		return fmt.Errorf("closing directory after syncing storage identity: %w", closeErr)
	}
	return nil
}
