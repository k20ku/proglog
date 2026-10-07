package replicate

import (
	"context"
	"log/slog"
	"sync"

	"google.golang.org/grpc"

	api "github.com/k20ku/proglog/gen/go/log/v1"
)

type Replicator struct {
	DialOptions []grpc.DialOption
	LocalServer api.LogServiceClient

	Logger *slog.Logger

	mu               sync.Mutex
	serverLeaveChans map[string]chan struct{}
	closed           bool
	close            chan struct{}
}

// Joins the member that has a name and addr
// why does Join do replication? Only Jeffery knows...
// Join starts replication to local log server in another goroutine and
// returns nil whether or not replication succeed.
func (r *Replicator) Join(name, addr string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.init()

	if r.closed {
		return nil
	}

	if _, ok := r.serverLeaveChans[name]; ok {
		// already replicating so skip
		return nil
	}
	r.serverLeaveChans[name] = make(chan struct{})

	go r.replicate(addr, r.serverLeaveChans[name])

	return nil
}

func (r *Replicator) replicate(addr string, leave chan struct{}) {
	// build gRPC channel to remote server
	conn, err := grpc.NewClient(addr, r.DialOptions...)
	if err != nil {
		r.logError(err, "failed to dial", addr)
		return
	}
	defer func() {
		_ = conn.Close()
	}()

	client := api.NewLogServiceClient(conn)

	ctx := context.Background()
	stream, err := client.ConsumeStream(ctx,
		&api.ConsumeStreamRequest{
			Offset: 0,
		},
	)
	if err != nil {
		r.logError(err, "failed to consume", addr)
		return
	}

	records := make(chan *api.Record)
	// producer produce records from another server.
	// executed in another goroutine
	go func() {
		for {
			recv, err := stream.Recv()
			if err != nil {
				r.logError(err, "failed to receive", addr)
				return
			}
			records <- recv.Record
		}
	}()

	// consumer
	// wait for producer
	for {
		select {
		case <-r.close:
			return
		case <-leave:
			return
		case record := <-records:
			if _, err = r.LocalServer.Produce(ctx,
				&api.ProduceRequest{
					Record: record,
				},
			); err != nil {
				r.logError(err, "failed to produce", addr)
				return
			}
		}
	}
}

func (r *Replicator) Leave(name string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.init()
	if _, ok := r.serverLeaveChans[name]; !ok {
		return nil
	}
	close(r.serverLeaveChans[name])
	delete(r.serverLeaveChans, name)
	return nil
}

func (r *Replicator) init() {
	if r.Logger == nil {
		r.Logger = slog.Default().WithGroup("replicator")
	}
	if r.serverLeaveChans == nil {
		r.serverLeaveChans = make(map[string]chan struct{})
	}
	if r.close == nil {
		r.close = make(chan struct{})
	}
}

func (r *Replicator) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.init()

	if r.closed {
		return nil
	}
	r.closed = true
	close(r.close)
	return nil
}

func (r *Replicator) logError(err error, msg, addr string) {
	r.Logger.Error(
		msg,
		slog.String("addr", addr),
		slog.String("error", err.Error()),
	)
}
