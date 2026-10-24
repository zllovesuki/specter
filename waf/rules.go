package waf

import (
	"archive/zip"
	"bytes"
	_ "embed"
	"fmt"
	"io"
	"io/fs"
	"strings"

	openpgp "github.com/ProtonMail/go-crypto/openpgp/v2"
	lts "github.com/corazawaf/coraza-coreruleset/lts/v4"
)

const (
	maxArchiveBytes   = 8 << 20
	maxSignatureBytes = 64 << 10
	maxExpandedBytes  = 32 << 20
	maxRuleFileBytes  = 4 << 20
	maxArchiveFiles   = 512
	crsFingerprint    = "36006F0E0BA167832158821138EEACA1AB8A6E72"
)

// The release key is authenticated by the fingerprint published in upstream's
// SECURITY.md. New trust roots ship with Specter, never through release discovery.
//
//go:embed crs-security.asc
var crsPublicKey string

var bundledVersion = strings.TrimPrefix(lts.Version, "v")

type ruleRelease struct {
	version   string
	rules     fs.FS
	archive   []byte
	signature []byte
}

func trustedCRSKeys() (openpgp.EntityList, error) {
	keys, err := openpgp.ReadArmoredKeyRing(strings.NewReader(crsPublicKey))
	if err != nil {
		return nil, err
	}
	if len(keys) != 1 || fmt.Sprintf("%X", keys[0].PrimaryKey.Fingerprint) != crsFingerprint {
		return nil, fmt.Errorf("embedded CRS signing key has an unexpected fingerprint")
	}
	return keys, nil
}

func verifyRelease(version string, archive, signature []byte, keys openpgp.EntityList) (*ruleRelease, error) {
	if !validReleaseVersion(version) || len(archive) > maxArchiveBytes || len(signature) > maxSignatureBytes {
		return nil, fmt.Errorf("invalid CRS release or archive size")
	}
	if _, _, err := openpgp.VerifyArmoredDetachedSignature(keys, bytes.NewReader(archive), bytes.NewReader(signature), nil); err != nil {
		return nil, fmt.Errorf("verify CRS %s signature: %w", version, err)
	}
	rules, err := openRulesArchive(version, archive)
	if err != nil {
		return nil, err
	}
	return &ruleRelease{version: version, rules: rules, archive: archive, signature: signature}, nil
}

// A ZIP reader owns the archive bytes and implements fs.FS. Engines therefore
// remain usable after their on-disk cache files have been retired.
func openRulesArchive(version string, archive []byte) (fs.FS, error) {
	r, err := zip.NewReader(bytes.NewReader(archive), int64(len(archive)))
	if err != nil {
		return nil, fmt.Errorf("open CRS archive: %w", err)
	}
	if len(r.File) > maxArchiveFiles {
		return nil, fmt.Errorf("CRS archive contains too many files")
	}
	root := "coreruleset-" + version
	seen := make(map[string]bool, len(r.File))
	var expanded uint64
	for _, f := range r.File {
		name := strings.TrimSuffix(f.Name, "/")
		if !fs.ValidPath(name) || strings.Contains(name, "\\") || (name != root && !strings.HasPrefix(name, root+"/")) || seen[name] {
			return nil, fmt.Errorf("invalid or duplicate CRS archive path %q", f.Name)
		}
		seen[name] = true
		if f.FileInfo().IsDir() {
			continue
		}
		if !f.Mode().IsRegular() || f.UncompressedSize64 > maxRuleFileBytes {
			return nil, fmt.Errorf("invalid CRS archive file %q", f.Name)
		}
		expanded += f.UncompressedSize64
		if expanded > maxExpandedBytes {
			return nil, fmt.Errorf("expanded CRS archive is too large")
		}
		body, err := f.Open()
		if err != nil {
			return nil, err
		}
		n, readErr := io.Copy(io.Discard, io.LimitReader(body, maxRuleFileBytes+1))
		closeErr := body.Close()
		if readErr != nil || closeErr != nil || n != int64(f.UncompressedSize64) {
			return nil, fmt.Errorf("invalid CRS archive content %q", f.Name)
		}
	}
	for _, required := range []string{"crs-setup.conf.example", "rules/REQUEST-901-INITIALIZATION.conf", "rules/REQUEST-949-BLOCKING-EVALUATION.conf"} {
		if !seen[root+"/"+required] {
			return nil, fmt.Errorf("CRS archive is missing %s", required)
		}
	}
	return fs.Sub(r, root)
}

// Adapt the upstream embedded aliases once to the same layout as a downloaded
// release. The embedded module is authenticated with the Specter binary.
func bundledRules() (fs.FS, error) {
	var data bytes.Buffer
	w := zip.NewWriter(&data)
	root := "coreruleset-" + bundledVersion + "/"
	copyFile := func(source, target string) error {
		contents, err := fs.ReadFile(lts.FS, source)
		if err != nil {
			return err
		}
		file, err := w.CreateHeader(&zip.FileHeader{Name: root + target, Method: zip.Store})
		if err != nil {
			return err
		}
		_, err = file.Write(contents)
		return err
	}
	if err := copyFile("@crs-setup-lts.conf.example", "crs-setup.conf.example"); err != nil {
		return nil, err
	}
	if err := fs.WalkDir(lts.FS, "@owasp_crs_lts", func(name string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		return copyFile(name, "rules/"+strings.TrimPrefix(name, "@owasp_crs_lts/"))
	}); err != nil {
		return nil, err
	}
	if err := w.Close(); err != nil {
		return nil, err
	}
	return openRulesArchive(bundledVersion, data.Bytes())
}
