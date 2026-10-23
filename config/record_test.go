package config

import (
	"bytes"
	"encoding/json"
	"math"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func testRecord(t *testing.T, name string, schema, revision uint64, value []byte) Record {
	t.Helper()
	r, err := NewRecord(name, schema, revision, value)
	require.NoError(t, err)
	return r
}

func TestRecordPreservesExactPayload(t *testing.T) {
	value := []byte{'{', '\n', ' ', '}', 0xff, 0}
	record := testRecord(t, "future/config", math.MaxUint64, math.MaxUint64, value)
	encoded, err := json.Marshal(record)
	require.NoError(t, err)
	var decoded Record
	require.NoError(t, json.Unmarshal(encoded, &decoded))
	require.Equal(t, record, decoded)
	require.NoError(t, decoded.Validate())
	require.Equal(t, record.Token(), decoded.Token())
	version, err := decodeVersion(record.Version.bytes())
	require.NoError(t, err)
	require.Equal(t, record.Version, version)
	value[0] = 'x'
	require.Equal(t, byte('{'), record.Value[0])
}

func TestRecordValidation(t *testing.T) {
	valid := testRecord(t, "dns", 1, 1, []byte("opaque"))
	tests := []struct {
		name   string
		change func(*Record)
		err    error
	}{
		{"empty name", func(r *Record) { r.Name = "" }, ErrInvalidRecord},
		{"long name", func(r *Record) { r.Name = strings.Repeat("x", MaxNameBytes+1) }, ErrInvalidRecord},
		{"invalid UTF8", func(r *Record) { r.Name = "\xff" }, ErrInvalidRecord},
		{"name whitespace", func(r *Record) { r.Name = "dns config" }, ErrInvalidRecord},
		{"name control", func(r *Record) { r.Name = "dns\x00" }, ErrInvalidRecord},
		{"zero schema", func(r *Record) { r.Version.Schema = 0 }, ErrInvalidRecord},
		{"zero revision", func(r *Record) { r.Version.Revision = 0 }, ErrInvalidRecord},
		{"oversized value", func(r *Record) { r.Value = make([]byte, MaxValueBytes+1) }, ErrTooLarge},
		{"changed bytes", func(r *Record) { r.Value = []byte("changed") }, ErrInvalidRecord},
		{"bad digest", func(r *Record) { r.Version.Digest[0] ^= 1 }, ErrInvalidRecord},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := valid
			tt.change(&r)
			require.ErrorIs(t, r.Validate(), tt.err)
		})
	}
	_, err := NewRecord(strings.Repeat("x", MaxNameBytes), math.MaxUint64, math.MaxUint64, make([]byte, MaxValueBytes))
	require.NoError(t, err)
}

func TestRecordVersionComparison(t *testing.T) {
	base := testRecord(t, "dns/zöne", 2, 10, []byte("base"))
	newer := testRecord(t, base.Name, 2, 11, []byte("next"))
	newSchema := testRecord(t, base.Name, 3, 1, []byte("next format"))
	oldSchema := testRecord(t, base.Name, 1, math.MaxUint64, []byte("old writer"))
	tie := testRecord(t, base.Name, 2, 10, []byte("competing"))
	for _, tt := range []struct {
		record Record
		want   int
	}{
		{base, 0}, {newer, 1}, {newSchema, 1}, {oldSchema, -1},
		{Record{Name: base.Name}, -1}, {tie, bytes.Compare(tie.Version.Digest[:], base.Version.Digest[:])},
	} {
		got, err := tt.record.CompareVersion(base.Token())
		require.NoError(t, err)
		require.Equal(t, tt.want, got)
	}
	other := testRecord(t, "waf", 2, 10, base.Value)
	zeroSchema, zeroRevision := base, base
	zeroSchema.Version.Schema = 0
	zeroRevision.Version.Revision = 0
	for _, token := range []string{
		"", "bad", strings.Repeat("x", 1025), other.Token(),
		zeroSchema.Token(), zeroRevision.Token(),
		base.Token()[:len(base.Token())-1], base.Token() + "=",
		strings.Replace(base.Token(), ".", ".\n", 1),
		strings.ToUpper(base.Token()), base.Token() + ".extra",
	} {
		_, err := base.CompareVersion(token)
		require.ErrorIs(t, err, ErrInvalidVersion, token)
	}
}

func TestSnapshotCodecBoundsAndValidation(t *testing.T) {
	for _, records := range [][]Record{nil, {}, {testRecord(t, "dns", 1, 1, []byte{0xff, 0, '\n'})}} {
		data, err := EncodeSnapshot(records)
		require.NoError(t, err)
		decoded, err := DecodeSnapshot(data)
		require.NoError(t, err)
		require.ElementsMatch(t, records, decoded)
	}
	for _, data := range []string{
		"", "{",
		`{"format":2,"records":[]}`, `{"format":1,"records":null}`,
		`{"format":1}`, `{"format":1,"records":[]} trailing`,
		`{"format":1,"records":[],"extra":true}`,
		`{"format":1,"records":[{}],"records":[]}`,
		`{"format":1,"records":[],"records":[]}`,
	} {
		_, err := DecodeSnapshot([]byte(data))
		require.Error(t, err)
	}
	_, err := DecodeSnapshot(bytes.Repeat([]byte(" "), MaxSnapshotBytes+1))
	require.ErrorIs(t, err, ErrTooLarge)
	valid := testRecord(t, "dns", 1, 2, []byte("valid"))
	encoded, err := json.Marshal(valid)
	require.NoError(t, err)
	_, err = DecodeSnapshot([]byte(`{"format":1,"records":[` + strings.Repeat(string(encoded)+",", MaxDocuments) + string(encoded) + `]}`))
	require.ErrorIs(t, err, ErrTooLarge)
	invalid := valid
	invalid.Version.Revision = 1
	invalid.Value = []byte("changed bytes")
	data, err := json.Marshal(snapshot{Format: 1, Records: []Record{valid, invalid}})
	require.NoError(t, err)
	_, err = DecodeSnapshot(data)
	require.ErrorIs(t, err, ErrInvalidRecord)
}
