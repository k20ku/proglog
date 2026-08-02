package config

import (
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSetup(t *testing.T) {
	// ---- env ----
	err := os.Setenv("CERT_DIR", "testdata/cert")
	require.NoErrorf(t, err, "set env CERT_DIR=%q failed", "testdata/cert")

	// ---- server ----
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	cfg, err := NewTLSConfig(true)
	require.NoError(t, err)
	serverTLSConfig, err := SetupTLSConfig(cfg)
	require.NoError(t, err, "failed to set up server tlsconfig %+v.", cfg)
	server.TLS = serverTLSConfig

	server.StartTLS()
	t.Cleanup(func() {
		server.Close()
	})

	// ---- client ----
	cfg, err = NewTLSConfig(false)
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
