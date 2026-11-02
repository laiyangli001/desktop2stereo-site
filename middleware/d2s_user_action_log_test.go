package middleware

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

func TestD2SUserActionLogFailsClosedWhenAuditStorageIsUnavailable(t *testing.T) {
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
	router.POST("/purchase", D2SUserActionLog(model.D2SActionPurchase), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"success": true, "order_id": "sensitive-order"})
	})

	request := httptest.NewRequest(http.MethodPost, "/purchase", strings.NewReader(`{"amount_minor":1990}`))
	request.RemoteAddr = "192.0.2.11:5678"
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected HTTP %d, got %d", http.StatusServiceUnavailable, recorder.Code)
	}
	if strings.Contains(recorder.Body.String(), "sensitive-order") {
		t.Fatal("purchase response was exposed even though audit storage failed")
	}
	if recorder.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("audit failure response must not be cached")
	}
	if !strings.Contains(recorder.Body.String(), "USER_ACTION_AUDIT_UNAVAILABLE") {
		t.Fatalf("expected stable audit failure code, got %s", recorder.Body.String())
	}
}

func TestD2SUserActionLogPreservesRequestBodyBeyondInspectionLimit(t *testing.T) {
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

	payload := strings.Repeat("x", 70*1024)
	var controllerRead int
	router := gin.New()
	router.POST("/purchase", D2SUserActionLog(model.D2SActionPurchase), func(c *gin.Context) {
		body, err := io.ReadAll(c.Request.Body)
		if err != nil {
			t.Errorf("controller could not read request body: %v", err)
			return
		}
		controllerRead = len(body)
		c.JSON(http.StatusOK, gin.H{"success": true})
	})

	request := httptest.NewRequest(http.MethodPost, "/purchase", strings.NewReader(payload))
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)

	if controllerRead != len(payload) {
		t.Fatalf("expected controller to receive all %d request bytes, got %d", len(payload), controllerRead)
	}
	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected audit failure status, got %d", recorder.Code)
	}
}
