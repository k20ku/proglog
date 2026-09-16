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

	logger *slog.Logger

	mu      sync.Mutex
	servers map[string]chan struct{}
	closed  bool
	close   chan struct{}
}

func (r *Replicator) Join(name, addr string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.init()

	if r.closed {
		return nil
	}

	if _, ok := r.servers[name]; ok {
		// already replicating so skip
		return nil
	}
	r.servers[name] = make(chan struct{})

	// r.servers[name] : leave channel of server name
	go r.replicate(addr, r.servers[name])

	return nil
}

func (r *Replicator) replicate(addr string, leave chan struct{}) {
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
	// producer
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
	if _, ok := r.servers[name]; !ok {
		return nil
	}
	close(r.servers[name])
	delete(r.servers, name)
	return nil
}

func (r *Replicator) init() {
	if r.logger == nil {
		r.logger = slog.Default().WithGroup("replicator")
	}
	if r.servers == nil {
		r.servers = make(map[string]chan struct{})
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
	r.logger.Error(
		msg,
		slog.String("addr", addr),
		slog.String("error", err.Error()),
	)
}
