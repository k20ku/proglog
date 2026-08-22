package server

import (
	"errors"

	"github.com/k20ku/proglog/internal/auth"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

var _ Authorizer = (*aclAuthorizer)(nil)

type aclAuthorizer struct {
	a *auth.Authorizer
}

func NewACLAuthorizer(a *auth.Authorizer) *aclAuthorizer {
	return &aclAuthorizer{a: a}
}
func (aclAuth *aclAuthorizer) Authorize(subject, action, object string) error {
	err := aclAuth.a.Authorize(subject, action, object)
	if err != nil {
		if errpd, ok := errors.AsType[auth.ErrPermissionDenied](err); ok {
			return ErrPermissionDenied{
				Subject: errpd.Subject,
				Action:  errpd.Action,
				Object:  errpd.Object,
			}
		}
		return status.Errorf(codes.Internal, "authentication failed")
	}
	return nil
}
