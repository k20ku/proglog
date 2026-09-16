package testutil

import (
	"testing"

	api "github.com/k20ku/proglog/gen/go/log/v1"
	"github.com/k20ku/proglog/internal/config"
	"github.com/k20ku/proglog/internal/testdata"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

func NewClient(t *testing.T,
	addr string,
	certPath, keyPath string,
) (
	*grpc.ClientConn,
	api.LogServiceClient,
	[]grpc.DialOption,
) {
	t.Helper()
	clientTLSConfig, err := config.SetupTLSConfig(config.TLSConfig{
		CACertFile: testdata.CACertFile,
		CertFile:   certPath,
		KeyFile:    keyPath,
		Server:     false,
	})
	require.NoError(t, err, "setup client tls failed")
	clientCreds := credentials.NewTLS(clientTLSConfig)
	ops := []grpc.DialOption{grpc.WithTransportCredentials(clientCreds)}
	conn, err := grpc.NewClient(addr, ops...)
	require.NoErrorf(t, err, "new client %s", addr)
	client := api.NewLogServiceClient(conn)
	return conn, client, ops
}
