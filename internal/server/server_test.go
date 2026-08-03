package server

import (
	"context"
	"fmt"
	"net"
	"testing"

	api "github.com/k20ku/proglog/gen/go/log/v1"
	"github.com/k20ku/proglog/internal/config"
	"github.com/k20ku/proglog/internal/log"
	"github.com/k20ku/proglog/internal/testdata"
	"github.com/stretchr/testify/require"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/status"
)

type serviceClientList struct {
	AdminClient  api.LogServiceClient
	NobodyClient api.LogServiceClient
}

func TestServer(t *testing.T) {
	senarios := map[string]func(
		t *testing.T,
		clientList *serviceClientList,
		config *Config,
	){
		"produce/consume a message to/from the log succeeds": testProduceConsume,
		"consume past log boundary fails":                    testConsumePastBoundary,
		"produce/consume stream succeeds":                    testProduceConsumeStream,
	}

	for senario, fn := range senarios {
		t.Run(senario, func(t *testing.T) {
			clientList, config, teardown := setupTest(t, nil)
			t.Cleanup(teardown)
			fn(t, clientList, config)
		})
	}
}

func setupTest(t *testing.T, fn func(*Config)) (
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
	cfg = &Config{
		CommitLog: clog,
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
	newClient := func(certPath, keyPath string) (
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
		testdata.AdminCertFile,
		testdata.AdminKeyFile,
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

func testProduceConsume(t *testing.T, clientList *serviceClientList, config *Config) {
	ctx := context.Background()

	want := &api.Record{
		Value: []byte("Hello Proglog"),
	}

	client := clientList.AdminClient
	produceRsp, err := client.Produce(
		ctx,
		&api.ProduceRequest{
			Record: want,
		},
	)
	require.NoError(t, err, "client produce")

	consume, err := client.Consume(ctx, &api.ConsumeRequest{
		Offset: produceRsp.Offset,
	})
	require.NoError(t, err, "client consume")
	require.Equal(t, want.Value, consume.Record.Value)
	require.Equal(t, want.Offset, consume.Record.Offset)
}

func testConsumePastBoundary(
	t *testing.T,
	clientList *serviceClientList,
	_ *Config,
) {
	ctx := context.Background()

	client := clientList.AdminClient
	produce, err := client.Produce(ctx, &api.ProduceRequest{
		Record: &api.Record{
			Value: []byte("hello world"),
		},
	})
	require.NoError(t, err)

	consume, err := client.Consume(ctx, &api.ConsumeRequest{
		Offset: produce.Offset + 1,
	})
	require.Nil(t, consume, "consume not nil")
	got := status.Code(err)
	want := codes.OutOfRange
	require.Equal(t, want, got)
}

func testProduceConsumeStream(
	t *testing.T,
	clientList *serviceClientList,
	config *Config,
) {
	ctx := context.Background()
	records := []*api.Record{
		{
			Value:  []byte("first message"),
			Offset: 0,
		},
		{
			Value:  []byte("second message"),
			Offset: 1,
		},
	}

	client := clientList.AdminClient

	{
		stream, err := client.ProduceStream(ctx)
		require.NoError(t, err, "client produce stream failed")

		for offset, record := range records {
			err = stream.Send(&api.ProduceStreamRequest{
				Record: record,
			})
			require.NoErrorf(t, err, "client send %s failed", record.Value)
			res, err := stream.Recv()
			require.NoError(t, err, "client receive failed")
			require.Equal(t, res.Offset, uint64(offset), "offset not eqaul")
		}
	}

	{
		stream, err := client.ConsumeStream(
			ctx,
			&api.ConsumeStreamRequest{Offset: 0},
		)
		require.NoError(t, err)

		for i, record := range records {
			res, err := stream.Recv()
			require.NoError(t, err)
			require.Equal(t, res.Record, &api.Record{
				Value:  record.Value,
				Offset: uint64(i),
			})
		}
	}
	// stream context Done
}
