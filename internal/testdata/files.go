package testdata

import (
	"path/filepath"
	"runtime"
)

var (
	CACertFile     = TestdataPath("cert", "ca.pem")
	ServerCertFile = TestdataPath("cert", "server.crt")
	ServerKeyFile  = TestdataPath("cert", "server.key")
)

func TestdataPath(elem ...string) string {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		panic("testdata: failed to get caller")
	}
	parts := append([]string{filepath.Dir(file)}, elem...)
	return filepath.Join(parts...)
}
