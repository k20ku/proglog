package config

type TLSConfig struct {
	CertDir       string `env:"CERT_DIR,expand" envDefault:"${HOME}/.proglog/cert"`
	CACertFile    string `env:"CA_CERT,expand" envDefault:"${CERT_DIR}/ca.pem"`
	CertFile      string
	KeyFile       string
	Server        bool
	ServerAddress string `env:"SERVER_ADDRESS" envDefault:"localhost"`
}
