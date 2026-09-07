package middleware

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestD2SAdminWriteRoutesHaveStableAuditActions(t *testing.T) {
	want := map[string]string{
		"PUT /api/v1/admin/withdrawals/:id":     "d2s.withdrawal_review",
		"PUT /api/v1/admin/unbind-requests/:id": "d2s.unbind_review",
		"PUT /api/v1/admin/users/:id/region":    "d2s.region_update",
		"PUT /api/v1/admin/signing-keys/:id":    "d2s.signing_key_retire",
		"POST /api/v1/withdrawal/request":       "d2s.withdrawal_create",
	}

	for route, action := range want {
		require.Equal(t, action, auditRouteActions[route], route)
	}
}
