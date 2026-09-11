package server

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/urfave/cli/v3"
)

func TestServerCLIFlagSources(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
		want []string
	}{
		{
			name: "environment list",
			want: []string{"127.0.0.1:1113", "[::1]:1113"},
		},
		{
			name: "repeated flags override environment",
			args: []string{"--listen", "127.0.0.1:2113", "--listen-addr", "[::1]:2113"},
			want: []string{"127.0.0.1:2113", "[::1]:2113"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("LISTEN_ADDR", "127.0.0.1:1113,[::1]:1113")
			t.Setenv("KV_PROVIDER", "sqlite")
			dataDir, certDir := t.TempDir(), t.TempDir()
			cmd := Generate()
			called := false
			cmd.Action = func(_ context.Context, cmd *cli.Command) error {
				called = true
				require.Equal(t, tc.want, cmd.StringSlice("listen-addr"))
				require.Equal(t, "aof", cmd.String("kv-provider"))
				require.Equal(t, dataDir, cmd.String("data-dir"))
				require.Equal(t, certDir, cmd.String("cert-dir"))
				require.Equal(t, "admin@example.com", cmd.String("acme_email"))
				require.Equal(t, "hostedacme.com", cmd.String("acme_zone"))
				return nil
			}
			app := &cli.Command{Name: "specter", Commands: []*cli.Command{cmd}}
			args := []string{"specter", "server", "--data", dataDir, "--cert", certDir,
				"--advertise", "127.0.0.1:1113", "--apex", "example.com", "--kv-provider", "aof",
				"--acme", "acme://admin@example.com@hostedacme.com"}
			require.NoError(t, app.Run(t.Context(), append(args, tc.args...)))
			require.True(t, called)
		})
	}
}
