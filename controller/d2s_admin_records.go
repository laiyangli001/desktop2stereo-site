package controller

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

func D2SAdminRegistrationRecords(c *gin.Context) {
	start, err := parseD2SUnixTime(c, "start_time")
	if err != nil {
		respondD2SAdminAuditedFailure(c, "VIEW_REGISTRATION", "d2s_registration_records", "invalid_time_range", http.StatusBadRequest, "INVALID_TIME_RANGE")
		return
	}
	end, err := parseD2SUnixTime(c, "end_time")
	if err != nil || (start > 0 && end > 0 && start > end) {
		respondD2SAdminAuditedFailure(c, "VIEW_REGISTRATION", "d2s_registration_records", "invalid_time_range", http.StatusBadRequest, "INVALID_TIME_RANGE")
		return
	}
	page, pageSize := 1, 50
	if c.Query("page") != "" {
		page, err = strconv.Atoi(c.Query("page"))
	}
	if err != nil || page < 1 {
		respondD2SAdminAuditedFailure(c, "VIEW_REGISTRATION", "d2s_registration_records", "invalid_page", http.StatusBadRequest, "INVALID_PAGE")
		return
	}
	if c.Query("page_size") != "" {
		pageSize, err = strconv.Atoi(c.Query("page_size"))
	}
	if err != nil || pageSize < 1 || pageSize > 500 {
		respondD2SAdminAuditedFailure(c, "VIEW_REGISTRATION", "d2s_registration_records", "invalid_page_size", http.StatusBadRequest, "INVALID_PAGE_SIZE")
		return
	}
	items, total, err := model.ListD2SRegistrationRecords(start, end, page, pageSize)
	if err != nil {
		respondD2SAdminAuditedFailure(c, "VIEW_REGISTRATION", "d2s_registration_records", "query_failed", http.StatusInternalServerError, "REGISTRATION_QUERY_FAILED")
		return
	}
	if err := writeD2SAdminAudit(c, "VIEW_REGISTRATION", "d2s_registration_records", "SUCCESS", map[string]any{
		"start_time": start, "end_time": end, "page": page, "page_size": pageSize, "total": total,
	}); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "code": "AUDIT_WRITE_FAILED"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"items": items, "total": total, "page": page, "page_size": pageSize}})
}

func D2SAdminPurchaseRecords(c *gin.Context) {
	page, pageSize := 1, 50
	var err error
	if c.Query("page") != "" {
		page, err = strconv.Atoi(c.Query("page"))
	}
	if err != nil || page < 1 {
		respondD2SAdminAuditedFailure(c, "VIEW_PURCHASE", "d2s_orders", "invalid_page", http.StatusBadRequest, "INVALID_PAGE")
		return
	}
	if c.Query("page_size") != "" {
		pageSize, err = strconv.Atoi(c.Query("page_size"))
	}
	if err != nil || pageSize < 1 || pageSize > 500 {
		respondD2SAdminAuditedFailure(c, "VIEW_PURCHASE", "d2s_orders", "invalid_page_size", http.StatusBadRequest, "INVALID_PAGE_SIZE")
		return
	}
	start, err := parseD2SUnixTime(c, "start_time")
	if err != nil {
		respondD2SAdminAuditedFailure(c, "VIEW_PURCHASE", "d2s_orders", "invalid_time_range", http.StatusBadRequest, "INVALID_TIME_RANGE")
		return
	}
	end, err := parseD2SUnixTime(c, "end_time")
	if err != nil || (start > 0 && end > 0 && start > end) {
		respondD2SAdminAuditedFailure(c, "VIEW_PURCHASE", "d2s_orders", "invalid_time_range", http.StatusBadRequest, "INVALID_TIME_RANGE")
		return
	}
	query := model.DB.Model(&model.D2SOrder{})
	if start > 0 {
		query = query.Where("purchase_time >= ?", start)
	}
	if end > 0 {
		query = query.Where("purchase_time <= ?", end)
	}
	var parsedUserID *int
	if raw := strings.TrimSpace(c.Query("user_id")); raw != "" {
		userID, parseErr := strconv.Atoi(raw)
		if parseErr != nil || userID <= 0 {
			respondD2SAdminAuditedFailure(c, "VIEW_PURCHASE", "d2s_orders", "invalid_user_id", http.StatusBadRequest, "INVALID_USER_ID")
			return
		}
		parsedUserID = &userID
		query = query.Where("user_id = ?", userID)
	}
	emailFilter := model.NormalizeEmail(c.Query("email"))
	if emailFilter != "" {
		query = query.Where("email_snapshot = ?", emailFilter)
	}
	statusFilter := strings.ToUpper(strings.TrimSpace(c.Query("result_status")))
	if statusFilter != "" {
		if statusFilter != "SUCCESS" && statusFilter != "FAILURE" && statusFilter != "BLOCKED" {
			respondD2SAdminAuditedFailure(c, "VIEW_PURCHASE", "d2s_orders", "invalid_result_status", http.StatusBadRequest, "INVALID_RESULT_STATUS")
			return
		}
		query = query.Where("result_status = ?", statusFilter)
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		respondD2SAdminAuditedFailure(c, "VIEW_PURCHASE", "d2s_orders", "query_failed", http.StatusInternalServerError, "PURCHASE_QUERY_FAILED")
		return
	}
	var entries []model.D2SOrder
	if err := query.Order("purchase_time DESC").Order("id DESC").Offset((page - 1) * pageSize).Limit(pageSize).Find(&entries).Error; err != nil {
		respondD2SAdminAuditedFailure(c, "VIEW_PURCHASE", "d2s_orders", "query_failed", http.StatusInternalServerError, "PURCHASE_QUERY_FAILED")
		return
	}
	if err := writeD2SAdminAudit(c, "VIEW_PURCHASE", "d2s_orders", "SUCCESS", map[string]any{
		"start_time": start, "end_time": end, "user_id": parsedUserID, "email": emailFilter, "result_status": statusFilter,
		"page": page, "page_size": pageSize, "total": total,
	}); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "code": "AUDIT_WRITE_FAILED"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"items": entries, "total": total, "page": page, "page_size": pageSize}})
}

func D2SAdminExecuteAuditRetention(c *gin.Context) {
	var request struct {
		RetentionDays int `json:"retention_days"`
	}
	if err := common.DecodeJson(c.Request.Body, &request); err != nil {
		respondD2SAdminAuditedFailure(c, "AUDIT_RETENTION_CLEANUP", "d2s_audit_data", "invalid_request", http.StatusBadRequest, "INVALID_REQUEST")
		return
	}
	run, err := model.PruneD2SAuditData(request.RetentionDays, c.GetInt("d2s_admin_id"), c.GetString("d2s_admin_email"), c.ClientIP(), time.Now().UTC().Unix())
	if err != nil {
		if auditErr := writeD2SAdminAudit(c, "AUDIT_RETENTION_CLEANUP", "d2s_audit_data", "FAILURE", map[string]any{
			"retention_days": request.RetentionDays, "reason": err.Error(),
		}); auditErr != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"success": false, "code": "AUDIT_WRITE_FAILED"})
			return
		}
		status, code := http.StatusInternalServerError, "RETENTION_FAILED"
		if errors.Is(err, model.ErrD2SAuditRetention) {
			status, code = http.StatusBadRequest, "RETENTION_REJECTED"
		} else if errors.Is(err, model.ErrD2SAuditCleanupUnavailable) {
			status, code = http.StatusServiceUnavailable, "RETENTION_UNAVAILABLE"
		}
		c.JSON(status, gin.H{"success": false, "code": code})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": run})
}
