package waf

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"go.miragespace.co/specter/util"
)

type cacheState struct {
	Active   string `json:"active"`
	Previous string `json:"previous,omitempty"`
}

func readBoundedFile(name string, limit int64) ([]byte, error) {
	f, err := os.Open(name)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, limit+1))
	if err == nil && int64(len(data)) > limit {
		err = fmt.Errorf("cached WAF file exceeds size limit")
	}
	return data, err
}

func readCacheState(dir string) (cacheState, error) {
	var state cacheState
	data, err := readBoundedFile(filepath.Join(dir, "state.json"), 4096)
	if errors.Is(err, os.ErrNotExist) {
		return state, nil
	}
	if err != nil {
		return state, err
	}
	if err := json.Unmarshal(data, &state); err != nil {
		return cacheState{}, err
	}
	if !validReleaseVersion(state.Active) || (state.Previous != "" && !validReleaseVersion(state.Previous)) {
		return cacheState{}, fmt.Errorf("invalid cached WAF release state")
	}
	return state, nil
}

func (s *ruleSource) cached(dir, version string) (*ruleRelease, error) {
	if !validReleaseVersion(version) {
		return nil, fmt.Errorf("invalid cached WAF release version")
	}
	archive, err := readBoundedFile(filepath.Join(dir, version+".zip"), maxArchiveBytes)
	if err != nil {
		return nil, err
	}
	signature, err := readBoundedFile(filepath.Join(dir, version+".zip.asc"), maxSignatureBytes)
	if err != nil {
		return nil, err
	}
	return verifyRelease(version, archive, signature, s.keys)
}

func saveRelease(dir string, release *ruleRelease) error {
	if err := writeCacheFile(dir, release.version+".zip", release.archive); err != nil {
		return err
	}
	return writeCacheFile(dir, release.version+".zip.asc", release.signature)
}

func saveCacheState(dir string, state cacheState) error {
	data, err := json.Marshal(state)
	if err != nil {
		return err
	}
	return writeCacheFile(dir, "state.json", data)
}

func writeCacheFile(dir, name string, data []byte) error {
	if err := util.MkdirAllDurable(dir, 0700); err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, ".waf-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	defer f.Close()
	if _, err := f.Write(data); err != nil {
		return err
	}
	if err := f.Sync(); err != nil {
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	if err := os.Rename(f.Name(), filepath.Join(dir, name)); err != nil {
		return err
	}
	if runtime.GOOS == "windows" {
		return nil
	}
	d, err := os.Open(dir)
	if err != nil {
		return err
	}
	return errors.Join(d.Sync(), d.Close())
}

func pruneCache(dir string, state cacheState) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, entry := range entries {
		version := strings.TrimSuffix(strings.TrimSuffix(entry.Name(), ".asc"), ".zip")
		if entry.IsDir() || !validReleaseVersion(version) || version == state.Active || version == state.Previous {
			continue
		}
		if entry.Name() == version+".zip" || entry.Name() == version+".zip.asc" {
			_ = os.Remove(filepath.Join(dir, entry.Name()))
		}
	}
}
