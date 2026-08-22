package server

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"log/slog"
	"strings"

	grpcauth "github.com/grpc-ecosystem/go-grpc-middleware/v2/interceptors/auth"
	logging "github.com/grpc-ecosystem/go-grpc-middleware/v2/interceptors/logging"
	api "github.com/k20ku/proglog/gen/go/log/v1"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"
)

type grpcServer struct {
	api.UnimplementedLogServiceServer
	*Config
}

var _ api.LogServiceServer = (*grpcServer)(nil)

// authenticate is an interceptor that reads the subject out of the client's cert
// and write it to the RPC's context.
// With this interceptor, you can intercept and modify the execution
// of each RPC's call.
func authenticate(ctx context.Context) (context.Context, error) {
	peer, ok := peer.FromContext(ctx)
	if !ok {
		return ctx, status.Error(
			codes.Unknown,
			"couldn't find peer info",
		)
	}
	if peer.AuthInfo == nil {
		return context.WithValue(ctx, subjectContextKey{}, ""), nil
	}
	tlsInfo, ok := peer.AuthInfo.(credentials.TLSInfo)
	if !ok {
		return context.WithValue(ctx, subjectContextKey{}, ""), nil
	}
	uris := tlsInfo.State.VerifiedChains[0][0].URIs
	if len(uris) == 0 {
		return ctx, status.Error(
			codes.InvalidArgument,
			"length of URIs is 0.",
		)
	}
	uri := uris[0]
	paths := strings.Split(uri.Path, "/")
	subject := paths[len(paths)-1]
	ctx = context.WithValue(ctx, subjectContextKey{}, subject)

	return ctx, nil
}

func subject(ctx context.Context) string {
	return ctx.Value(subjectContextKey{}).(string)
}

type subjectContextKey struct{}

// InterceptorLogger adapts slog logger to interceptor logger.
func InterceptorLogger(l *slog.Logger) logging.Logger {
	return logging.LoggerFunc(func(ctx context.Context, lvl logging.Level, msg string, fields ...any) {
		l.Log(ctx, slog.Level(lvl), msg, fields...)
	})
}

func NewGRPCServer(config *Config, ops ...grpc.ServerOption) (
	*grpc.Server,
	error,
) {
	// logging
	logger := config.Logger
	opts := []logging.Option{
		logging.WithDurationField(
			logging.DurationToDurationField,
		),
	}

	ops = append(ops,
		grpc.ChainStreamInterceptor(
			grpcauth.StreamServerInterceptor(authenticate),
			logging.StreamServerInterceptor(
				InterceptorLogger(logger), opts...,
			),
		),
		grpc.ChainUnaryInterceptor(
			grpcauth.UnaryServerInterceptor(authenticate),
			logging.UnaryServerInterceptor(
				InterceptorLogger(logger),
				opts...,
			),
		),
		grpc.StatsHandler(otelgrpc.NewServerHandler()),
	)
	gsrv := grpc.NewServer(ops...)
	srv, err := newgrpcServer(config)
	if err != nil {
		return nil, fmt.Errorf("new gRPC Server: %w", err)
	}
	api.RegisterLogServiceServer(gsrv, srv)
	return gsrv, nil
}

func newgrpcServer(config *Config) (srv *grpcServer, err error) {
	srv = &grpcServer{
		Config: config,
	}

	return srv, nil
}

func (s *grpcServer) Produce(ctx context.Context, req *api.ProduceRequest) (
	*api.ProduceResponse, error,
) {
	if err := s.Authorizer.Authorize(
		subject(ctx),
		actionProduce,
		objectLogs,
	); err != nil {
		return nil, err
	}
	offset, err := s.CommitLog.Append(req.Record)
	if err != nil {
		return nil, err
	}
	return &api.ProduceResponse{Offset: uint64(offset)}, nil
}

func (s *grpcServer) Consume(ctx context.Context, req *api.ConsumeRequest) (
	*api.ConsumeResponse, error,
) {
	if err := s.Authorizer.Authorize(
		subject(ctx),
		actionConsume,
		objectLogs,
	); err != nil {
		return nil, err
	}
	record, err := s.CommitLog.Read(req.Offset)
	if err != nil {
		return nil, err
	}
	return &api.ConsumeResponse{Record: record}, nil
}

func (s *grpcServer) ProduceStream(stream api.LogService_ProduceStreamServer) error {
	if err := s.Authorizer.Authorize(
		subject(stream.Context()),
		actionProduce,
		objectLogs,
	); err != nil {
		return err
	}
	for {
		req, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			return nil
		} else if err != nil {
			return status.Errorf(codes.Internal, "failed to receive record")
		}

		offset, err := s.CommitLog.Append(req.Record)
		if err != nil {
			return err
		}
		if err := stream.Send(
			&api.ProduceStreamResponse{Offset: offset},
		); err != nil {
			log.Printf("send offset %d: %v", offset, err)
			return status.Error(codes.Internal, "failed to send record")
		}
	}
}

// When the server reaches the end of the log, the server will wait until someone appends record to the log and then continue streaming records to the client.
// [Travis Jeffery. distributed-services-with-go_P1.0 (Kindle Position No.2522-2523). Kindle]
func (s *grpcServer) ConsumeStream(
	req *api.ConsumeStreamRequest,
	stream api.LogService_ConsumeStreamServer,
) error {
	if err := s.Authorizer.Authorize(
		subject(stream.Context()),
		actionConsume,
		objectLogs,
	); err != nil {
		return err
	}
	for {
		// Continue reading until stream stops flowing.
		// request specifies first offset to be incrementally read to
		select {
		case <-stream.Context().Done():
			return nil
		default:
			record, err := s.CommitLog.Read(req.Offset)
			if err != nil {
				if _, ok := errors.AsType[ErrOffsetOutOfRange](err); ok {
					continue
				}
				return err
			}
			if err := stream.Send(
				&api.ConsumeStreamResponse{Record: record},
			); err != nil {
				return status.Error(codes.Internal, "Failed to Send Response")
			}
			req.Offset++
		}
	}
}
