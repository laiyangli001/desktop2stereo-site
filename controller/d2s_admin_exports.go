package controller

import (
	"encoding/csv"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

type d2sActionLogFilters struct {
	startTime int64
	endTime   int64
	userID    *int
	email     string
	sourceIP  string
	action    string
	page      int
	pageSize  int
}

func parseD2SUnixTime(c *gin.Context, key string) (int64, error) {
	raw := strings.TrimSpace(c.Query(key))
	if raw == "" {
		return 0, nil
	}
	if value, err := strconv.ParseInt(raw, 10, 64); err == nil {
		return value, nil
	}
	parsed, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return 0, err
	}
	return parsed.UTC().Unix(), nil
}

func parseD2SActionLogFilters(c *gin.Context, paged bool) (d2sActionLogFilters, error) {
	f := d2sActionLogFilters{
		email: strings.TrimSpace(c.Query("email")), sourceIP: strings.TrimSpace(c.Query("source_ip")),
		action: strings.ToUpper(strings.TrimSpace(c.Query("action_type"))), page: 1, pageSize: 50,
	}
	var err error
	f.startTime, err = parseD2SUnixTime(c, "start_time")
	if err != nil {
		return f, err
	}
	f.endTime, err = parseD2SUnixTime(c, "end_time")
	if err != nil || (f.startTime > 0 && f.endTime > 0 && f.startTime > f.endTime) {
		return f, fmt.Errorf("invalid time range")
	}
	if raw := strings.TrimSpace(c.Query("user_id")); raw != "" {
		id, parseErr := strconv.Atoi(raw)
		if parseErr != nil || id <= 0 {
			return f, fmt.Errorf("invalid user id")
		}
		f.userID = &id
	}
	if paged {
		if raw := c.Query("page"); raw != "" {
			f.page, err = strconv.Atoi(raw)
			if err != nil || f.page < 1 {
				return f, fmt.Errorf("invalid page")
			}
		}
		if raw := c.Query("page_size"); raw != "" {
			f.pageSize, err = strconv.Atoi(raw)
			if err != nil || f.pageSize < 1 || f.pageSize > 500 {
				return f, fmt.Errorf("invalid page size")
			}
		}
	}
	return f, nil
}

func d2sFilterDetails(f d2sActionLogFilters) map[string]any {
	return map[string]any{
		"start_time": f.startTime, "end_time": f.endTime, "user_id": f.userID,
		"email": f.email, "source_ip": f.sourceIP, "action_type": f.action,
	}
}

func D2SAdminUserActionLogs(c *gin.Context) {
	f, err := parseD2SActionLogFilters(c, true)
	if err != nil {
		respondD2SAdminAuditedFailure(c, "VIEW_USER_LOG", "d2s_user_action_logs", "invalid_filter", http.StatusBadRequest, "INVALID_FILTER")
		return
	}
	entries, total, err := model.ListD2SUserActionLogs(f.startTime, f.endTime, f.userID, f.email, f.sourceIP, f.action, f.page, f.pageSize)
	if err != nil {
		respondD2SAdminAuditedFailure(c, "VIEW_USER_LOG", "d2s_user_action_logs", "query_failed", http.StatusInternalServerError, "LOG_QUERY_FAILED")
		return
	}
	details := d2sFilterDetails(f)
	details["page"], details["page_size"], details["total"] = f.page, f.pageSize, total
	if err := writeD2SAdminAudit(c, "VIEW_USER_LOG", "d2s_user_action_logs", "SUCCESS", details); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "code": "AUDIT_WRITE_FAILED"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"items": entries, "total": total, "page": f.page, "page_size": f.pageSize}})
}

func D2SAdminVerifyUserActionLog(c *gin.Context) {
	id := strings.TrimSpace(c.Param("id"))
	consistent, err := model.VerifyD2SUserActionLogHash(id)
	result := "SUCCESS"
	if err != nil {
		result = "FAILURE"
	}
	if auditErr := writeD2SAdminAudit(c, "VERIFY_USER_LOG_HASH", id, result, map[string]any{"consistent": consistent}); auditErr != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "code": "AUDIT_WRITE_FAILED"})
		return
	}
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "code": "LOG_NOT_FOUND"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"log_id": id, "consistent": consistent}})
}

func d2sCSVCell(value string) string {
	trimmed := strings.TrimLeft(value, " \t\r\n")
	if len(trimmed) > 0 && strings.ContainsRune("=+-@'", rune(trimmed[0])) {
		return "'" + value
	}
	return value
}

func d2sCSVTime(seconds int64) string {
	if seconds <= 0 {
		return ""
	}
	return time.Unix(seconds, 0).UTC().Format(time.RFC3339)
}

func d2sCSVInt(value *int) string {
	if value == nil {
		return ""
	}
	return strconv.Itoa(*value)
}

func d2sCSVString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func writeD2SCSV(c *gin.Context, operationType, filename string, header []string, rows func(*csv.Writer) error) {
	c.Header("Content-Type", "text/csv; charset=utf-8")
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filename))
	c.Header("Cache-Control", "no-store")
	c.Status(http.StatusOK)
	writer := csv.NewWriter(c.Writer)
	if err := writer.Write(header); err != nil {
		recordD2SCSVTerminalAudit(c, operationType, filename, "FAILURE", "header_write_failed")
		return
	}
	if err := rows(writer); err != nil {
		recordD2SCSVTerminalAudit(c, operationType, filename, "FAILURE", "stream_write_failed")
		return
	}
	writer.Flush()
	if err := writer.Error(); err != nil {
		recordD2SCSVTerminalAudit(c, operationType, filename, "FAILURE", "stream_flush_failed")
		return
	}
	if err := writeD2SAdminAudit(c, operationType, filename, "SUCCESS", map[string]any{"phase": "stream_complete"}); err != nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("D2S CSV terminal audit failed: operation=%s filename=%s", operationType, filename))
	}
}

func recordD2SCSVTerminalAudit(c *gin.Context, operationType, filename, result, reason string) {
	if err := writeD2SAdminAudit(c, operationType, filename, result, map[string]any{"reason": reason}); err != nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("D2S CSV terminal audit failed: operation=%s filename=%s result=%s", operationType, filename, result))
	}
}

func requireD2SExportApproval(c *gin.Context, approvalID, exportType, target string, snapshot map[string]any) bool {
	err := consumeD2SExportApproval(c, approvalID, exportType, snapshot)
	if err == nil {
		return true
	}
	status, code, reason := http.StatusForbidden, "EXPORT_APPROVAL_REQUIRED", "approval_missing_or_invalid"
	switch {
	case errors.Is(err, model.ErrD2SExportApprovalExpired):
		status, code, reason = http.StatusGone, "EXPORT_APPROVAL_EXPIRED", "approval_expired"
	case errors.Is(err, model.ErrD2SExportApprovalNotFound):
		status, code, reason = http.StatusForbidden, "EXPORT_APPROVAL_REQUIRED", "approval_not_found"
	case errors.Is(err, model.ErrD2SExportApprovalFilterMismatch):
		status, code, reason = http.StatusForbidden, "EXPORT_APPROVAL_SCOPE_MISMATCH", "scope_mismatch"
	case errors.Is(err, model.ErrD2SExportApprovalNotApproved):
		status, code, reason = http.StatusForbidden, "EXPORT_APPROVAL_REQUIRED", "approval_not_approved_or_already_used"
	case !errors.Is(err, model.ErrD2SExportApprovalInvalid):
		status, code, reason = http.StatusInternalServerError, "EXPORT_APPROVAL_CHECK_FAILED", "approval_check_failed"
	}
	respondD2SAdminAuditedFailure(c, "EXPORT_LOG", target, reason, status, code)
	return false
}

func D2SAdminExportUserActionLogs(c *gin.Context) {
	f, err := parseD2SActionLogFilters(c, false)
	if err != nil {
		respondD2SAdminAuditedFailure(c, "EXPORT_LOG", "d2s_user_action_logs", "invalid_filter", http.StatusBadRequest, "INVALID_FILTER")
		return
	}
	_, total, err := model.ListD2SUserActionLogs(f.startTime, f.endTime, f.userID, f.email, f.sourceIP, f.action, 1, 1)
	if err != nil {
		respondD2SAdminAuditedFailure(c, "EXPORT_LOG", "d2s_user_action_logs", "query_failed", http.StatusInternalServerError, "LOG_QUERY_FAILED")
		return
	}
	if !requireD2SExportApproval(c, c.Query("approval_id"), model.D2SExportApprovalTypeUserActionLogs, "d2s_user_action_logs", d2sFilterDetails(f)) {
		return
	}
	filename := "access_log_" + time.Now().UTC().Format("20060102_150405") + ".csv"
	details := d2sFilterDetails(f)
	details["record_count"], details["approval_id"] = total, c.Query("approval_id")
	if err := writeD2SAdminAudit(c, "EXPORT_LOG", filename, "STARTED", details); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "code": "AUDIT_WRITE_FAILED"})
		return
	}
	header := []string{"id", "user_id", "email", "action_type", "event_time", "source_ip", "source_port", "destination_ip", "destination_port", "user_agent", "device_fingerprint", "request_path", "result_status", "record_hash"}
	writeD2SCSV(c, "EXPORT_LOG", filename, header, func(writer *csv.Writer) error {
		for page := 1; ; page++ {
			entries, _, queryErr := model.ListD2SUserActionLogs(f.startTime, f.endTime, f.userID, f.email, f.sourceIP, f.action, page, 500)
			if queryErr != nil {
				return queryErr
			}
			for _, entry := range entries {
				userID := ""
				if entry.UserID != nil {
					userID = strconv.Itoa(*entry.UserID)
				}
				row := []string{entry.ID, userID, entry.Email, entry.ActionType, d2sCSVTime(entry.EventTime), entry.SourceIP,
					d2sCSVInt(entry.SourcePort), d2sCSVString(entry.DestinationIP), d2sCSVInt(entry.DestinationPort),
					entry.UserAgent, d2sCSVString(entry.DeviceFingerprint), entry.RequestPath, entry.ResultStatus, entry.RecordHash}
				for i := range row {
					row[i] = d2sCSVCell(row[i])
				}
				if err := writer.Write(row); err != nil {
					return err
				}
			}
			if len(entries) < 500 {
				return nil
			}
		}
	})
}

func D2SAdminExportRegistrationRecords(c *gin.Context) {
	startTime, err := parseD2SUnixTime(c, "start_time")
	if err != nil {
		respondD2SAdminAuditedFailure(c, "EXPORT_REGISTRATION", "d2s_registration_records", "invalid_time_range", http.StatusBadRequest, "INVALID_TIME_RANGE")
		return
	}
	endTime, err := parseD2SUnixTime(c, "end_time")
	if err != nil || (startTime > 0 && endTime > 0 && startTime > endTime) {
		respondD2SAdminAuditedFailure(c, "EXPORT_REGISTRATION", "d2s_registration_records", "invalid_time_range", http.StatusBadRequest, "INVALID_TIME_RANGE")
		return
	}
	_, total, err := model.ListD2SRegistrationRecords(startTime, endTime, 1, 1)
	if err != nil {
		respondD2SAdminAuditedFailure(c, "EXPORT_REGISTRATION", "d2s_registration_records", "query_failed", http.StatusInternalServerError, "REGISTRATION_QUERY_FAILED")
		return
	}
	if !requireD2SExportApproval(c, c.Query("approval_id"), model.D2SExportApprovalTypeRegistrations, "d2s_registration_records", map[string]any{"start_time": startTime, "end_time": endTime}) {
		return
	}
	filename := "registration_records_" + time.Now().UTC().Format("20060102_150405") + ".csv"
	if err := writeD2SAdminAudit(c, "EXPORT_REGISTRATION", filename, "STARTED", map[string]any{"start_time": startTime, "end_time": endTime, "record_count": total, "approval_id": c.Query("approval_id")}); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "code": "AUDIT_WRITE_FAILED"})
		return
	}
	header := []string{"user_id", "email", "register_ip", "register_time", "register_user_agent", "email_verified", "email_verified_at"}
	writeD2SCSV(c, "EXPORT_REGISTRATION", filename, header, func(writer *csv.Writer) error {
		for page := 1; ; page++ {
			entries, _, queryErr := model.ListD2SRegistrationRecords(startTime, endTime, page, 500)
			if queryErr != nil {
				return queryErr
			}
			for _, entry := range entries {
				row := []string{strconv.Itoa(entry.UserID), entry.Email, entry.RegisterIP, d2sCSVTime(entry.RegisterTime), entry.RegisterUserAgent, strconv.FormatBool(entry.EmailVerified), d2sCSVTime(entry.EmailVerifiedAt)}
				for i := range row {
					row[i] = d2sCSVCell(row[i])
				}
				if err := writer.Write(row); err != nil {
					return err
				}
			}
			if len(entries) < 500 {
				return nil
			}
		}
	})
}
