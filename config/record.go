// Package config stores complete, versioned configuration documents on each
// physical server, independently of Chord key ownership. Consumers own document
// schema validation and activation.
package config

import (
	"bytes"
	"cmp"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"
)

const (
	MaxNameBytes     = 512
	MaxValueBytes    = 256 << 10
	MaxDocuments     = 4096
	MaxSnapshotBytes = 8 << 20
	versionBytes     = 48
)

var (
	ErrInvalidRecord       = errors.New("invalid configuration record")
	ErrInvalidVersion      = errors.New("invalid configuration version")
	ErrTooLarge            = errors.New("configuration exceeds size limit")
	ErrConflict            = errors.New("configuration version changed")
	ErrSchemaRegression    = errors.New("configuration schema cannot decrease")
	ErrDurabilityUncertain = errors.New("configuration save outcome is uncertain")
)

// Version orders copies of a document without interpreting its value. Schema
// takes precedence so an older writer cannot defeat a newer document format.
// The digest breaks equal-revision ties and detects changed document bytes.
type Version struct {
	Schema   uint64
	Revision uint64
	Digest   [sha256.Size]byte
}

func (v Version) bytes() []byte {
	data := make([]byte, versionBytes)
	binary.BigEndian.PutUint64(data, v.Schema)
	binary.BigEndian.PutUint64(data[8:], v.Revision)
	copy(data[16:], v.Digest[:])
	return data
}

func decodeVersion(data []byte) (Version, error) {
	if len(data) != versionBytes {
		return Version{}, ErrInvalidVersion
	}
	v := Version{Schema: binary.BigEndian.Uint64(data), Revision: binary.BigEndian.Uint64(data[8:])}
	copy(v.Digest[:], data[16:])
	if v.Schema == 0 || v.Revision == 0 {
		return Version{}, ErrInvalidVersion
	}
	return v, nil
}

// MarshalText gives JSON the same bounded, opaque version used by storage.
func (v Version) MarshalText() ([]byte, error) {
	return []byte(base64.RawURLEncoding.EncodeToString(v.bytes())), nil
}

func (v *Version) UnmarshalText(data []byte) error {
	if len(data) != base64.RawURLEncoding.EncodedLen(versionBytes) {
		return ErrInvalidVersion
	}
	decoded, err := base64.RawURLEncoding.Strict().DecodeString(string(data))
	if err != nil {
		return ErrInvalidVersion
	}
	*v, err = decodeVersion(decoded)
	return err
}

// Record is one complete document. JSON encodes Value as base64 so relays
// preserve exact bytes, including schemas they do not understand.
type Record struct {
	Name    string  `json:"name"`
	Version Version `json:"version"`
	Value   []byte  `json:"value,omitempty"`
}

func NewRecord(name string, schema, revision uint64, value []byte) (Record, error) {
	if len(value) > MaxValueBytes {
		return Record{}, ErrTooLarge
	}
	r := Record{Name: name, Version: Version{Schema: schema, Revision: revision, Digest: sha256.Sum256(value)}, Value: bytes.Clone(value)}
	return r, r.Validate()
}

func (r Record) Validate() error {
	if !validName(r.Name) {
		return fmt.Errorf("%w: invalid document name", ErrInvalidRecord)
	}
	if r.Version.Schema == 0 || r.Version.Revision == 0 {
		return fmt.Errorf("%w: schema and revision must be positive", ErrInvalidRecord)
	}
	if len(r.Value) > MaxValueBytes {
		return fmt.Errorf("%w: document %q", ErrTooLarge, r.Name)
	}
	if r.Version.Digest != sha256.Sum256(r.Value) {
		return fmt.Errorf("%w: document %q digest mismatch", ErrInvalidRecord, r.Name)
	}
	return nil
}

// Token is a name-scoped local editing guard, not a globally serialized revision
// or a replication acknowledgement.
func (r Record) Token() string {
	if r.Name == "" {
		return ""
	}
	version, _ := r.Version.MarshalText()
	return "v1." + base64.RawURLEncoding.EncodeToString([]byte(r.Name)) + "." + string(version)
}

// CompareVersion compares against a token for the same document. The order is
// deterministic; it does not imply that concurrent edits were combined.
func (r Record) CompareVersion(token string) (int, error) {
	if len(token) > 1024 {
		return 0, ErrInvalidVersion
	}
	parts := strings.Split(token, ".")
	if len(parts) != 3 || parts[0] != "v1" {
		return 0, ErrInvalidVersion
	}
	name, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil || !validName(string(name)) || string(name) != r.Name {
		return 0, ErrInvalidVersion
	}
	other := Record{Name: string(name)}
	if err := other.Version.UnmarshalText([]byte(parts[2])); err != nil {
		return 0, ErrInvalidVersion
	}
	if other.Token() != token {
		return 0, ErrInvalidVersion
	}
	return compare(r, other), nil
}

func validName(name string) bool {
	if len(name) == 0 || len(name) > MaxNameBytes || !utf8.ValidString(name) {
		return false
	}
	for _, c := range name {
		if unicode.IsControl(c) || unicode.IsSpace(c) {
			return false
		}
	}
	return true
}

func compare(a, b Record) int {
	if c := cmp.Compare(a.Version.Schema, b.Version.Schema); c != 0 {
		return c
	}
	if c := cmp.Compare(a.Version.Revision, b.Version.Revision); c != 0 {
		return c
	}
	return bytes.Compare(a.Version.Digest[:], b.Version.Digest[:])
}
