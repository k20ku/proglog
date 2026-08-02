package config

import (
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/k20ku/proglog/internal/testdata"
	"github.com/stretchr/testify/require"
)

func TestSetup(t *testing.T) {
	// ---- env ----
	certDir := testdata.TestdataPath("cert")
	err := os.Setenv("CERT_DIR", certDir)
	require.NoErrorf(t, err, "set env CERT_DIR=%q failed", certDir)

	// ---- server ----
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	cfg, err := NewServerTLSConfig()
	require.NoError(t, err)
	serverTLSConfig, err := SetupTLSConfig(cfg)
	require.NoError(t, err, "failed to set up server tlsconfig %+v.", cfg)
	server.TLS = serverTLSConfig

	server.StartTLS()
	t.Cleanup(func() {
		server.Close()
	})

	// ---- client ----
	cfg, err = NewClientTLSConfig()
	require.NoError(t, err, "new client Config failed.")

	clientTLSConfig, err := SetupTLSConfig(cfg)
	require.NoError(t, err, "failed set up client tlsconfig %+v.", cfg)
	client := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: clientTLSConfig,
		},
	}
	resp, err := client.Get(server.URL)
	require.NoErrorf(t, err, "client failed to get %q", server.URL)
	t.Cleanup(func() {
		_ = resp.Body.Close()
	})
}
