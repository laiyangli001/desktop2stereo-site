package controller

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupD2SDownloadActionLogTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	previousDB := model.DB
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.D2SUserActionLog{}))
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

func TestD2SDownloadActionLogRecordsDirectClientPort(t *testing.T) {
	t.Setenv("TRUSTED_PROXIES", "none")
	db := setupD2SDownloadActionLogTestDB(t)
	gin.SetMode(gin.TestMode)
	context, _ := gin.CreateTestContext(httptest.NewRecorder())
	context.Request = httptest.NewRequest(http.MethodPost, "/api/v1/downloads/release/track", nil)
	context.Request.RemoteAddr = "203.0.113.10:43120"
	user := &model.User{Id: 12001, Email: "download-port@example.com"}

	require.NoError(t, recordD2SDownloadClick(context, user, "release"))

	var entry model.D2SUserActionLog
	require.NoError(t, db.Where("action_type = ?", model.D2SActionDownload).First(&entry).Error)
	require.NotNil(t, entry.SourcePort)
	require.Equal(t, 43120, *entry.SourcePort)
	require.Equal(t, "203.0.113.10", entry.SourceIP)
}

func TestD2SDownloadActionLogOmitsProxyPort(t *testing.T) {
	t.Setenv("TRUSTED_PROXIES", "10.0.0.0/8")
	db := setupD2SDownloadActionLogTestDB(t)
	gin.SetMode(gin.TestMode)
	context, engine := gin.CreateTestContext(httptest.NewRecorder())
	require.NoError(t, engine.SetTrustedProxies([]string{"10.0.0.0/8"}))
	context.Request = httptest.NewRequest(http.MethodPost, "/api/v1/downloads/release/track", nil)
	context.Request.RemoteAddr = "10.0.0.5:37890"
	context.Request.Header.Set("X-Forwarded-For", "203.0.113.20")
	user := &model.User{Id: 12002, Email: "proxied-download@example.com"}

	require.NoError(t, recordD2SDownloadClick(context, user, "release"))

	var entry model.D2SUserActionLog
	require.NoError(t, db.Where("action_type = ?", model.D2SActionDownload).First(&entry).Error)
	require.Nil(t, entry.SourcePort)
	require.Equal(t, "203.0.113.20", entry.SourceIP)
}

func TestD2SAnonymousDownloadClickRecordsNetworkWithoutAccount(t *testing.T) {
	db := setupD2SDownloadActionLogTestDB(t)
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Params = gin.Params{{Key: "target", Value: "release"}}
	context.Request = httptest.NewRequest(http.MethodPost, "/api/v1/downloads/release/track", nil)
	context.Request.RemoteAddr = "203.0.113.40:43124"
	context.Request.Header.Set("User-Agent", "anonymous-download-test")

	D2STrackDownload(context)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Contains(t, recorder.Body.String(), `"logged":true`)
	var entry model.D2SUserActionLog
	require.NoError(t, db.Where("action_type = ?", model.D2SActionDownload).First(&entry).Error)
	require.Nil(t, entry.UserID)
	require.Empty(t, entry.Email)
	require.Equal(t, "203.0.113.40", entry.SourceIP)
	require.Equal(t, "anonymous-download-test", entry.UserAgent)
	require.Equal(t, model.D2SActionResultSuccess, entry.ResultStatus)
}
