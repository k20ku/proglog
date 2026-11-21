package testutil

import (
	"testing"

	"github.com/k20ku/proglog/internal/auth"
	"github.com/k20ku/proglog/internal/server"
	"github.com/k20ku/proglog/internal/testdata"
	"github.com/stretchr/testify/require"
)

func NewACLAuthorizer(t *testing.T) server.Authorizer {
	t.Helper()

	authorizer, err := auth.New(testdata.ACLModelFile, testdata.ACLPolicyFile)
	require.NoError(t, err, "failed to new authorozer ACLModelFile=%q, ACLPolicyFile=%q", testdata.ACLModelFile, testdata.ACLPolicyFile)
	aclAuth := server.NewACLAuthorizer(authorizer)
	return aclAuth
}
