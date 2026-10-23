package client

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tunclient "go.miragespace.co/specter/tun/client"

	"github.com/stretchr/testify/require"
	"github.com/urfave/cli/v3"
)

func TestConfigExampleUsesRootWriter(t *testing.T) {
	var output bytes.Buffer
	app := &cli.Command{Name: "specter", Writer: &output, Commands: []*cli.Command{Generate()}}
	require.NoError(t, app.Run(t.Context(), []string{"specter", "client", "config-example"}))
	require.Equal(t, tunclient.ExampleConfigYAML(), output.String())
}

func TestLightweightRejectsOwnerFlags(t *testing.T) {
	for _, name := range []string{"expose", "serve"} {
		app := &cli.Command{
			Name:     "specter",
			Commands: []*cli.Command{Generate()},
		}
		require.ErrorContains(t, app.Run(t.Context(), []string{"specter", "client", name, "--apex", "example.com", "--server", "a", "http://localhost"}), "flag provided but not defined: -server")
	}
}

func TestLoadTunnelToken(t *testing.T) {
	for _, tc := range []struct {
		name    string
		file    string
		env     string
		fileSet bool
		envSet  bool
		valid   bool
	}{
		{
			name:    "file",
			file:    " secret\n",
			fileSet: true,
			valid:   true,
		},
		{
			name:   "env",
			env:    " secret\n",
			envSet: true,
			valid:  true,
		},
		{
			name:    "both",
			file:    "secret",
			env:     "secret",
			fileSet: true,
			envSet:  true,
		},
		{name: "neither"},
		{
			name:    "large file",
			file:    strings.Repeat("x", 4097),
			fileSet: true,
		},
		{
			name:   "large env",
			env:    strings.Repeat("x", 4097),
			envSet: true,
		},
		{
			name:   "empty",
			env:    " \n",
			envSet: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := ""
			if tc.fileSet {
				path = filepath.Join(t.TempDir(), "token")
				require.NoError(t, os.WriteFile(path, []byte(tc.file), 0600))
			}
			token, err := loadTunnelToken(path, tc.env, tc.envSet)
			if tc.valid {
				require.NoError(t, err)
				require.Equal(t, "secret", token)
			} else {
				require.Error(t, err)
			}
		})
	}
}
