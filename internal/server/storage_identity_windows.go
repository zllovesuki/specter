package server

import (
	"fmt"
	"os"
	"path/filepath"

	"golang.org/x/sys/windows"
)

func prepareKVProviderDirectory(dir string) error {
	return os.MkdirAll(dir, 0750)
}

func publishKVProviderMarker(tempPath, dir string) error {
	from, err := windows.UTF16PtrFromString(tempPath)
	if err != nil {
		return err
	}
	to, err := windows.UTF16PtrFromString(filepath.Join(dir, kvProviderMarker))
	if err != nil {
		return err
	}
	// Windows does not support syncing a read-only directory handle. Publish
	// the synced file with a write-through move, without replacing any marker
	// established by a concurrent startup.
	return windows.MoveFileEx(from, to, windows.MOVEFILE_WRITE_THROUGH)
}

func syncKVProviderIdentity(dir string) error {
	// FlushFileBuffers requires a writable file handle on Windows.
	f, err := os.OpenFile(filepath.Join(dir, kvProviderMarker), os.O_RDWR, 0)
	if err != nil {
		return fmt.Errorf("opening storage identity to sync: %w", err)
	}
	defer f.Close()
	if err := f.Sync(); err != nil {
		return fmt.Errorf("syncing storage identity: %w", err)
	}
	return nil
}
