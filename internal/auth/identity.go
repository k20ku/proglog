package auth

import "crypto/tls"

type Identity struct {
	SPIFFEID string
}

func IdentityFromTLS(cs *tls.ConnectionState) (Identity, error) {
	// TODO: implement
	return Identity{}, nil
}
