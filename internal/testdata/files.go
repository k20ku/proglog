package testdata

import (
	"path/filepath"
	"runtime"
)

var (
	CACertFile     = TestdataPath("cert", "ca.pem")
	ServerCertFile = TestdataPath("cert", "server.crt")
	ServerKeyFile  = TestdataPath("cert", "server.key")
	NobodyCertFile = TestdataPath("cert", "nobody-client.crt")
	NobodyKeyFile  = TestdataPath("cert", "nobody-client.key")
	AdminCertFile  = TestdataPath("cert", "admin-client.crt")
	AdminKeyFile   = TestdataPath("cert", "admin-client.key")
	ACLModelFile   = TestdataPath("auth", "model.conf")
	ACLPolicyFile  = TestdataPath("auth", "policy.csv")
)

func TestdataPath(elem ...string) string {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		panic("testdata: failed to get caller")
	}
	parts := append([]string{filepath.Dir(file)}, elem...)
	return filepath.Join(parts...)
}
