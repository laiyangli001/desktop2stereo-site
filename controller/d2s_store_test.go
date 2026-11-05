package controller

import (
	"github.com/QuantumNous/new-api/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestD2SStoreCatalogShowsPricesWithoutOpeningPurchases(t *testing.T) {
	t.Setenv("D2S_PURCHASES_ENABLED", "false")
	t.Setenv("D2S_OFFLINE_EXTENSION_CNY_MINOR", "1990")
	t.Setenv("D2S_OFFLINE_EXTENSION_USD_MINOR", "")
	gin.SetMode(gin.TestMode)
	response := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(response)
	D2SStoreProducts(c)
	require.Equal(t, http.StatusOK, response.Code)
	var payload struct {
		Success bool `json:"success"`
		Data    struct {
			Products []struct {
				ID         string           `json:"id"`
				Prices     map[string]int64 `json:"prices"`
				StartingAt bool             `json:"starting_at"`
			} `json:"products"`
		} `json:"data"`
	}
	require.NoError(t, common.Unmarshal(response.Body.Bytes(), &payload))
	require.True(t, payload.Success)
	require.Len(t, payload.Data.Products, 3)
	require.EqualValues(t, 9900, payload.Data.Products[0].Prices["CNY"])
	require.EqualValues(t, 2990, payload.Data.Products[0].Prices["USD"])
	require.True(t, payload.Data.Products[1].StartingAt)
	require.EqualValues(t, 1990, payload.Data.Products[2].Prices["CNY"])
	require.NotContains(t, payload.Data.Products[2].Prices, "USD", "missing price must not be advertised as zero")
}
