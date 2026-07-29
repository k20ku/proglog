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

	clog, err := log.NewLog(dir, log.NewConfig())
	require.NoErrorf(t, err, "new log at %d", dir)

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
		_ = clog.Close()
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
	fmt.Printf("\n%#v\n", got)
	// require.Implements(t, (*ErrOffsetOutOfRange)(nil), got)
	// want := grpc.Code(ErrOffsetOutOfRange{}.GRPCStatus().Err())
	// if got != want {
	// 	t.Fatalf("got err: %v, want: %v", got, want)
	// }
}
