package server

import (
	"context"
	"errors"
	"fmt"
	"io"

	api "github.com/k20ku/proglog/gen/go/log/v1"
)

type grpcServer struct {
	api.UnimplementedLogServiceServer
	*Config
}

var _ api.LogServiceServer = (*grpcServer)(nil)

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
	record, err := s.CommitLog.Read(req.Offset)
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
			return fmt.Errorf("gepcserver ProduceStream: recv req failed: %w", err)
		}
		offset, err := s.appendRecord(req.Record)
		if err != nil {
			return fmt.Errorf("grpcServer ProduceStream: append record failed: %w", err)
		}
		if err := stream.Send(
			&api.ProduceStreamResponse{Offset: offset},
		); err != nil {
			// TODO: sendということであれば普通にまともにエラーresponseを送るのは困難だと
			// since the connection is likely to be closed unhappily
			// if send failed
			return fmt.Errorf("ProduceStream: send faied: %w", err)
		}
	}
}
func (s *grpcServer) appendRecord(
	record *api.Record,
) (offset uint64, err error) {
	offset, err = s.CommitLog.Append(record)
	if err != nil {
		// TODO: to think of "うーん絶対これ呼び出し側でappend recordってエラーに書くからいいよねこれで"
		return 0, err
	}
	return offset, nil
}
