package server

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/urfave/cli/v3"
	"go.uber.org/zap"
)

func TestServerCLIFlagSources(t *testing.T) {
	for _, tc := range []struct {
		name            string
		args            []string
		want            []string
		explicitOptions bool
	}{
		{
			name: "environment list",
			want: []string{"127.0.0.1:1113", "[::1]:1113"},
		},
		{
			name:            "repeated flags override environment",
			args:            []string{"--listen", "127.0.0.1:2113", "--listen-addr", "[::1]:2113", "--listen-rpc=", "--listen-http=0", "--cert-env=false", "--join=", "--sentry="},
			want:            []string{"127.0.0.1:2113", "[::1]:2113"},
			explicitOptions: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("LISTEN_ADDR", "127.0.0.1:1113,[::1]:1113")
			t.Setenv("KV_PROVIDER", "sqlite")
			t.Setenv("CERT_CA", "encoded-ca")
			dataDir, certDir := t.TempDir(), t.TempDir()
			cmd := Generate()
			called := false
			cmd.Action = func(_ context.Context, cmd *cli.Command) error {
				called = true
				options := serverOptions(cmd, zap.NewNop())
				require.Equal(t, tc.want, options.ListenAddresses)
				require.Equal(t, "aof", options.KVProvider)
				require.Equal(t, dataDir, options.DataDir)
				require.Equal(t, certDir, *options.CertDirectory)
				require.Equal(t, "127.0.0.1:1113", *options.AdvertiseAddress)
				require.True(t, options.ACMEEnabled)
				require.Equal(t, "admin@example.com", options.ACMEEmail)
				require.Equal(t, "hostedacme.com", options.ACMEZone)
				if tc.explicitOptions {
					require.NotNil(t, options.RPCListen)
					require.Empty(t, *options.RPCListen)
					require.NotNil(t, options.HTTPPort)
					require.Zero(t, *options.HTTPPort)
					require.NotNil(t, options.JoinAddress)
					require.Empty(t, *options.JoinAddress)
					require.NotNil(t, options.SentryDSN)
					require.Empty(t, *options.SentryDSN)
					require.NotNil(t, options.CertEnvironment)
					require.Equal(t, "encoded-ca", options.CertEnvironment.CA)
				} else {
					require.Nil(t, options.RPCListen)
					require.Nil(t, options.HTTPPort)
					require.Nil(t, options.CertEnvironment)
				}
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
