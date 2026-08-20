package server

import (
	"context"
	"testing"

	api "github.com/k20ku/proglog/gen/go/log/v1"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestServer(t *testing.T) {
	senarios := map[string]func(
		t *testing.T,
		clientList *serviceClientList,
		config *Config,
	){
		"produce/consume a message to/from the log succeeds": testProduceConsume,
		"consume past log boundary fails":                    testConsumePastBoundary,
		"produce/consume stream succeeds":                    testProduceConsumeStream,
		"test unauthorized client":                           testUnauthorized,
	}

	for senario, fn := range senarios {
		t.Run(senario, func(t *testing.T) {
			clientList, config, teardown := clientSetupTest(t, nil)
			t.Cleanup(teardown)
			fn(t, clientList, config)
		})
	}
}

func testUnauthorized(t *testing.T, clientList *serviceClientList, config *Config) {
	ctx := context.Background()
	nobodyClient := clientList.NobodyClient
	produceResp, err := nobodyClient.Produce(ctx, &api.ProduceRequest{
		Record: &api.Record{Value: []byte("helloworld")},
	})
	require.Nil(t, produceResp, "response must be nil by unauthed produce")
	gotCode := status.Code(err)
	wantCode := codes.PermissionDenied
	require.Equal(t, gotCode, wantCode, "unauthorized error not expected")

	// ---- consume ----
	consumeResp, err := nobodyClient.Consume(ctx, &api.ConsumeRequest{
		Offset: 0,
	})
	require.Nil(t, consumeResp, "response must be nil by unauthed consume")
	gotCode = status.Code(err)
	wantCode = codes.PermissionDenied
	require.Equal(t, gotCode, wantCode, "unauthorized error not expected")
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
