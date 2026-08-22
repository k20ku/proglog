package auth

import (
	"errors"
	"testing"

	"github.com/k20ku/proglog/internal/testdata"
	"github.com/stretchr/testify/require"
)

func TestAuthorize(t *testing.T) {
	model := testdata.ACLModelFile
	policy := testdata.ACLPolicyFile
	authr, err := New(model, policy)
	require.NoError(
		t, err,
		"failed to new authenticator model=%q, policy=%q",
		model, policy,
	)
	sub, act, obj := "admin", "produce", "logs"
	err = authr.Authorize(sub, act, obj)
	require.NoError(t, err, "admin auth failed")
	sub, act, obj = "nobody", "produce", "logs"
	err = authr.Authorize(sub, act, obj)
	require.Error(t, err, "admin auth failed")
	errPD, ok := errors.AsType[ErrPermissionDenied](err)
	require.True(t, ok, "err is not %T, got=%+v", ErrPermissionDenied{}, err)
	require.Equal(t, errPD.Subject, sub)
	require.Equal(t, errPD.Action, act)
	require.Equal(t, errPD.Object, obj)
}
