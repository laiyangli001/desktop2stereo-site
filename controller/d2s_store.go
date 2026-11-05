package controller

import (
	"net/http"

	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

func D2SStoreProducts(c *gin.Context) {
	type product struct {
		ID         string           `json:"id"`
		Prices     map[string]int64 `json:"prices"`
		StartingAt bool             `json:"starting_at"`
	}
	products := make([]product, 0, 3)
	for _, id := range []string{model.D2SOrderProductLicense, model.D2SOrderProductPaidRevoke, model.D2SOrderProductOfflineExtension} {
		row := product{ID: id, Prices: make(map[string]int64), StartingAt: id == model.D2SOrderProductPaidRevoke}
		for _, currency := range []string{"CNY", "USD"} {
			amount, err := model.D2SProductPrice(id, currency, 1)
			if err == nil {
				row.Prices[currency] = amount
			}
		}
		products = append(products, row)
	}
	d2sSuccess(c, http.StatusOK, gin.H{"products": products})
}
