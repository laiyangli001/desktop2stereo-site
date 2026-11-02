package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/controller"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestRejectedRegistrationAttemptIsAudited(t *testing.T) {
	previousDB := model.DB
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	sqlDB.SetMaxOpenConns(1)
	if err := db.AutoMigrate(&model.User{}, &model.D2SUserActionLog{}); err != nil {
		t.Fatal(err)
	}
	model.DB = db

	previousRegisterEnabled := common.RegisterEnabled
	previousPasswordRegisterEnabled := common.PasswordRegisterEnabled
	common.RegisterEnabled = true
	common.PasswordRegisterEnabled = true

	previousMode := gin.Mode()
	gin.SetMode(gin.TestMode)
	t.Cleanup(func() {
		model.DB = previousDB
		common.RegisterEnabled = previousRegisterEnabled
		common.PasswordRegisterEnabled = previousPasswordRegisterEnabled
		gin.SetMode(previousMode)
		_ = sqlDB.Close()
	})

	router := gin.New()
	router.POST("/api/user/register", middleware.D2SUserActionLog(model.D2SActionRegister), controller.Register)

	request := httptest.NewRequest(http.MethodPost, "/api/user/register", strings.NewReader(
		`{"email":"Rejected-User@Example.Test","username":"rejected-user"}`))
	request.RemoteAddr = "203.0.113.77:52134"
	request.Header.Set("User-Agent", "d2s-registration-audit-test")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("expected HTTP %d for missing behavior captcha, got %d: %s", http.StatusBadRequest, recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), "BEHAVIOR_CAPTCHA_REQUIRED") {
		t.Fatalf("expected behavior captcha rejection, got %s", recorder.Body.String())
	}

	var entries []model.D2SUserActionLog
	if err := db.Order("event_time ASC").Find(&entries).Error; err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected one registration audit event, got %d", len(entries))
	}
	entry := entries[0]
	if entry.UserID != nil {
		t.Fatalf("rejected unregistered account should not have user_id, got %d", *entry.UserID)
	}
	if entry.Email != "rejected-user@example.test" {
		t.Fatalf("expected normalized email snapshot, got %q", entry.Email)
	}
	if entry.ActionType != model.D2SActionRegister {
		t.Fatalf("expected REGISTER action, got %q", entry.ActionType)
	}
	if entry.ResultStatus != model.D2SActionResultFailure {
		t.Fatalf("expected FAILURE status for rejected registration, got %q", entry.ResultStatus)
	}
	if entry.SourceIP != "203.0.113.77" {
		t.Fatalf("expected source IP to be recorded, got %q", entry.SourceIP)
	}
	if entry.UserAgent != "d2s-registration-audit-test" {
		t.Fatalf("expected user agent to be recorded, got %q", entry.UserAgent)
	}
	if entry.RequestPath != "/api/user/register" {
		t.Fatalf("expected registration route to be recorded, got %q", entry.RequestPath)
	}
	if entry.RecordHash == "" {
		t.Fatal("expected rejected registration event to have a record hash")
	}
}
