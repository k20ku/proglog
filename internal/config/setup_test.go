package config

import (
	"fmt"
	"io"
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
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.TLS == nil {
			msg := "r.TLS is nil"
			http.Error(w, msg, http.StatusBadRequest)
			return
		}
		if len(r.TLS.PeerCertificates) == 0 {
			msg := "r.TLS.PeerCertificates length is 0"
			http.Error(w, msg, http.StatusBadRequest)
			return
		}
		leaf := r.TLS.PeerCertificates[0]
		if len(leaf.URIs) == 0 {
			msg := "r.TLS.PeerCertificates[0].URIs length is 0"
			http.Error(w, msg, http.StatusBadRequest)
			return
		}
		expectedURI := "spiffe://proglog/workload/nobady"
		actualURI := leaf.URIs[0].String()
		if actualURI != expectedURI {
			msg := fmt.Sprintf(
				"expected=%q, got=%q.", expectedURI, actualURI,
			)
			http.Error(w, msg, http.StatusBadRequest)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	cfg := TLSConfig{
		CertDir:       certDir,
		CACertFile:    testdata.CACertFile,
		CertFile:      testdata.ServerCertFile,
		KeyFile:       testdata.ServerKeyFile,
		Server:        true,
		ServerAddress: "localhost",
	}
	serverTLSConfig, err := SetupTLSConfig(cfg)
	require.NoError(t, err, "failed to set up server tlsconfig %+v.", cfg)
	server.TLS = serverTLSConfig

	server.StartTLS()
	t.Cleanup(func() {
		server.Close()
	})

	// ---- client ----
	cfg = TLSConfig{
		CertDir:    certDir,
		CACertFile: testdata.CACertFile,
		CertFile:   testdata.ClientCertFile,
		KeyFile:    testdata.ClientKeyFile,
		Server:     false,
	}
	clientTLSConfig, err := SetupTLSConfig(cfg)
	require.NoError(t, err, "failed set up client tlsconfig %+v.", cfg)
	client := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: clientTLSConfig,
		},
	}
	resp, err := client.Get(server.URL)
	require.NoErrorf(t, err, "client failed to get %q", server.URL)

	b, err := io.ReadAll(resp.Body)
	require.NoError(t, err, "read response body failed")
	require.Equalf(t, http.StatusOK, resp.StatusCode, "status not OK, got=%q, body=%q", resp.Status, b)
	t.Cleanup(func() {
		_ = resp.Body.Close()
	})
}
