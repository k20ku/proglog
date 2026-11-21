package testutil

import (
	"testing"

	"github.com/k20ku/proglog/internal/log"
	"github.com/k20ku/proglog/internal/server"
	"github.com/stretchr/testify/require"
)

func NewWalCommitLog(t *testing.T) (
	clog server.CommitLog,
	closeLog func(),
) {
	t.Helper()

	dir := t.TempDir()
	wlog, err := log.NewLog(dir, log.NewConfig())
	require.NoErrorf(t, err, "new log at %d", dir)
	clog = server.NewWalCommitLog(wlog)
	return clog, func() { _ = wlog.Close() }
}
