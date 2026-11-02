package router

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestUnauthenticatedD2SPurchaseAttemptsAreLogged(t *testing.T) {
	database, err := gorm.Open(sqlite.Open("file:d2s_purchase_audit_test?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	if err := database.AutoMigrate(&model.D2SUserActionLog{}); err != nil {
		t.Fatalf("migrate action log table: %v", err)
	}

	previousDB := model.DB
	model.DB = database
	t.Cleanup(func() { model.DB = previousDB })

	previousMode := gin.Mode()
	gin.SetMode(gin.TestMode)
	t.Cleanup(func() { gin.SetMode(previousMode) })

	engine := gin.New()
	registerD2SRoutes(engine.Group("/api"), func(c *gin.Context) { c.Next() })

	purchaseRoutes := []struct {
		name        string
		requestPath string
		auditPath   string
	}{
		{name: "order preview", requestPath: "/api/v1/orders/preview", auditPath: "/api/v1/orders/preview"},
		{name: "order create", requestPath: "/api/v1/orders/create", auditPath: "/api/v1/orders/create"},
		{name: "order checkout", requestPath: "/api/v1/orders/order-test/checkout", auditPath: "/api/v1/orders/:id/checkout"},
		{name: "paid revoke", requestPath: "/api/v1/license/revoke/paid", auditPath: "/api/v1/license/revoke/paid"},
		{name: "offline extend", requestPath: "/api/v1/license/offline/extend", auditPath: "/api/v1/license/offline/extend"},
	}
	for _, route := range purchaseRoutes {
		t.Run(route.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodPost, route.requestPath, strings.NewReader("{}"))
			request.Header.Set("Content-Type", "application/json")
			request.RemoteAddr = "203.0.113.9:43120"
			response := httptest.NewRecorder()
			engine.ServeHTTP(response, request)

			if response.Code != http.StatusUnauthorized {
				t.Fatalf("expected unauthenticated purchase attempt to return 401, got %d: %s", response.Code, response.Body.String())
			}

			var entry model.D2SUserActionLog
			if err := database.Where("request_path = ?", route.auditPath).First(&entry).Error; err != nil {
				t.Fatalf("expected purchase attempt audit record: %v", err)
			}
			if entry.ActionType != model.D2SActionPurchase {
				t.Errorf("expected action type %q, got %q", model.D2SActionPurchase, entry.ActionType)
			}
			if entry.ResultStatus != model.D2SActionResultFailure {
				t.Errorf("expected result status %q, got %q", model.D2SActionResultFailure, entry.ResultStatus)
			}
			if entry.SourceIP != "203.0.113.9" {
				t.Errorf("expected source IP to be recorded, got %q", entry.SourceIP)
			}
		})
	}
}
