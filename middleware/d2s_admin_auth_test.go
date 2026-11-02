package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupD2SAdminRoleAuditDB(t *testing.T, migrate bool) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	if migrate {
		require.NoError(t, db.AutoMigrate(&model.D2SAdminOperationAudit{}))
	}
	previousDB := model.DB
	model.DB = db
	t.Cleanup(func() {
		model.DB = previousDB
		sqlDB, dbErr := db.DB()
		if dbErr == nil {
			_ = sqlDB.Close()
		}
	})
	return db
}

func TestD2SAdminRequireRoleAuditsDeniedRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupD2SAdminRoleAuditDB(t, true)
	router := gin.New()
	called := false
	router.PUT("/admin/users/:id/emergency",
		func(c *gin.Context) {
			c.Set("d2s_admin_id", 17)
			c.Set("d2s_admin_email", "reader@example.com")
			c.Set("d2s_admin_role", model.D2SAdminRoleAuditReader)
			c.Next()
		},
		D2SAdminRequireRole(model.D2SAdminRoleAccountOperator),
		func(c *gin.Context) {
			called = true
			c.Status(http.StatusOK)
		},
	)

	request := httptest.NewRequest(http.MethodPut, "/admin/users/42/emergency", nil)
	request.RemoteAddr = "198.51.100.7:43120"
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusForbidden, recorder.Code)
	assert.False(t, called)

	var audit model.D2SAdminOperationAudit
	require.NoError(t, db.First(&audit).Error)
	assert.Equal(t, 17, audit.OperatorID)
	assert.Equal(t, "reader@example.com", audit.OperatorEmail)
	assert.Equal(t, "ADMIN_PERMISSION_DENIED", audit.OperationType)
	assert.Equal(t, "/admin/users/:id/emergency", audit.TargetResource)
	assert.Equal(t, "198.51.100.7", audit.SourceIP)
	assert.Equal(t, "FAILURE", audit.Result)
	var details map[string]any
	require.NoError(t, common.Unmarshal([]byte(audit.DetailsJSON), &details))
	assert.Equal(t, model.D2SAdminRoleAuditReader, details["role"])
	assert.Equal(t, http.MethodPut, details["method"])
	assert.Equal(t, []any{model.D2SAdminRoleAccountOperator}, details["required_roles"])
}

func TestD2SAdminRequireRoleFailsClosedWhenAuditUnavailable(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupD2SAdminRoleAuditDB(t, false)
	router := gin.New()
	called := false
	router.GET("/admin/audit",
		func(c *gin.Context) {
			c.Set("d2s_admin_id", 18)
			c.Set("d2s_admin_email", "operator@example.com")
			c.Set("d2s_admin_role", model.D2SAdminRoleAccountOperator)
			c.Next()
		},
		D2SAdminRequireRole(model.D2SAdminRoleAuditReader),
		func(c *gin.Context) {
			called = true
			c.Status(http.StatusOK)
		},
	)

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/admin/audit", nil))

	require.Equal(t, http.StatusInternalServerError, recorder.Code)
	assert.Contains(t, recorder.Body.String(), "ADMIN_AUDIT_UNAVAILABLE")
	assert.False(t, called)
}
