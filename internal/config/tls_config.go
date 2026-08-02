package config

import (
	"fmt"

	"github.com/caarlos0/env/v11"
)

type TLSConfig struct {
	CertDir        string `env:"CERT_DIR,expand" envDefault:"${HOME}/.proglog/cert"`
	CACertFile     string `env:"CA_CERT,expand" envDefault:"${CERT_DIR}/ca.pem"`
	ServerCertFile string `env:"SERVER_CERT,expand" envDefault:"${CERT_DIR}/server.crt"`
	ServerKeyFile  string `env:"SERVER_KEY,expand" envDefault:"${CERT_DIR}/server.key"`
	Server         bool
	ServerAddress  string `env:"SERVER_ADDRESS" envDefault:"localhost"`
}

func NewTLSConfig(isServer bool) (TLSConfig, error) {
	cfg, err := env.ParseAs[TLSConfig]()
	if err != nil {
		return cfg, fmt.Errorf("parse config: %w", err)
	}
	cfg.Server = isServer
	return cfg, nil
}
