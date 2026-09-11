package client

import (
	"bytes"
	"context"
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
