package server

import (
	"fmt"
	"net"
	"testing"

	api "github.com/k20ku/proglog/gen/go/log/v1"
	"github.com/k20ku/proglog/internal/auth"
	"github.com/k20ku/proglog/internal/config"
	"github.com/k20ku/proglog/internal/log"
	"github.com/k20ku/proglog/internal/testdata"
	"github.com/stretchr/testify/require"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

type serviceClientList struct {
	AdminClient  api.LogServiceClient
	NobodyClient api.LogServiceClient
}

func clientSetupTest(t *testing.T, fn func(*Config)) (
	clientList *serviceClientList,
	cfg *Config,
	teardown func(),
) {
	t.Helper()

	l, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err, "listen on port 0")

	// ---- commit log ----
	dir := t.TempDir()
	wlog, err := log.NewLog(dir, log.NewConfig())
	require.NoErrorf(t, err, "new log at %d", dir)
	clog := NewWalCommitLog(wlog)
	require.Implements(t, (*CommitLog)(nil), clog, "log does not implement commitlog")
	authorizer, err := auth.New(testdata.ACLModelFile, testdata.ACLPolicyFile)
	require.NoError(t, err, "failed to new authorozer ACLModelFile=%q, ACLPolicyFile=%q", testdata.ACLModelFile, testdata.ACLPolicyFile)
	aclAuth := NewACLAuthorizer(authorizer)
	cfg = &Config{
		CommitLog:  clog,
		Authorizer: aclAuth,
	}
	if fn != nil {
		fn(cfg)
	}

	// ---- server TLS ----
	serverTLSConfig, err := config.SetupTLSConfig(config.TLSConfig{
		CACertFile:    testdata.CACertFile,
		KeyFile:       testdata.ServerKeyFile,
		CertFile:      testdata.ServerCertFile,
		Server:        true,
		ServerAddress: l.Addr().String(),
	})
	require.NoError(t, err, "client setup tls failed")

	serverCreds := credentials.NewTLS(serverTLSConfig)
	server, err := NewGRPCServer(cfg, grpc.Creds(serverCreds))
	require.NoError(t, err, "new gRPC server")

	// TODO: review scope of context
	eg, _ := errgroup.WithContext(t.Context())
	eg.Go(func() error {
		err := server.Serve(l)
		if err != nil {
			return fmt.Errorf("server serve: %w", err)
		}
		return nil
	})

	// ---- client TLS ----
	newClient := func(
		certPath, keyPath string,
	) (
		*grpc.ClientConn,
		api.LogServiceClient,
		[]grpc.DialOption,
	) {
		clientTLSConfig, err := config.SetupTLSConfig(config.TLSConfig{
			CACertFile: testdata.CACertFile,
			CertFile:   certPath,
			KeyFile:    keyPath,
			Server:     false,
		})
		require.NoError(t, err, "setup client tls failed")
		clientCreds := credentials.NewTLS(clientTLSConfig)
		ops := []grpc.DialOption{grpc.WithTransportCredentials(clientCreds)}
		conn, err := grpc.NewClient(l.Addr().String(), ops...)
		require.NoErrorf(t, err, "new client %s", l.Addr().String())
		client := api.NewLogServiceClient(conn)
		return conn, client, ops
	}

	adminConn, adminClient, _ := newClient(
		testdata.AdminCertFile,
		testdata.AdminKeyFile,
	)

	nobodyConn, nobodyClient, _ := newClient(
		testdata.NobodyCertFile,
		testdata.NobodyKeyFile,
	)
	return &serviceClientList{
			AdminClient:  adminClient,
			NobodyClient: nobodyClient,
		}, cfg, func() {
			server.Stop()
			_ = adminConn.Close()
			_ = nobodyConn.Close()
			_ = l.Close()
			_ = wlog.Close()
		}
}
