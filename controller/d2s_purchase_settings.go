package controller

import (
	"net/http"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

func D2SSystemPurchaseSettings(c *gin.Context) {
	if c.Request.Method == http.MethodGet {
		enabled, err := model.D2SPurchasesEnabled()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"success": false, "code": "SETTINGS_UNAVAILABLE"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"enabled": enabled}})
		return
	}
	var request struct {
		Enabled *bool `json:"enabled"`
	}
	if err := common.DecodeJson(c.Request.Body, &request); err != nil || request.Enabled == nil {
		respondD2SAdminAuditedFailure(c, "UPDATE_PURCHASE_SETTINGS", model.D2SPurchasesEnabledOption, "invalid_request", http.StatusBadRequest, "INVALID_PURCHASE_SETTINGS")
		return
	}
	audit, err := d2SAdminAuditEntry(c, "UPDATE_PURCHASE_SETTINGS", model.D2SPurchasesEnabledOption, "SUCCESS", map[string]any{"enabled": *request.Enabled})
	if err == nil {
		err = model.SetD2SPurchasesEnabled(*request.Enabled, audit)
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "code": "SETTINGS_UPDATE_FAILED"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"enabled": *request.Enabled}})
}
