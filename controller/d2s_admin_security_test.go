package controller

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupD2SEmergencyControlTest(t *testing.T, withSessionTables bool) *gorm.DB {
	t.Helper()

	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)

	previousDB, previousLogDB := model.DB, model.LOG_DB
	previousMainType, previousLogType := common.MainDatabaseType(), common.LogDatabaseType()
	previousRedisEnabled := common.RedisEnabled
	model.DB, model.LOG_DB = db, db
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	common.RedisEnabled = false
	t.Cleanup(func() {
		model.DB, model.LOG_DB = previousDB, previousLogDB
		common.SetDatabaseTypes(previousMainType, previousLogType)
		common.RedisEnabled = previousRedisEnabled
	})

	models := []any{&model.User{}, &model.D2SUserSecurityControl{}, &model.D2SAdminOperationAudit{}}
	if withSessionTables {
		models = append(models, &model.UserSession{}, &model.D2SOnlineLease{})
	}
	require.NoError(t, db.AutoMigrate(models...))
	require.NoError(t, db.Create(&model.User{
		Id: 1901, Username: "security-target", Email: "target@example.com",
		Status: common.UserStatusEnabled, AuthVersion: 1,
	}).Error)
	return db
}

func emergencyControlRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/users/:id/security", func(c *gin.Context) {
		c.Set("d2s_admin_id", 17)
		c.Set("d2s_admin_email", "operator@example.com")
		D2SAdminSetEmergencyControl(c)
	})
	return router
}

func TestD2SAdminForceLogoutAuditCompletesOnlyAfterSuccess(t *testing.T) {
	db := setupD2SEmergencyControlTest(t, true)
	request := httptest.NewRequest(http.MethodPost, "/users/1901/security", strings.NewReader(`{"force_logout":true,"reason":"incident"}`))
	request.RemoteAddr = "192.0.2.10:43123"
	recorder := httptest.NewRecorder()
	emergencyControlRouter().ServeHTTP(recorder, request)

	require.Equal(t, http.StatusOK, recorder.Code)
	var response struct {
		Success bool `json:"success"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &response))
	require.True(t, response.Success)

	var audits []model.D2SAdminOperationAudit
	require.NoError(t, db.Where("target_resource = ?", "1901").Order("id").Find(&audits).Error)
	require.Len(t, audits, 2)
	results := map[string]bool{}
	for _, audit := range audits {
		results[audit.Result] = true
		require.Equal(t, "EMERGENCY_LOCK", audit.OperationType)
	}
	require.True(t, results["STARTED"])
	require.True(t, results["SUCCESS"])
}

func TestD2SAdminForceLogoutFailureIsNotAuditedAsSuccess(t *testing.T) {
	db := setupD2SEmergencyControlTest(t, false)
	request := httptest.NewRequest(http.MethodPost, "/users/1901/security", strings.NewReader(`{"force_logout":true,"reason":"incident"}`))
	request.RemoteAddr = "192.0.2.10:43123"
	recorder := httptest.NewRecorder()
	emergencyControlRouter().ServeHTTP(recorder, request)

	require.Equal(t, http.StatusInternalServerError, recorder.Code)

	var audits []model.D2SAdminOperationAudit
	require.NoError(t, db.Where("target_resource = ?", "1901").Find(&audits).Error)
	require.Len(t, audits, 2)
	results := map[string]bool{}
	for _, audit := range audits {
		results[audit.Result] = true
		require.Equal(t, "EMERGENCY_LOCK", audit.OperationType)
	}
	require.True(t, results["STARTED"])
	require.True(t, results["FAILURE"])
	require.False(t, results["SUCCESS"])
}

func TestD2SAdminEmergencyControlFailsClosedWhenFailureAuditCannotBeWritten(t *testing.T) {
	db := setupD2SEmergencyControlTest(t, false)
	require.NoError(t, db.Migrator().DropTable(&model.D2SAdminOperationAudit{}))
	request := httptest.NewRequest(http.MethodPost, "/users/1901/security", strings.NewReader(`{"reason":"missing action"}`))
	request.RemoteAddr = "192.0.2.10:43123"
	recorder := httptest.NewRecorder()
	emergencyControlRouter().ServeHTTP(recorder, request)

	require.Equal(t, http.StatusInternalServerError, recorder.Code)
	require.Contains(t, recorder.Body.String(), "AUDIT_WRITE_FAILED")
}
