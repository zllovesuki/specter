package config

import (
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go.miragespace.co/specter/acme"
	"go.miragespace.co/specter/config"

	"github.com/stretchr/testify/require"
	"github.com/urfave/cli/v3"
)

const initialDNSValue = `{"zone":"EXAMPLE.COM","nameservers":[{"host":"NS.EXAMPLE.COM","addresses":["2001:db8:0::1","192.0.2.1"]}]}`
const initialDNS = `{"name":"dns","schema":1,"value":` + initialDNSValue + `}`

func runInit(t *testing.T, dataDir, input string) (string, error) {
	t.Helper()
	inputPath := filepath.Join(t.TempDir(), "initial.json")
	require.NoError(t, os.WriteFile(inputPath, []byte(input), 0600))
	var output bytes.Buffer
	app := &cli.Command{
		Name:      "specter",
		Writer:    &output,
		ErrWriter: &output,
		Commands:  []*cli.Command{Generate()},
	}
	err := app.Run(t.Context(), []string{"specter", "config", "init", "--data-dir", dataDir, "--file", inputPath})
	return output.String(), err
}

func TestInitCreatesDurableDocumentsWithoutPrintingValues(t *testing.T) {
	dataDir := t.TempDir()
	input := `[` + initialDNS + `,{"name":"future-policy","schema":7,"value":{"secret":"not-for-output"}}]`
	output, err := runInit(t, dataDir, input)
	require.NoError(t, err)
	require.Contains(t, output, "Initialized 2 shared configuration document(s)")
	require.Contains(t, output, filepath.Join(dataDir, "config", config.FileName))
	require.NotContains(t, output, "not-for-output")
	require.NotContains(t, output, "192.0.2.1")

	store, err := config.Open(filepath.Join(dataDir, "config"))
	require.NoError(t, err)
	defer store.Close()
	record, ok, err := store.Get("dns")
	require.NoError(t, err)
	require.True(t, ok)
	require.EqualValues(t, 1, record.Version.Revision)
	require.NoError(t, record.Validate())
	require.Equal(t, initialDNSValue, string(record.Value))
	dnsConfig, err := acme.DecodeDNSConfig(record, "example.com")
	require.NoError(t, err)
	require.Equal(t, "example.com.", dnsConfig.Zone)
	require.Equal(t, []acme.DNSNameserver{{Host: "ns.example.com.", Addresses: []string{"192.0.2.1", "2001:db8::1"}}}, dnsConfig.Nameservers)

	record, ok, err = store.Get("future-policy")
	require.NoError(t, err)
	require.True(t, ok)
	require.EqualValues(t, 7, record.Version.Schema)
	require.JSONEq(t, `{"secret":"not-for-output"}`, string(record.Value))
}

func TestInitNeverReplacesExistingConfiguration(t *testing.T) {
	dataDir := t.TempDir()
	_, err := runInit(t, dataDir, `[`+initialDNS+`]`)
	require.NoError(t, err)
	store, err := config.Open(filepath.Join(dataDir, "config"))
	require.NoError(t, err)
	initialRecord, exists, err := store.Get(acme.DNSConfigName)
	require.NoError(t, err)
	require.True(t, exists)
	require.NoError(t, store.Close())

	output, err := runInit(t, dataDir, `[{"name":"replacement","schema":1,"value":{}}]`)
	require.ErrorIs(t, err, fs.ErrExist)
	require.Empty(t, output)
	store, err = config.Open(filepath.Join(dataDir, "config"))
	require.NoError(t, err)
	records, err := store.Snapshot()
	require.NoError(t, err)
	require.Equal(t, []config.Record{initialRecord}, records)
	require.NoError(t, store.Close())
}

func TestInitRejectsInvalidInputBeforeCreatingSharedData(t *testing.T) {
	documents := make([]string, config.MaxDocuments+1)
	for i := range documents {
		documents[i] = fmt.Sprintf(`{"name":"doc-%d","schema":1,"value":{}}`, i)
	}
	for _, test := range []struct {
		name    string
		input   string
		message string
	}{
		{name: "empty array", input: `[]`},
		{name: "null", input: `null`},
		{name: "not an array", input: `{}`},
		{name: "trailing value", input: `[` + initialDNS + `] {}`},
		{name: "duplicate document", input: `[` + initialDNS + `,` + initialDNS + `]`},
		{name: "explicit revision", input: `[{"name":"future-policy","schema":1,"revision":99,"value":{}}]`},
		{name: "missing value", input: `[{"name":"future-policy","schema":1}]`},
		{name: "zero schema", input: `[{"name":"future-policy","schema":0,"value":{}}]`},
		{name: "unsupported DNS schema", input: `[{"name":"dns","schema":2,"value":{}}]`},
		{name: "later invalid DNS", input: `[{"name":"future-policy","schema":1,"value":{}},{"name":"dns","schema":1,"value":{"zone":"example.com","nameservers":[]}}]`},
		{name: "oversized document", input: `[{"name":"future-policy","schema":1,"value":"` + strings.Repeat("x", config.MaxValueBytes+1) + `"}]`},
		{name: "oversized file", input: strings.Repeat(" ", config.MaxSnapshotBytes) + `[` + initialDNS + `]`, message: "initial configuration exceeds"},
		{name: "too many documents", input: `[` + strings.Join(documents, ",") + `]`, message: "initial configuration must contain between"},
	} {
		t.Run(test.name, func(t *testing.T) {
			dataDir := t.TempDir()
			output, err := runInit(t, dataDir, test.input)
			require.Error(t, err)
			if test.message != "" {
				require.ErrorContains(t, err, test.message)
			}
			require.Empty(t, output)
			_, err = os.Stat(filepath.Join(dataDir, "config"))
			require.ErrorIs(t, err, fs.ErrNotExist)
		})
	}
}
