package controller

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

func TestD2SAdminLoginAuditsMalformedRequests(t *testing.T) {
	database, err := gorm.Open(sqlite.Open("file:d2s_admin_login_audit_test?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, database.AutoMigrate(&model.D2SAdminOperationAudit{}))

	previousDB := model.DB
	model.DB = database
	t.Cleanup(func() { model.DB = previousDB })

	previousMode := gin.Mode()
	gin.SetMode(gin.TestMode)
	t.Cleanup(func() { gin.SetMode(previousMode) })

	requests := []struct {
		name string
		body string
	}{
		{name: "malformed JSON", body: `{"email":"admin@example.test","password":"must-not-be-logged"`},
		{name: "invalid email", body: `{"email":"not-an-email","password":"must-not-be-logged"}`},
		{name: "missing password", body: `{"email":"admin@example.test"}`},
	}
	for index, testRequest := range requests {
		t.Run(testRequest.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			context, _ := gin.CreateTestContext(recorder)
			context.Request = httptest.NewRequest(http.MethodPost, "/api/v1/admin/auth/login", strings.NewReader(testRequest.body))
			context.Request.Header.Set("Content-Type", "application/json")
			context.Request.RemoteAddr = "198.51.100.25:43120"
			D2SAdminLogin(context)

			require.Equal(t, http.StatusBadRequest, recorder.Code)
			var audit model.D2SAdminOperationAudit
			require.NoError(t, database.Where("operation_type = ? AND source_ip = ?", "LOGIN_ADMIN", "198.51.100.25").Order("rowid DESC").First(&audit).Error)
			require.Equal(t, "FAILURE", audit.Result)
			require.NotEmpty(t, audit.OperatorEmail)
			require.NotContains(t, audit.DetailsJSON, "must-not-be-logged")
			_ = index
		})
	}

	var count int64
	require.NoError(t, database.Model(&model.D2SAdminOperationAudit{}).Where("operation_type = ?", "LOGIN_ADMIN").Count(&count).Error)
	require.Equal(t, int64(len(requests)), count)
}
