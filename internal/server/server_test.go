package server

import (
	"context"
	"fmt"
	"net"
	"testing"

	api "github.com/k20ku/proglog/gen/go/log/v1"
	"github.com/k20ku/proglog/internal/log"
	"github.com/stretchr/testify/require"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
)

func TestServer(t *testing.T) {
	senarios := map[string]func(
		t *testing.T,
		client api.LogServiceClient,
		config *Config,
	){
		"produce/consume a message to/from the log succeeds": testProduceConsume,
		"consume past log boundary fails":                    testConsumePastBoundary,
		"produce/consume stream succeeds":                    testProduceConsumeStream,
	}

	for senario, fn := range senarios {
		t.Run(senario, func(t *testing.T) {
			client, config, teardown := setupTest(t, nil)
			t.Cleanup(teardown)
			fn(t, client, config)
		})
	}
}

func setupTest(t *testing.T, fn func(*Config)) (
	client api.LogServiceClient,
	config *Config,
	teardown func(),
) {
	t.Helper()

	l, err := net.Listen("tcp", ":0")
	require.NoError(t, err, "listen on port 0")

	clientOptions := []grpc.DialOption{grpc.WithTransportCredentials(insecure.NewCredentials())}
	cc, err := grpc.NewClient(l.Addr().String(), clientOptions...)
	require.NoErrorf(t, err, "new client %s", l.Addr().String())

	dir := t.TempDir()

	wlog, err := log.NewLog(dir, log.NewConfig())
	require.NoErrorf(t, err, "new log at %d", dir)

	clog := NewWalCommitLog(wlog)
	require.Implements(t, (*CommitLog)(nil), clog, "log does not implement commitlog")
	config = &Config{
		CommitLog: clog,
	}

	if fn != nil {
		fn(config)
	}
	server, err := NewGRPCServer(config)
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

	client = api.NewLogServiceClient(cc)
	return client, config, func() {
		server.Stop()
		_ = cc.Close()
		_ = l.Close()
		_ = wlog.Close()
	}
}
func testProduceConsume(t *testing.T, client api.LogServiceClient, config *Config) {
	ctx := context.Background()

	want := &api.Record{
		Value: []byte("Hello Proglog"),
	}

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
	client api.LogServiceClient,
	_ *Config,
) {
	ctx := context.Background()

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
	client api.LogServiceClient,
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
