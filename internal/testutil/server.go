package testutil

import (
	"testing"

	"github.com/k20ku/proglog/internal/config"
	"github.com/k20ku/proglog/internal/server"
	"github.com/k20ku/proglog/internal/testdata"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

func NewServer(t *testing.T,
	addr string,
	cfg *server.Config,
) *grpc.Server {
	serverTLSConfig, err := config.SetupTLSConfig(config.TLSConfig{
		CACertFile:    testdata.CACertFile,
		KeyFile:       testdata.ServerKeyFile,
		CertFile:      testdata.ServerCertFile,
		Server:        true,
		ServerAddress: addr,
	})
	require.NoError(t, err, "setup server tls failed")

	sCreds := credentials.NewTLS(serverTLSConfig)
	s, err := server.NewGRPCServer(cfg, grpc.Creds(sCreds))
	require.NoError(t, err, "new gRPC server")
	return s
}
