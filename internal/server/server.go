package server

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"

	api "github.com/k20ku/proglog/gen/go/log/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type grpcServer struct {
	api.UnimplementedLogServiceServer
	*Config
}

var _ api.LogServiceServer = (*grpcServer)(nil)

func NewGRPCServer(config *Config) (*grpc.Server, error) {
	gsrv := grpc.NewServer()
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
	offset, err := s.appendRecord(req.Record)
	if err != nil {
		// TODO: respond error status
		return nil, err
	}
	return &api.ProduceResponse{Offset: uint64(offset)}, nil
}

func (s *grpcServer) Consume(ctx context.Context, req *api.ConsumeRequest) (
	*api.ConsumeResponse, error,
) {
	record, err, _ := s.readRecord(req.Offset)
	if err != nil {
		return nil, err
	}
	return &api.ConsumeResponse{Record: record}, nil
}

func (s *grpcServer) ProduceStream(stream api.LogService_ProduceStreamServer) error {
	for {
		req, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			return nil
		} else if err != nil {
			return status.Errorf(codes.Internal, "failed to receive")
		}
		offset, err := s.appendRecord(req.Record)
		if err != nil {
			return err
		}
		if err := stream.Send(
			&api.ProduceStreamResponse{Offset: offset},
		); err != nil {
			log.Printf("send offset %d: %v", offset, err)
		}
	}
}

// When the server reaches the end of the log, the server will wait until someone appends a record to the log and then continue streaming records to the client.
// [Travis Jeffery. distributed-services-with-go_P1.0 (Kindle Position No.2522-2523). Kindle Version. ]
func (s *grpcServer) ConsumeStream(
	req *api.ConsumeStreamRequest,
	stream api.LogService_ConsumeStreamServer,
) error {
	for {
		// Continue reading until stream stops flowing.
		// request specifies first offset to be incrementally read to
		select {
		case <-stream.Context().Done():
			return nil
		default:
			record, err, outOfRange := s.readRecord(req.Offset)
			if err != nil {
				if !outOfRange {
					return err
				}
				continue
			}
			if err := stream.Send(
				&api.ConsumeStreamResponse{Record: record},
			); err != nil {
				log.Fatalf("failed to Send: %v", err)
				return status.Error(codes.Internal, "failed to send")
			}
			req.Offset++
		}
	}
}

func (s *grpcServer) appendRecord(
	record *api.Record,
) (offset uint64, err error) {
	offset, err = s.CommitLog.Append(record)
	if err != nil {
		return 0, err
	}
	return offset, nil
}

func (s *grpcServer) readRecord(
	offset uint64,
) (record *api.Record, err error, isOutOfRange bool) {
	record, err = s.CommitLog.Read(offset)
	if err != nil {
		if _, ok := errors.AsType[ErrOffsetOutOfRange](err); ok {
			return nil, err, ok
		}
		return nil,
			status.Error(
				codes.Internal, "Internal Server Error",
			),
			false
	}
	return record, nil, false
}
