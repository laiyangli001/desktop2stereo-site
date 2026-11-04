package controller

import (
	"net/http"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

type d2SAlipayQRModeRequest struct {
	QRPayMode string `json:"qr_pay_mode"`
}

func D2SAdminAlipayPCSettings(c *gin.Context) {
	settings, err := model.GetD2SAlipayPCSettings()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "code": "SETTINGS_UNAVAILABLE"})
		return
	}
	if c.Request.Method == http.MethodGet {
		c.JSON(http.StatusOK, gin.H{"success": true, "data": settings})
		return
	}
	var request d2SAlipayQRModeRequest
	if err := common.DecodeJson(c.Request.Body, &request); err != nil || !validAlipayQRPayMode(strings.TrimSpace(request.QRPayMode)) {
		respondD2SAdminAuditedFailure(c, "UPDATE_ALIPAY_QR_MODE", "alipay_pc_settings", "invalid_qr_pay_mode", http.StatusBadRequest, "INVALID_QR_PAY_MODE")
		return
	}
	mode := strings.TrimSpace(request.QRPayMode)
	if err := writeD2SAdminAudit(c, "UPDATE_ALIPAY_QR_MODE", "alipay_pc_settings", "REQUESTED", map[string]any{"qr_pay_mode": mode}); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "code": "AUDIT_WRITE_FAILED"})
		return
	}
	updated, err := model.SetD2SAlipayPCQRPayMode(mode, time.Now().UTC().Unix())
	result := "SUCCESS"
	if err != nil {
		result = "FAILURE"
	}
	auditErr := writeD2SAdminAudit(c, "UPDATE_ALIPAY_QR_MODE_RESULT", "alipay_pc_settings", result, map[string]any{"qr_pay_mode": mode})
	if auditErr != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "code": "AUDIT_WRITE_FAILED"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "code": "SETTINGS_UPDATE_FAILED"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": updated})
}
