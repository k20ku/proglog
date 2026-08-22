package auth

import "fmt"

var _ error = ErrPermissionDenied{}

type ErrPermissionDenied struct {
	Subject, Action, Object string
}

func (e ErrPermissionDenied) Error() string {
	return fmt.Sprintf(
		"%q not permitted to do %q to %q",
		e.Subject,
		e.Action,
		e.Object,
	)
}
