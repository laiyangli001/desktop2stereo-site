package controller

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func TestD2SSpecializedOrderEndpointsRejectWrongProduct(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tests := []struct {
		name    string
		handler gin.HandlerFunc
		product string
	}{
		{name: "paid revoke", handler: D2SLicensePaidRevoke, product: "license"},
		{name: "offline extension", handler: D2SLicenseOfflineExtend, product: "paid_revoke"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			context, _ := gin.CreateTestContext(recorder)
			context.Set("id", 1)
			context.Request = httptest.NewRequest(
				http.MethodPost,
				"/api/v1/license/"+strings.ReplaceAll(test.name, " ", "/"),
				strings.NewReader(`{"product":"`+test.product+`","provider":"stripe","idempotency_key":"wrong-product"}`),
			)

			test.handler(context)

			assert.Equal(t, http.StatusBadRequest, recorder.Code)
		})
	}
}
