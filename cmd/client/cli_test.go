package client

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tunclient "go.miragespace.co/specter/tun/client"

	"github.com/stretchr/testify/require"
	"github.com/urfave/cli/v3"
)

func TestClientCLIInheritsFlagsContextAndMetadata(t *testing.T) {
	type contextKey struct{}
	ctx := context.WithValue(t.Context(), contextKey{}, "request-context")
	cmd := Generate()
	called := false
	for _, subcommand := range cmd.Commands {
		if subcommand.Name == "connect" {
			subcommand.Action = func(ctx context.Context, command *cli.Command) error {
				called = true
				require.True(t, command.Bool("insecure"))
				require.Equal(t, "target.example.com", command.Args().First())
				require.Equal(t, "request-context", ctx.Value(contextKey{}))
				require.Equal(t, "root-metadata", command.Root().Metadata["test"])
				return nil
			}
		}
	}
	app := &cli.Command{
		Name:     "specter",
		Commands: []*cli.Command{cmd},
		Metadata: map[string]any{"test": "root-metadata"},
	}
	require.NoError(t, app.Run(ctx, []string{"specter", "client", "--insecure", "connect", "target.example.com"}))
	require.True(t, called)
}

func TestConfigExampleUsesRootWriter(t *testing.T) {
	var output bytes.Buffer
	app := &cli.Command{Name: "specter", Writer: &output, Commands: []*cli.Command{Generate()}}
	require.NoError(t, app.Run(t.Context(), []string{"specter", "client", "config-example"}))
	require.Equal(t, tunclient.ExampleConfigYAML(), output.String())
}

func TestLightweightFlags(t *testing.T) {
	for _, name := range []string{"expose", "serve", "token"} {
		t.Run(name, func(t *testing.T) {
			cmd := Generate()
			called := false
			args := []string{"specter", "client", "--insecure", name}
			for _, sub := range cmd.Commands {
				if sub.Name != name {
					continue
				}
				if name == "token" {
					sub = sub.Commands[1]
					args = append(args, "list", "--config", "owner.yaml")
				} else {
					args = append(args, "--apex", "example.com", "http://localhost:8080")
				}
				sub.Action = func(_ context.Context, cmd *cli.Command) error {
					called = true
					require.True(t, cmd.Bool("insecure"))
					return nil
				}
			}
			app := &cli.Command{
				Name:     "specter",
				Commands: []*cli.Command{cmd},
			}
			require.NoError(t, app.Run(t.Context(), args))
			require.True(t, called)
		})
	}
	for _, name := range []string{"expose", "serve"} {
		app := &cli.Command{
			Name:     "specter",
			Commands: []*cli.Command{Generate()},
		}
		require.ErrorContains(t, app.Run(t.Context(), []string{"specter", "client", name, "--apex", "example.com", "--server", "a", "http://localhost"}), "flag provided but not defined: -server")
	}
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
