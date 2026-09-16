package replicate

import (
	"fmt"
	"log/slog"
	"testing"

	"github.com/k20ku/proglog/internal/server"
	"github.com/k20ku/proglog/internal/testutil"
	"golang.org/x/sync/errgroup"
)

func TestReplicator(t *testing.T) {
	t.SkipNow()
	ctx := t.Context()
	var configs []*server.Config
	for i := range 2 {
		clog, closeLog := testutil.NewWalCommitLog(t)
		t.Cleanup(closeLog)
		aclAuth := testutil.NewACLAuthorizer(t)
		cfg := &server.Config{
			Logger:     slog.Default().WithGroup(fmt.Sprintf("server%d", i)),
			CommitLog:  clog,
			Authorizer: aclAuth,
		}
		configs = append(configs, cfg)
	}

	eg, _ := errgroup.WithContext(ctx)
	for _, cfg := range configs {
		eg.Go(func() error {
			server := testutil.NewServer(t, "", cfg)
			return server.Serve(nil)
		})
	}
}

func newReplicator(t *testing.T) *Replicator {
	return &Replicator{}
}
