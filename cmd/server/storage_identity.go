package server

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const kvProviderMarker = "kv-provider"

// preflightKVProvider establishes identity for the entire server data directory
// before any provider opens files or any vnode joins the ring. Layout knowledge
// belongs here, where provider selection happens, rather than in the providers.
func preflightKVProvider(option, dir string) error {
	switch option {
	case "memory", "aof", "sqlite":
	default:
		return fmt.Errorf("unknown kv provider: %s", option)
	}
	if dir == "" {
		return fmt.Errorf("empty data directory")
	}

	marker, err := readKVProviderMarker(dir)
	if err != nil {
		return err
	}
	layout, err := inspectKVLayout(dir)
	if err != nil {
		return err
	}
	if marker != "" && layout != "" && marker != layout {
		return fmt.Errorf("storage provider marker identifies %s but data directory %q contains %s storage; preserve the directory and resolve the conflicting stores through recovery before restarting", marker, dir, layout)
	}
	for _, existing := range []string{marker, layout} {
		if existing != "" && existing != option {
			return fmt.Errorf("configured kv provider %s conflicts with %s storage in %q; restart with --kv-provider=%s to recover existing data, or use a separate empty --data-dir for a new store; do not remove the marker or stored data to bypass this check", option, existing, dir, existing)
		}
	}
	if option == "memory" {
		return nil
	}
	if marker != "" {
		// Retry a flush if an earlier startup published the marker but failed
		// to finish syncing its identity before returning an error.
		return syncKVProviderIdentity(dir)
	}
	return writeKVProviderMarker(dir, option)
}

func readKVProviderMarker(dir string) (string, error) {
	path := filepath.Join(dir, kvProviderMarker)
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("inspecting storage provider marker: %w", err)
	}
	if !info.Mode().IsRegular() || info.Size() > 16 {
		return "", fmt.Errorf("invalid storage provider marker %q; preserve the data directory and recover its provider identity before restarting", path)
	}
	f, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("reading storage provider marker: %w", err)
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, 17))
	if err != nil {
		return "", fmt.Errorf("reading storage provider marker: %w", err)
	}
	switch string(data) {
	case "aof\n":
		return "aof", nil
	case "sqlite\n":
		return "sqlite", nil
	default:
		return "", fmt.Errorf("invalid storage provider marker %q; expected aof or sqlite followed by a newline; preserve the data directory and recover its provider identity before restarting", path)
	}
}

// inspectKVLayout only reads directory metadata. Opening a provider here could
// replay, repair, or create storage before we know the chosen provider is safe.
func inspectKVLayout(dir string) (string, error) {
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("reading data directory: %w", err)
	}
	var provider string
	for _, entry := range entries {
		// Inspect every existing vnode, including those beyond --virtual. Other
		// top-level entries (for example the SQLite runtime cache) aren't stores.
		if strings.Trim(entry.Name(), "0123456789") != "" {
			continue
		}
		vnodeDir := filepath.Join(dir, entry.Name())
		children, err := os.ReadDir(vnodeDir)
		if err != nil {
			return "", fmt.Errorf("reading vnode storage directory: %w", err)
		}
		for _, child := range children {
			var found string
			switch child.Name() {
			case "wal":
				found = "aof"
			case "sqlite3":
				found = "sqlite"
			default:
				return "", fmt.Errorf("unrecognized storage layout at %q; preserve the directory and recover or migrate the existing store before restarting", filepath.Join(vnodeDir, child.Name()))
			}
			info, err := os.Stat(filepath.Join(vnodeDir, child.Name()))
			if err != nil {
				return "", fmt.Errorf("inspecting %s storage directory: %w", found, err)
			}
			if !info.IsDir() {
				return "", fmt.Errorf("expected %s storage directory at %q", found, filepath.Join(vnodeDir, child.Name()))
			}
			if provider != "" && provider != found {
				return "", fmt.Errorf("mixed aof and sqlite storage in data directory %q; preserve the entire directory and resolve the stores through explicit recovery or migration before restarting", dir)
			}
			provider = found
		}
	}
	return provider, nil
}

func writeKVProviderMarker(dir, provider string) error {
	if err := prepareKVProviderDirectory(dir); err != nil {
		return fmt.Errorf("creating data directory: %w", err)
	}
	f, err := os.CreateTemp(dir, ".kv-provider-*")
	if err != nil {
		return fmt.Errorf("creating storage provider marker: %w", err)
	}
	defer os.Remove(f.Name())
	defer f.Close()
	if _, err := f.WriteString(provider + "\n"); err != nil {
		return fmt.Errorf("writing storage provider marker: %w", err)
	}
	if err := f.Sync(); err != nil {
		return fmt.Errorf("syncing storage provider marker: %w", err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("closing storage provider marker: %w", err)
	}
	if err := publishKVProviderMarker(f.Name(), dir); err != nil {
		return fmt.Errorf("publishing storage provider marker: %w", err)
	}
	return nil
}
