package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func TestD2SAdminReconciliationAuthAcceptsMachineToken(t *testing.T) {
	gin.SetMode(gin.TestMode)
	token := strings.Repeat("a", 64)
	t.Setenv("D2S_RECONCILIATION_TOKEN", token)

	router := gin.New()
	router.GET("/reconciliation", D2SAdminReconciliationAuth(), D2SAdminRequireRole("super_admin"), func(c *gin.Context) {
		c.String(http.StatusOK, c.GetString("d2s_admin_email"))
	})
	request := httptest.NewRequest(http.MethodGet, "/reconciliation", nil)
	request.Header.Set("Authorization", "Bearer "+token)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)

	assert.Equal(t, http.StatusOK, recorder.Code)
	assert.Equal(t, "system:payment-reconciliation", recorder.Body.String())
}

func TestD2SAdminReconciliationAuthRejectsUnavailableOrInvalidToken(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/reconciliation", D2SAdminReconciliationAuth(), func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	t.Setenv("D2S_RECONCILIATION_TOKEN", "")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/reconciliation", nil))
	assert.Equal(t, http.StatusServiceUnavailable, recorder.Code)

	t.Setenv("D2S_RECONCILIATION_TOKEN", strings.Repeat("a", 64))
	request := httptest.NewRequest(http.MethodGet, "/reconciliation", nil)
	request.Header.Set("Authorization", "Bearer "+strings.Repeat("b", 64))
	recorder = httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	assert.Equal(t, http.StatusUnauthorized, recorder.Code)

	adminRouter := gin.New()
	adminRouter.GET("/admin", D2SAdminAuth(), func(c *gin.Context) {
		c.Status(http.StatusOK)
	})
	request = httptest.NewRequest(http.MethodGet, "/admin", nil)
	request.Header.Set("Authorization", "Bearer "+strings.Repeat("a", 64))
	recorder = httptest.NewRecorder()
	adminRouter.ServeHTTP(recorder, request)
	assert.Equal(t, http.StatusUnauthorized, recorder.Code)
}
