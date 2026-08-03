package auth

import (
	"fmt"

	"github.com/casbin/casbin/v3"
)

type Authorizer struct {
	enforcer *casbin.Enforcer
}

func New(model, policy string) (*Authorizer, error) {
	enforcer, err := casbin.NewEnforcer(model, policy)
	if err != nil {
		return nil, fmt.Errorf(
			"new enforcer from model=%q, policy=%q: %w",
			model, policy, err,
		)
	}
	return &Authorizer{enforcer: enforcer}, nil
}

// Authorizes whether subject is permitted to run the action on the object.
// If not permitted, returns ErrPermissionDenied.
func (a *Authorizer) Authorize(subject, action, object string) error {
	ok, err := a.enforcer.Enforce(subject, object, action)
	if err != nil {
		return fmt.Errorf(
			"enforcing sub=%q, obj=%q, act=%q failed: %w",
			subject,
			object,
			action,
			err,
		)
	}
	if !ok {
		return ErrPermissionDenied{
			Subject: subject,
			Object:  object,
			Action:  action,
		}
	}
	return nil
}
