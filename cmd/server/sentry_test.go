package server

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/TheZeroSlave/zapsentry"
	"github.com/getsentry/sentry-go"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

type recordingSentryTransport struct {
	events []*sentry.Event
}

func (t *recordingSentryTransport) Configure(sentry.ClientOptions) {}
func (t *recordingSentryTransport) Flush(time.Duration) bool       { return true }
func (t *recordingSentryTransport) FlushWithContext(context.Context) bool {
	return true
}
func (t *recordingSentryTransport) Close() {}
func (t *recordingSentryTransport) SendEvent(event *sentry.Event) {
	t.events = append(t.events, event)
}

func TestSentryLoggerPreservesEventsAndBreadcrumbs(t *testing.T) {
	transport := &recordingSentryTransport{}
	client, err := sentry.NewClient(sentry.ClientOptions{
		Dsn:       "https://public@example.com/1",
		Release:   "test-release",
		Transport: transport,
	})
	require.NoError(t, err)
	t.Cleanup(client.Close)

	logger := modifyToSentryLogger(zap.NewNop(), client).With(zapsentry.NewScope())
	logger.Info("connected")
	logger.Warn("retrying", zap.String("peer", "test-peer"))
	logger.Error("disconnected", zap.Error(errors.New("connection lost")))
	require.NoError(t, logger.Sync())

	require.Len(t, transport.events, 2, "info logs should only become breadcrumbs")
	warning, failure := transport.events[0], transport.events[1]
	require.Equal(t, sentry.LevelWarning, warning.Level)
	require.Equal(t, "retrying", warning.Message)
	require.Equal(t, "test-release", warning.Release)
	require.Equal(t, "test-peer", warning.Contexts["Extra"]["peer"])
	require.NotEmpty(t, warning.Breadcrumbs)
	require.Equal(t, "connected", warning.Breadcrumbs[0].Message)
	require.Equal(t, sentry.LevelError, failure.Level)
	require.Equal(t, "disconnected", failure.Message)
	require.Len(t, failure.Exception, 1)
	require.Equal(t, "connection lost", failure.Exception[0].Value)
}
