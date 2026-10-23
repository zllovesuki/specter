//go:build !windows

package server

import (
	"fmt"
	"os"
	"path/filepath"
)

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
