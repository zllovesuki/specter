package config

import (
	"bytes"
	"database/sql"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/ncruces/go-sqlite3"
	"github.com/stretchr/testify/require"
)

func openTestStore(t *testing.T, dir string) *Store {
	t.Helper()
	s, err := Open(dir)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, s.Close()) })
	return s
}

func storeSnapshot(t *testing.T, s *Store) []Record {
	t.Helper()
	records, err := s.Snapshot()
	require.NoError(t, err)
	return records
}

func TestStoreInitAndRestart(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "new # %", "config")
	require.Error(t, Init(dir, nil))
	_, err := os.Stat(dir)
	require.ErrorIs(t, err, os.ErrNotExist)
	record := testRecord(t, "future/document", math.MaxUint64, math.MaxUint64, []byte{'\n', 0xff, 0})
	require.NoError(t, Init(dir, []Record{record}))
	require.ErrorIs(t, Init(dir, []Record{record}), os.ErrExist)
	s := openTestStore(t, dir)
	require.Equal(t, []Record{record}, storeSnapshot(t, s))
	require.ErrorIs(t, Init(dir, []Record{record}), os.ErrExist)
	require.NoError(t, s.Close())
	reopened := openTestStore(t, dir)
	require.Equal(t, []Record{record}, storeSnapshot(t, reopened))
	require.NoError(t, reopened.Close())

	emptyDir := t.TempDir()
	empty := openTestStore(t, emptyDir)
	require.Empty(t, storeSnapshot(t, empty))
	require.NoError(t, Init(emptyDir, []Record{record}))
	require.Equal(t, []Record{record}, storeSnapshot(t, empty))
	require.NoError(t, empty.Close())
	require.ErrorIs(t, Init(emptyDir, []Record{record}), os.ErrExist)

	writtenDir := t.TempDir()
	written := openTestStore(t, writtenDir)
	_, err = written.Put("dns", 1, nil, "")
	require.NoError(t, err)
	require.NoError(t, written.Close())
	require.ErrorIs(t, Init(writtenDir, []Record{record}), os.ErrExist)
}

func TestStoreConcurrentInit(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "config")
	type result struct {
		record Record
		err    error
	}
	start := make(chan struct{})
	results := make(chan result, 2)
	for _, record := range []Record{testRecord(t, "first", 1, 1, nil), testRecord(t, "second", 1, 1, nil)} {
		go func() {
			<-start
			results <- result{record: record, err: Init(dir, []Record{record})}
		}()
	}
	close(start)
	a, b := <-results, <-results
	winner := a.record
	if a.err == nil {
		require.ErrorIs(t, b.err, os.ErrExist)
	} else {
		require.ErrorIs(t, a.err, os.ErrExist)
		require.NoError(t, b.err)
		winner = b.record
	}
	require.Equal(t, []Record{winner}, storeSnapshot(t, openTestStore(t, dir)))
	if runtime.GOOS != "windows" {
		info, err := os.Stat(filepath.Join(dir, FileName))
		require.NoError(t, err)
		require.Equal(t, os.FileMode(0600), info.Mode().Perm())
	}
}

func TestStoreCopiesAndLocalVersionGuard(t *testing.T) {
	dir := t.TempDir()
	s := openTestStore(t, dir)
	input := []byte("first")
	first, err := s.Put("dns", 1, input, "")
	require.NoError(t, err)
	token := first.Token()
	input[0], first.Value[0] = 'x', 'x'
	got, found, err := s.Get("dns")
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, []byte("first"), got.Value)
	got.Value[0] = 'x'
	snapshot := storeSnapshot(t, s)
	snapshot[0].Value[0] = 'x'
	got, _, err = s.Get("dns")
	require.NoError(t, err)
	require.Equal(t, []byte("first"), got.Value)
	_, err = s.Put("dns", 1, nil, "")
	require.ErrorIs(t, err, ErrConflict)

	start := make(chan struct{})
	results := make(chan error, 2)
	for i, store := range []*Store{s, openTestStore(t, dir)} {
		go func() {
			<-start
			_, err := store.Put("dns", 1, []byte(fmt.Sprint(i)), token)
			results <- err
		}()
	}
	close(start)
	a, b := <-results, <-results
	if a == nil {
		require.ErrorIs(t, b, ErrConflict)
	} else {
		require.ErrorIs(t, a, ErrConflict)
		require.NoError(t, b)
	}
	got, _, err = s.Get("dns")
	require.NoError(t, err)
	require.Equal(t, uint64(2), got.Version.Revision)
	upgraded, err := s.Put("dns", 2, nil, got.Token())
	require.NoError(t, err)
	_, err = s.Put("dns", 1, nil, upgraded.Token())
	require.ErrorIs(t, err, ErrSchemaRegression)
	written, err := s.Put("empty", 1, []byte{}, "")
	require.NoError(t, err)
	empty, found, err := s.Get("empty")
	require.NoError(t, err)
	require.True(t, found)
	require.Empty(t, empty.Value)
	before, err := EncodeSnapshot([]Record{written})
	require.NoError(t, err)
	after, err := EncodeSnapshot([]Record{empty})
	require.NoError(t, err)
	require.Equal(t, before, after, "empty SQL BLOBs must retain the same wire size")
}

func TestStoreMergeConvergesAcrossOrderAndDuplicates(t *testing.T) {
	stores := []*Store{openTestStore(t, t.TempDir()), openTestStore(t, t.TempDir()), openTestStore(t, t.TempDir())}
	a := testRecord(t, "dns", 1, 7, []byte("a"))
	b := testRecord(t, "dns", 1, 7, []byte("b"))
	future := testRecord(t, "future", 9, 1, []byte{0xff, '\n', 0})
	oldWriter := testRecord(t, "future", 8, math.MaxUint64, []byte("old schema"))
	newer := testRecord(t, "revision", 1, math.MaxUint64, []byte("newer"))
	older := testRecord(t, "revision", 1, math.MaxInt64, []byte("older"))
	for i, records := range [][]Record{{a, future, older}, {b, oldWriter, newer}, {future, a, older}} {
		require.NoError(t, stores[i].Merge(records))
	}
	for _, pair := range [][2]int{{0, 1}, {1, 2}, {2, 0}, {0, 2}, {1, 0}} {
		require.NoError(t, stores[pair[0]].Merge(storeSnapshot(t, stores[pair[1]])))
	}
	winner := a
	if compare(b, a) > 0 {
		winner = b
	}
	for _, s := range stores {
		require.Equal(t, []Record{winner, future, newer}, storeSnapshot(t, s))
		require.NoError(t, s.Merge([]Record{oldWriter, b, a, future, future, older, newer}))
		require.Equal(t, []Record{winner, future, newer}, storeSnapshot(t, s))
	}
	invalid := a
	invalid.Version.Revision = 1
	invalid.Value = []byte("changed without digest")
	err := stores[0].Merge([]Record{testRecord(t, "new", 1, 1, nil), invalid})
	require.ErrorIs(t, err, ErrInvalidRecord)
	require.Equal(t, []Record{winner, future, newer}, storeSnapshot(t, stores[0]))
}

func TestStoreRejectsOversizedUnionAtomically(t *testing.T) {
	dir := t.TempDir()
	s := openTestStore(t, dir)
	value := bytes.Repeat([]byte{'x'}, MaxValueBytes)
	var first, second []Record
	for i := 0; i < 24; i++ {
		record := testRecord(t, fmt.Sprintf("doc-%02d", i), 1, 1, value)
		if i < 12 {
			first = append(first, record)
		} else {
			second = append(second, record)
		}
	}
	require.NoError(t, s.Merge(first))
	err := s.Merge(second)
	require.ErrorIs(t, err, ErrTooLarge)
	require.Equal(t, first, storeSnapshot(t, s))
	require.NoError(t, s.Close())
	require.Equal(t, first, storeSnapshot(t, openTestStore(t, dir)))
}

func TestStoreUnknownCommitRecoversAuthoritativeState(t *testing.T) {
	failure := errors.New("injected commit result")
	for _, committed := range []bool{false, true} {
		t.Run(fmt.Sprint(committed), func(t *testing.T) {
			dir := t.TempDir()
			s := openTestStore(t, dir)
			first, err := s.Put("dns", 1, []byte("old"), "")
			require.NoError(t, err)
			s.commit = func(tx *sql.Tx) error {
				if committed {
					require.NoError(t, tx.Commit())
				}
				return failure
			}
			attempt, err := s.Put("dns", 1, []byte("new"), first.Token())
			require.ErrorIs(t, err, ErrDurabilityUncertain)
			got, found, err := s.Get("dns")
			require.NoError(t, err)
			require.True(t, found)
			want := first
			if committed {
				want = attempt
			}
			require.Equal(t, want, got)
			require.Equal(t, []Record{want}, storeSnapshot(t, s))
			require.NoError(t, s.Merge([]Record{want}), "an unchanged merge must not commit")
			s.commit = (*sql.Tx).Commit
			_, err = s.Put("dns", 1, []byte("later"), want.Token())
			require.NoError(t, err)
			require.NoError(t, s.Close())
			reopened := openTestStore(t, dir)
			got, _, err = reopened.Get("dns")
			require.NoError(t, err)
			require.Equal(t, want.Version.Revision+1, got.Version.Revision)
		})
	}
}

func TestStoreRestartAfterUnknownCommit(t *testing.T) {
	dir := t.TempDir()
	s := openTestStore(t, dir)
	s.commit = func(tx *sql.Tx) error {
		require.NoError(t, tx.Commit())
		return errors.New("acknowledgement lost")
	}
	attempt, err := s.Put("dns", 1, []byte("saved"), "")
	require.ErrorIs(t, err, ErrDurabilityUncertain)
	require.NoError(t, s.Close())
	reopened := openTestStore(t, dir)
	got, _, err := reopened.Get("dns")
	require.NoError(t, err)
	require.Equal(t, attempt, got)
	_, _, err = s.Get("dns")
	require.Error(t, err)
}

func TestStoreRefusesCorruptForeignAndUnsupportedDatabases(t *testing.T) {
	record := testRecord(t, "dns", 1, 1, []byte("initial"))
	for _, body := range [][]byte{[]byte("not a database"), []byte("SQLite format 3\x00invalid")} {
		dir := t.TempDir()
		path := filepath.Join(dir, FileName)
		require.NoError(t, os.WriteFile(path, body, 0600))
		_, err := Open(dir)
		require.Error(t, err)
		require.Error(t, Init(dir, []Record{record}))
		got, err := os.ReadFile(path)
		require.NoError(t, err)
		require.True(t, bytes.Equal(body, got))
	}
	for _, mutation := range []string{
		"PRAGMA application_id=123", "PRAGMA user_version=2",
		"UPDATE documents SET value=x'00'", "UPDATE documents SET version=zeroblob(48)",
	} {
		t.Run(mutation, func(t *testing.T) {
			dir := t.TempDir()
			s := openTestStore(t, dir)
			_, err := s.Put("dns", 1, []byte("saved"), "")
			require.NoError(t, err)
			_, err = s.db.Exec(mutation)
			require.NoError(t, err)
			if strings.HasPrefix(mutation, "UPDATE") {
				replacement := testRecord(t, "dns", 1, 2, []byte("replacement"))
				require.Error(t, s.Merge([]Record{replacement}), "a winning version must not hide existing corruption")
			}
			require.NoError(t, s.Close())
			path := filepath.Join(dir, FileName)
			before, err := os.ReadFile(path)
			require.NoError(t, err)
			_, err = Open(dir)
			require.Error(t, err)
			require.Error(t, Init(dir, []Record{record}))
			after, err := os.ReadFile(path)
			require.NoError(t, err)
			require.Equal(t, before, after)
		})
	}
}

func TestStoreInitializesOnlyPristineSQLite(t *testing.T) {
	record := testRecord(t, "dns", 1, 1, []byte("initial"))
	for _, foreign := range []bool{false, true} {
		t.Run(fmt.Sprint(foreign), func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, FileName)
			conn, err := sqlite3.Open(path)
			require.NoError(t, err)
			require.NoError(t, conn.Exec("VACUUM"))
			if foreign {
				require.NoError(t, conn.Exec("CREATE TABLE unrelated(value BLOB)"))
			}
			require.NoError(t, conn.Close())
			before, err := os.ReadFile(path)
			require.NoError(t, err)
			s, err := Open(dir)
			if foreign {
				require.Error(t, err)
				require.Error(t, Init(dir, []Record{record}))
				after, err := os.ReadFile(path)
				require.NoError(t, err)
				require.Equal(t, before, after)
			} else {
				require.NoError(t, err)
				require.Empty(t, storeSnapshot(t, s))
				require.NoError(t, s.Close())
				require.NoError(t, Init(dir, []Record{record}))
			}
		})
	}
}

func TestStoreBoundsStoredRows(t *testing.T) {
	dir := t.TempDir()
	s := openTestStore(t, dir)
	_, err := s.Put("dns", 1, []byte("saved"), "")
	require.NoError(t, err)
	require.NoError(t, s.Close())

	// Simulate a database written outside this store. The engine's row limit
	// rejects it before scanRecord copies the oversized BLOB into Go memory.
	conn, err := sqlite3.Open(filepath.Join(dir, FileName))
	require.NoError(t, err)
	require.NoError(t, conn.Exec("UPDATE documents SET value=zeroblob(1048576)"))
	require.NoError(t, conn.Close())
	_, err = Open(dir)
	require.ErrorIs(t, err, sqlite3.TOOBIG)
}
