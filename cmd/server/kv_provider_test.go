package server

import (
	"context"
	"flag"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/urfave/cli/v2"
	"go.uber.org/zap"
)

func TestKVProviderPreflightPreservesExistingStores(t *testing.T) {
	for _, provider := range []string{"aof", "sqlite"} {
		t.Run(provider, func(t *testing.T) {
			dir := t.TempDir()
			vnodeDir := filepath.Join(dir, "12")
			cacheDir := filepath.Join(dir, "cache")
			kv, stop, err := getKVProvider(zap.NewNop(), provider, vnodeDir, cacheDir)
			require.NoError(t, err)
			err = kv.Put(context.Background(), []byte("existing"), []byte("preserve me"))
			stop()
			require.NoError(t, err)
			before := snapshotStorage(t, dir)

			other := "sqlite"
			if provider == "sqlite" {
				other = "aof"
			}
			// An unmarked store from an older server must not be silently replaced.
			require.ErrorContains(t, preflightKVProvider(other, dir), "conflicts")
			require.ErrorContains(t, preflightKVProvider("memory", dir), "conflicts")
			require.Equal(t, before, snapshotStorage(t, dir))

			require.NoError(t, preflightKVProvider(provider, dir))
			before[kvProviderMarker] = provider + "\n"
			require.Equal(t, before, snapshotStorage(t, dir), "adoption must only add the provider marker")
			require.NoError(t, preflightKVProvider(provider, dir))
			require.ErrorContains(t, preflightKVProvider(other, dir), "conflicts")
			require.Equal(t, before, snapshotStorage(t, dir))

			kv, stop, err = getKVProvider(zap.NewNop(), provider, vnodeDir, cacheDir)
			require.NoError(t, err)
			defer stop()
			value, err := kv.Get(context.Background(), []byte("existing"))
			require.NoError(t, err)
			require.Equal(t, []byte("preserve me"), value)
		})
	}
}

func TestKVProviderPreflightLayouts(t *testing.T) {
	tests := []struct {
		name     string
		provider string
		dirs     []string
		files    map[string]string
		wantErr  string
	}{
		{name: "fresh aof", provider: "aof"},
		{name: "fresh sqlite", provider: "sqlite"},
		{name: "fresh memory", provider: "memory"},
		{name: "empty vnode", provider: "aof", dirs: []string{"0", "8"}},
		{name: "cache alone is not storage", provider: "memory", files: map[string]string{"cache/sqlite3/runtime": "cache"}},
		{name: "empty aof directory identifies storage", provider: "sqlite", dirs: []string{"0/wal"}, wantErr: "conflicts"},
		{name: "empty sqlite directory identifies storage", provider: "aof", dirs: []string{"0/sqlite3"}, wantErr: "conflicts"},
		{name: "sqlite sidecars identify storage", provider: "aof", files: map[string]string{"0/sqlite3/db-wal": "journal", "0/sqlite3/db-shm": "shared memory"}, wantErr: "conflicts"},
		{name: "same vnode mixture", provider: "aof", dirs: []string{"0/wal", "0/sqlite3"}, wantErr: "mixed aof and sqlite"},
		{name: "cross vnode mixture", provider: "sqlite", dirs: []string{"0/sqlite3", "99/wal"}, wantErr: "mixed aof and sqlite"},
		{name: "marker conflict without store", provider: "sqlite", files: map[string]string{kvProviderMarker: "aof\n"}, wantErr: "conflicts"},
		{name: "marker cannot hide conflicting layout", provider: "aof", dirs: []string{"0/sqlite3"}, files: map[string]string{kvProviderMarker: "aof\n"}, wantErr: "marker identifies aof"},
		{name: "memory rejects marker", provider: "memory", files: map[string]string{kvProviderMarker: "sqlite\n"}, wantErr: "conflicts"},
		{name: "memory rejects layout", provider: "memory", dirs: []string{"0/wal"}, wantErr: "conflicts"},
		{name: "empty marker", provider: "aof", files: map[string]string{kvProviderMarker: ""}, wantErr: "invalid storage provider marker"},
		{name: "unknown marker", provider: "aof", files: map[string]string{kvProviderMarker: "unknown\n"}, wantErr: "invalid storage provider marker"},
		{name: "memory marker is invalid", provider: "memory", files: map[string]string{kvProviderMarker: "memory\n"}, wantErr: "invalid storage provider marker"},
		{name: "marker lacks newline", provider: "aof", files: map[string]string{kvProviderMarker: "aof"}, wantErr: "invalid storage provider marker"},
		{name: "oversized marker", provider: "aof", files: map[string]string{kvProviderMarker: "aof\naof\naof\naof\naof\n"}, wantErr: "invalid storage provider marker"},
		{name: "marker is directory", provider: "aof", dirs: []string{kvProviderMarker}, wantErr: "invalid storage provider marker"},
		{name: "vnode is file", provider: "aof", files: map[string]string{"0": "data"}, wantErr: "reading vnode storage directory"},
		{name: "provider path is file", provider: "aof", files: map[string]string{"0/wal": "data"}, wantErr: "expected aof storage directory"},
		{name: "ancient direct aof segments", provider: "aof", files: map[string]string{"0/00000000000000000001": "legacy log"}, wantErr: "unrecognized storage layout"},
		{name: "unknown provider", provider: "typo", wantErr: "unknown kv provider"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			for _, path := range tt.dirs {
				require.NoError(t, os.MkdirAll(filepath.Join(dir, path), 0750))
			}
			for path, data := range tt.files {
				path = filepath.Join(dir, path)
				require.NoError(t, os.MkdirAll(filepath.Dir(path), 0750))
				require.NoError(t, os.WriteFile(path, []byte(data), 0600))
			}
			before := snapshotStorage(t, dir)
			err := preflightKVProvider(tt.provider, dir)
			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
			} else {
				require.NoError(t, err)
				if tt.provider != "memory" {
					before[kvProviderMarker] = tt.provider + "\n"
				}
			}
			require.Equal(t, before, snapshotStorage(t, dir))
		})
	}
}

func TestKVProviderPreflightNewDirectory(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "new")
	require.NoError(t, preflightKVProvider("memory", dir))
	_, err := os.Stat(dir)
	require.ErrorIs(t, err, os.ErrNotExist)
	require.NoError(t, preflightKVProvider("sqlite", dir))
	data, err := os.ReadFile(filepath.Join(dir, kvProviderMarker))
	require.NoError(t, err)
	require.Equal(t, "sqlite\n", string(data))
}

func TestKVProviderConcurrentIdentityClaim(t *testing.T) {
	dir := t.TempDir()
	start := make(chan struct{})
	type result struct {
		provider string
		err      error
	}
	results := make(chan result, 2)
	for _, provider := range []string{"aof", "sqlite"} {
		go func() {
			<-start
			results <- result{provider, preflightKVProvider(provider, dir)}
		}()
	}
	close(start)
	first, second := <-results, <-results
	if first.err != nil {
		first, second = second, first
	}
	require.NoError(t, first.err)
	require.Error(t, second.err)
	data, err := os.ReadFile(filepath.Join(dir, kvProviderMarker))
	require.NoError(t, err)
	require.Equal(t, first.provider+"\n", string(data))
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	require.Len(t, entries, 1, "temporary identity files must be cleaned up")
}

func TestKVProviderPreflightMarkerWriteFailure(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("requires Unix directory permissions without root bypass")
	}
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "0/wal"), 0750))
	before := snapshotStorage(t, dir)
	require.NoError(t, os.Chmod(dir, 0550))
	t.Cleanup(func() { _ = os.Chmod(dir, 0750) })
	require.ErrorContains(t, preflightKVProvider("aof", dir), "creating storage provider marker")
	require.Equal(t, before, snapshotStorage(t, dir))
}

func TestKVProviderPreflightMarkerReadFailure(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("requires Unix file permissions without root bypass")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, kvProviderMarker)
	require.NoError(t, os.WriteFile(path, []byte("aof\n"), 0600))
	require.NoError(t, os.Chmod(path, 0000))
	t.Cleanup(func() { _ = os.Chmod(path, 0600) })
	require.ErrorContains(t, preflightKVProvider("aof", dir), "reading storage provider marker")
	require.NoError(t, os.Chmod(path, 0600))
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, "aof\n", string(data))
}

func TestKVProviderPreflightAllowsTraversalOnlyAncestor(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("requires Unix directory permissions without root bypass")
	}
	parent := t.TempDir()
	dir := filepath.Join(parent, "existing", "new", "data")
	require.NoError(t, os.Mkdir(filepath.Join(parent, "existing"), 0750))
	// The service can traverse this existing ancestor and read its own storage
	// directory, but cannot enumerate the ancestor's other contents.
	require.NoError(t, os.Chmod(parent, 0110))
	t.Cleanup(func() { _ = os.Chmod(parent, 0750) })
	require.NoError(t, preflightKVProvider("aof", dir))
	data, err := os.ReadFile(filepath.Join(dir, kvProviderMarker))
	require.NoError(t, err)
	require.Equal(t, "aof\n", string(data))
	// A normal restart should only need the marker's containing directory.
	require.NoError(t, preflightKVProvider("aof", dir))
}

func TestKVProviderPreflightSyncsNewDirectoryBeforeMarker(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("requires Unix directory permissions without root bypass")
	}
	parent := t.TempDir()
	dir := filepath.Join(parent, "data")
	// Creating an entry is allowed, but opening its parent to make that entry
	// durable is not. Failure must happen before publishing a provider marker.
	require.NoError(t, os.Chmod(parent, 0330))
	t.Cleanup(func() { _ = os.Chmod(parent, 0750) })
	require.ErrorContains(t, preflightKVProvider("aof", dir), "opening directory to sync storage identity")
	_, err := os.Stat(dir)
	require.ErrorIs(t, err, os.ErrNotExist, "remove only the new empty directory after its creation cannot be synced")
	require.NoError(t, os.Chmod(parent, 0750))
	require.NoError(t, preflightKVProvider("aof", dir))
	data, err := os.ReadFile(filepath.Join(dir, kvProviderMarker))
	require.NoError(t, err)
	require.Equal(t, "aof\n", string(data))
}

func TestKVProviderPreflightMarkerSymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation may require elevated privileges on Windows")
	}
	dir := t.TempDir()
	target := filepath.Join(t.TempDir(), "marker")
	require.NoError(t, os.WriteFile(target, []byte("aof\n"), 0600))
	path := filepath.Join(dir, kvProviderMarker)
	require.NoError(t, os.Symlink(target, path))
	require.ErrorContains(t, preflightKVProvider("aof", dir), "invalid storage provider marker")
	link, err := os.Readlink(path)
	require.NoError(t, err)
	require.Equal(t, target, link)
}

func TestServerRejectsConflictingStorageBeforeSetup(t *testing.T) {
	dir := t.TempDir()
	kv, stop, err := getKVProvider(zap.NewNop(), "aof", filepath.Join(dir, "9"), filepath.Join(dir, "cache"))
	require.NoError(t, err)
	err = kv.Put(context.Background(), []byte("existing"), []byte("preserve me"))
	stop()
	require.NoError(t, err)
	before := snapshotStorage(t, dir)

	flags := flag.NewFlagSet(t.Name(), flag.ContinueOnError)
	flags.String("data-dir", dir, "")
	flags.String("kv-provider", "sqlite", "")
	flags.Int("virtual", 1, "")
	// Deliberately unusable later configuration proves preflight runs before
	// further startup work, even for stored vnodes outside the configured count.
	flags.String("proxy-buffer", "invalid", "")
	ctx := cli.NewContext(&cli.App{Metadata: map[string]any{"logger": zap.NewNop()}}, flags, nil)
	err = cmdServer(ctx)
	require.ErrorContains(t, err, "storage preflight")
	require.ErrorContains(t, err, "aof")
	require.ErrorContains(t, err, "sqlite")
	require.Equal(t, before, snapshotStorage(t, dir))
}

func snapshotStorage(t *testing.T, dir string) map[string]string {
	t.Helper()
	files := make(map[string]string)
	err := filepath.WalkDir(dir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		if entry.IsDir() {
			files[rel+"/"] = ""
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		files[rel] = string(data)
		return nil
	})
	require.NoError(t, err)
	return files
}
