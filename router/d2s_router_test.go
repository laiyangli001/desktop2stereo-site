package router

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestD2SRoutesExposeAuthenticatedSupportAndNoIdentityEntryAPI(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	registerD2SRoutes(engine.Group("/api"), func(c *gin.Context) { c.Next() })

	paths := make(map[string]bool)
	for _, route := range engine.Routes() {
		paths[route.Method+" "+route.Path] = true
	}
	require.True(t, paths[http.MethodPost+" /api/v1/support/tickets"])
	require.True(t, paths[http.MethodGet+" /api/v1/support/my-tickets"])
	require.True(t, paths[http.MethodPost+" /api/v1/admin/audit/export-approvals"])
	require.True(t, paths[http.MethodGet+" /api/v1/admin/audit/export-approvals/:id"])
	require.True(t, paths[http.MethodGet+" /api/v1/admin/audit/export-approvals"])
	require.True(t, paths[http.MethodPut+" /api/v1/admin/audit/export-approvals/:id"])
	for route := range paths {
		require.False(t, strings.Contains(route, "/identity"), "identity collection route is disabled: %s", route)
	}

	for _, request := range []*http.Request{
		httptest.NewRequest(http.MethodPost, "/api/v1/support/tickets", strings.NewReader(`{"category":"OTHER"}`)),
		httptest.NewRequest(http.MethodGet, "/api/v1/support/my-tickets", nil),
	} {
		recorder := httptest.NewRecorder()
		engine.ServeHTTP(recorder, request)
		require.Equal(t, http.StatusUnauthorized, recorder.Code)
	}
}
