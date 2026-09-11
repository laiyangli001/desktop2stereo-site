package controller

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

func TestD2SErrorMapsInvalidOfflinePeriod(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)

	d2sError(context, model.ErrD2SOfflinePeriodInvalid)

	assert.Equal(t, http.StatusBadRequest, recorder.Code)
	var response struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &response))
	assert.Equal(t, "invalid_offline_period", response.Error.Code)
}
