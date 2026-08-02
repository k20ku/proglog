package config

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"os"
)

// Sets up tls.Config from TLSConfig
func SetupTLSConfig(cfg TLSConfig) (tlsConfig *tls.Config, err error) {
	tlsConfig = &tls.Config{}
	if cfg.ServerCertFile != "" && cfg.ServerKeyFile != "" {
		tlsConfig.Certificates = make([]tls.Certificate, 1)
		tlsConfig.Certificates[0], err = tls.LoadX509KeyPair(
			cfg.ServerCertFile,
			cfg.ServerKeyFile,
		)
		if err != nil {
			return nil, fmt.Errorf(
				"load x509 key pair crt=%q key=%q: %w",
				cfg.ServerCertFile, cfg.ServerKeyFile, err,
			)
		}
	}

	if cfg.CACertFile != "" {
		b, err := os.ReadFile(cfg.CACertFile)
		if err != nil {
			return nil, fmt.Errorf("read %q: %w", cfg.CACertFile, err)
		}
		ca := x509.NewCertPool()
		ok := ca.AppendCertsFromPEM(b)
		if !ok {
			return nil, fmt.Errorf(
				"parse CA cert %q: %w",
				cfg.CACertFile, err,
			)
		}
		if cfg.Server {
			tlsConfig.ClientCAs = ca
			tlsConfig.ClientAuth = tls.RequireAndVerifyClientCert
		} else {
			tlsConfig.RootCAs = ca
		}
		tlsConfig.ServerName = cfg.ServerAddress
	}
	return tlsConfig, nil
}
