package middleware

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestD2SLoginActionLogFailsClosedWhenAuditStorageIsUnavailable(t *testing.T) {
	previousDB := model.DB
	model.DB = nil
	t.Cleanup(func() {
		model.DB = previousDB
	})

	previousMode := gin.Mode()
	gin.SetMode(gin.TestMode)
	t.Cleanup(func() {
		gin.SetMode(previousMode)
	})

	router := gin.New()
	router.POST("/login", D2SLoginActionLog(), func(c *gin.Context) {
		c.Header("Set-Cookie", "d2s_refresh=sensitive-token; HttpOnly")
		c.JSON(http.StatusOK, gin.H{
			"success":      true,
			"access_token": "sensitive-token",
		})
	})

	request := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(`{"username":"test-user","password":"secret"}`))
	request.RemoteAddr = "192.0.2.10:4567"
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected HTTP %d, got %d", http.StatusServiceUnavailable, recorder.Code)
	}
	if strings.Contains(recorder.Body.String(), "sensitive-token") {
		t.Fatal("login response was exposed even though audit storage failed")
	}
	if got := recorder.Header().Get("Set-Cookie"); got != "" {
		t.Fatalf("login cookie was exposed despite audit failure: %q", got)
	}
	if recorder.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("audit failure response must not be cached")
	}
	if !strings.Contains(recorder.Body.String(), "LOGIN_AUDIT_UNAVAILABLE") {
		t.Fatalf("expected stable audit failure code, got %s", recorder.Body.String())
	}
}

func TestD2SLoginActionLogFlushesResponseAfterAuditWrite(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:d2s_login_action_log_success_test?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.D2SUserActionLog{}); err != nil {
		t.Fatal(err)
	}
	previousDB := model.DB
	model.DB = db
	t.Cleanup(func() {
		model.DB = previousDB
	})

	previousMode := gin.Mode()
	gin.SetMode(gin.TestMode)
	t.Cleanup(func() {
		gin.SetMode(previousMode)
	})

	router := gin.New()
	router.POST("/login", D2SLoginActionLog(), func(c *gin.Context) {
		c.Header("Set-Cookie", "d2s_refresh=valid-session; HttpOnly")
		c.JSON(http.StatusOK, gin.H{"success": true, "access_token": "valid-token"})
	})

	request := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(`{"username":"test-user","password":"secret"}`))
	request.RemoteAddr = "192.0.2.12:6789"
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), "valid-token") {
		t.Fatalf("expected buffered login response after audit success, got status %d and body %s", recorder.Code, recorder.Body.String())
	}
	if got := recorder.Header().Get("Set-Cookie"); !strings.Contains(got, "valid-session") {
		t.Fatalf("expected login cookie after audit success, got %q", got)
	}
	var count int64
	if err := db.Model(&model.D2SUserActionLog{}).Where("action_type = ?", model.D2SActionLogin).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("expected one persisted login action, got %d", count)
	}
}

func TestD2SLoginActionLogAssociatesDeviceTokenEvents(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:d2s_device_login_action_log_test?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.D2SUserActionLog{}, &model.D2SDeviceCode{}); err != nil {
		t.Fatal(err)
	}
	previousDB := model.DB
	model.DB = db
	t.Cleanup(func() {
		model.DB = previousDB
	})

	secret := strings.Repeat("a", 64)
	secretHash := sha256.Sum256([]byte(secret))
	deviceHash := strings.Repeat("b", 64)
	if err := db.Create(&model.D2SDeviceCode{
		ID: "device-login-audit", DeviceCodeHash: hex.EncodeToString(secretHash[:]), UserCode: "ABCD-1234",
		UserID: 42, DeviceHash: deviceHash, FingerprintVersion: 1, Status: model.D2SDeviceCodeApproved,
		CreatedAt: 2_000_000_000, ExpiresAt: 2_000_000_600, ApprovedAt: 2_000_000_001,
	}).Error; err != nil {
		t.Fatal(err)
	}

	previousMode := gin.Mode()
	gin.SetMode(gin.TestMode)
	t.Cleanup(func() {
		gin.SetMode(previousMode)
	})

	router := gin.New()
	router.POST("/device/token", D2SLoginActionLog(), func(c *gin.Context) {
		c.JSON(http.StatusUnauthorized, gin.H{"version": 1, "success": false, "error": gin.H{"code": "invalid_device_code"}})
	})
	router.POST("/device/token-success", D2SLoginActionLog(), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"version": 1, "success": true, "data": gin.H{"user_id": 43, "device_hash": deviceHash}})
	})
	router.POST("/device/token-blocked", D2SLoginActionLog(), func(c *gin.Context) {
		c.JSON(http.StatusLocked, gin.H{"version": 1, "success": false, "code": "account_login_blocked"})
	})

	request := httptest.NewRequest(http.MethodPost, "/device/token", strings.NewReader(`{"device_code":"`+secret+`"}`))
	request.RemoteAddr = "192.0.2.21:4567"
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("expected failed device login, got HTTP %d", recorder.Code)
	}

	request = httptest.NewRequest(http.MethodPost, "/device/token-success", strings.NewReader(`{}`))
	request.RemoteAddr = "192.0.2.22:5678"
	recorder = httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected successful device login, got HTTP %d", recorder.Code)
	}

	request = httptest.NewRequest(http.MethodPost, "/device/token-blocked", strings.NewReader(`{"device_code":"`+secret+`"}`))
	request.RemoteAddr = "192.0.2.23:6789"
	recorder = httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusLocked {
		t.Fatalf("expected blocked device login, got HTTP %d", recorder.Code)
	}

	var failed model.D2SUserActionLog
	if err := db.Where("request_path = ?", "/device/token").First(&failed).Error; err != nil {
		t.Fatal(err)
	}
	if failed.UserID == nil || *failed.UserID != 42 || failed.ResultStatus != model.D2SActionResultFailure {
		t.Fatalf("failed device login was not associated with its account: %+v", failed)
	}
	if failed.DeviceFingerprint == nil || *failed.DeviceFingerprint != deviceHash {
		t.Fatalf("failed device login did not retain the device hash: %+v", failed.DeviceFingerprint)
	}

	var successful model.D2SUserActionLog
	if err := db.Where("request_path = ?", "/device/token-success").First(&successful).Error; err != nil {
		t.Fatal(err)
	}
	if successful.UserID == nil || *successful.UserID != 43 || successful.ResultStatus != model.D2SActionResultSuccess {
		t.Fatalf("successful device login was not associated with its account: %+v", successful)
	}
	if successful.DeviceFingerprint == nil || *successful.DeviceFingerprint != deviceHash {
		t.Fatalf("successful device login did not retain the device hash: %+v", successful.DeviceFingerprint)
	}

	var blocked model.D2SUserActionLog
	if err := db.Where("request_path = ?", "/device/token-blocked").First(&blocked).Error; err != nil {
		t.Fatal(err)
	}
	if blocked.UserID == nil || *blocked.UserID != 42 || blocked.ResultStatus != model.D2SActionResultBlocked {
		t.Fatalf("blocked device login was not retained as BLOCKED: %+v", blocked)
	}
	if blocked.DeviceFingerprint == nil || *blocked.DeviceFingerprint != deviceHash {
		t.Fatalf("blocked device login did not retain the device hash: %+v", blocked.DeviceFingerprint)
	}
}
