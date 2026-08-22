package config

type TLSConfig struct {
	CertDir       string `env:"CERT_DIR,expand" envDefault:"${HOME}/.proglog/cert"`
	CACertFile    string `env:"CA_CERT,expand" envDefault:"${CERT_DIR}/ca.pem"`
	CertFile      string `env:"SERVER_CERT,expand" envDefault:"${CERT_DIR}/server.crt"`
	KeyFile       string `env:"SERVER_KEY,expand" envDefault:"${CERT_DIR}/server.key"`
	Server        bool
	ServerAddress string `env:"SERVER_ADDRESS" envDefault:"localhost"`
}
