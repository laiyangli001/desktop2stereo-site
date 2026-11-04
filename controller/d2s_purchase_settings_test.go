package controller

import (
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestD2SPurchaseAdminSwitchClosesAllEntryPoints(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Option{}, &model.D2SAdminOperationAudit{}))
	previous := model.DB
	model.DB = db
	t.Cleanup(func() { model.DB = previous })
	t.Setenv("D2S_PURCHASES_ENABLED", "true")
	require.NoError(t, model.SetD2SPurchasesEnabled(false, &model.D2SAdminOperationAudit{
		OperatorID: 1, OperatorEmail: "admin@example.com", OperationType: "UPDATE_PURCHASE_SETTINGS",
	}))
	for _, handler := range []gin.HandlerFunc{D2SOrderPreview, D2SOrderCreate, D2SOrderCheckout, D2SLicensePaidRevoke, D2SLicenseOfflineExtend} {
		recorder := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(recorder)
		c.Set("id", 1)
		c.Request = httptest.NewRequest(http.MethodPost, "/", strings.NewReader("{}"))
		handler(c)
		require.Equal(t, http.StatusServiceUnavailable, recorder.Code, recorder.Body.String())
		require.Contains(t, recorder.Body.String(), "purchases_disabled")
	}
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	D2SOrderProviders(c)
	require.Equal(t, http.StatusOK, recorder.Code)
	require.Contains(t, recorder.Body.String(), `"enabled":false`)
}
