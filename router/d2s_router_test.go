package router

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
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
	require.True(t, paths[http.MethodGet+" /api/v1/admin/withdrawals/:id/payout"])
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

func TestD2SAnonymousDownloadRouteLogsClickWithoutAccount(t *testing.T) {
	gin.SetMode(gin.TestMode)
	previousDB := model.DB
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	model.DB = db
	t.Cleanup(func() {
		model.DB = previousDB
		sqlDB, dbErr := db.DB()
		if dbErr == nil {
			_ = sqlDB.Close()
		}
	})
	require.NoError(t, db.AutoMigrate(&model.D2SUserActionLog{}))

	engine := gin.New()
	registerD2SRoutes(engine.Group("/api"), func(c *gin.Context) { c.Next() })
	request := httptest.NewRequest(http.MethodPost, "/api/v1/downloads/release/track", nil)
	request.RemoteAddr = "203.0.113.41:43125"
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Contains(t, recorder.Body.String(), `"logged":true`)
	var entry model.D2SUserActionLog
	require.NoError(t, db.Where("action_type = ?", model.D2SActionDownload).First(&entry).Error)
	require.Nil(t, entry.UserID)
	require.Empty(t, entry.Email)
	require.Equal(t, "203.0.113.41", entry.SourceIP)
}
