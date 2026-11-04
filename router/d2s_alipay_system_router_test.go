package router

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestSystemAlipayRequiresSystemRootAndAuditsItsIdentity(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Option{}, &model.User{}, &model.Log{}, &model.D2SAlipayPCSettings{}, &model.D2SAdminOperationAudit{}))
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	previousDB, previousLogDB, previousRedis := model.DB, model.LOG_DB, common.RedisEnabled
	previousMain, previousLog := common.MainDatabaseType(), common.LogDatabaseType()
	model.DB, model.LOG_DB, common.RedisEnabled = db, db, false
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	t.Cleanup(func() {
		model.DB, model.LOG_DB, common.RedisEnabled = previousDB, previousLogDB, previousRedis
		common.SetDatabaseTypes(previousMain, previousLog)
	})
	for _, user := range []struct {
		id, role int
		token    string
	}{
		{91001, common.RoleRootUser, "root-payment-test-token"},
		{91002, common.RoleAdminUser, "admin-payment-test-token"},
		{91003, common.RoleCommonUser, "user-payment-test-token"},
	} {
		token := user.token
		require.NoError(t, db.Create(&model.User{
			Id: user.id, Username: "payment-" + token, Email: token + "@example.com",
			Password: "unused", Role: user.role, Status: common.UserStatusEnabled,
			Group: "default", AuthVersion: 1, AccessToken: &token, AffCode: token,
		}).Error)
	}
	engine := gin.New()
	registerSystemAlipayRoutes(engine.Group("/api/option", middleware.RootAuth()))
	registerD2SRoutes(engine.Group("/api"), func(c *gin.Context) { c.Next() })
	for _, test := range []struct {
		token  string
		status int
	}{
		{"", http.StatusUnauthorized},
		{"user-payment-test-token", http.StatusForbidden},
		{"admin-payment-test-token", http.StatusForbidden},
		{"root-payment-test-token", http.StatusOK},
	} {
		request := httptest.NewRequest(http.MethodGet, "/api/option/d2s-purchases", nil)
		request.AddCookie(&http.Cookie{Name: "d2s_admin_session", Value: "independent-security-cookie"})
		if test.token != "" {
			request.Header.Set("Authorization", "Bearer "+test.token)
		}
		response := httptest.NewRecorder()
		engine.ServeHTTP(response, request)
		require.Equal(t, test.status, response.Code, response.Body.String())
	}
	for _, enabled := range []bool{true, false} {
		body := "{\"enabled\":false}"
		if enabled {
			body = "{\"enabled\":true}"
		}
		request := httptest.NewRequest(http.MethodPut, "/api/option/d2s-purchases", strings.NewReader(body))
		request.Header.Set("Authorization", "Bearer root-payment-test-token")
		response := httptest.NewRecorder()
		engine.ServeHTTP(response, request)
		require.Equal(t, http.StatusOK, response.Code, response.Body.String())
		actual, err := model.D2SPurchasesEnabled()
		require.NoError(t, err)
		require.Equal(t, enabled, actual)
	}
	var settingAudits []model.D2SAdminOperationAudit
	require.NoError(t, db.Where("operation_type = ?", "UPDATE_PURCHASE_SETTINGS").Find(&settingAudits).Error)
	require.Len(t, settingAudits, 2)
	require.Equal(t, 91001, settingAudits[0].OperatorID)
	require.Contains(t, settingAudits[0].DetailsJSON, `"operator_realm":"system_admin"`)
	require.NotEmpty(t, settingAudits[0].SourceIP)

	request := httptest.NewRequest(http.MethodPut, "/api/option/alipay/config", strings.NewReader(`{"qr_pay_mode":"2"}`))
	request.Header.Set("Authorization", "Bearer root-payment-test-token")
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	engine.ServeHTTP(response, request)
	require.Equal(t, http.StatusBadRequest, response.Code, response.Body.String())
	var audit model.D2SAdminOperationAudit
	require.NoError(t, db.Where("operation_type = ?", "UPDATE_ALIPAY_CONFIG").First(&audit).Error)
	require.Equal(t, 91001, audit.OperatorID)
	require.Equal(t, "root-payment-test-token@example.com", audit.OperatorEmail)
	require.Contains(t, audit.DetailsJSON, `"operator_realm":"system_admin"`)
	for _, route := range engine.Routes() {
		require.False(t, strings.HasPrefix(route.Path, "/api/v1/admin/alipay"), "security console must not own payment routes")
	}
}
