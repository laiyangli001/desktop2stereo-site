package controller

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

const d2sAdminExportApprovalLifetime = int64(30 * time.Minute / time.Second)

type d2sAdminExportApprovalRequest struct {
	Reason string `json:"reason"`
}

type d2sAdminExportApprovalDecision struct {
	Decision string `json:"decision"`
	Reason   string `json:"reason"`
}

func d2sExportApprovalSnapshot(c *gin.Context, exportType string) (map[string]any, error) {
	switch exportType {
	case model.D2SExportApprovalTypeUserActionLogs:
		filters, err := parseD2SActionLogFilters(c, false)
		if err != nil {
			return nil, err
		}
		return d2sFilterDetails(filters), nil
	case model.D2SExportApprovalTypeRegistrations:
		startTime, err := parseD2SUnixTime(c, "start_time")
		if err != nil {
			return nil, err
		}
		endTime, err := parseD2SUnixTime(c, "end_time")
		if err != nil || (startTime > 0 && endTime > 0 && startTime > endTime) {
			return nil, errors.New("invalid time range")
		}
		return map[string]any{"start_time": startTime, "end_time": endTime}, nil
	default:
		return nil, errors.New("unsupported export type")
	}
}

func d2sExportApprovalFingerprint(exportType string, snapshot map[string]any) (string, string, error) {
	encoded, err := common.Marshal(snapshot)
	if err != nil {
		return "", "", err
	}
	payload := strings.ToUpper(strings.TrimSpace(exportType)) + "\x00" + string(encoded)
	digest := sha256.Sum256([]byte(payload))
	return string(encoded), hex.EncodeToString(digest[:]), nil
}

func D2SAdminRequestExportApproval(c *gin.Context) {
	exportType := strings.ToUpper(strings.TrimSpace(c.Query("export_type")))
	snapshot, err := d2sExportApprovalSnapshot(c, exportType)
	if err != nil {
		respondD2SAdminAuditedFailure(c, "REQUEST_EXPORT_APPROVAL", exportType, "invalid_export_scope", http.StatusBadRequest, "INVALID_EXPORT_SCOPE")
		return
	}
	var request d2sAdminExportApprovalRequest
	if err := common.DecodeJson(c.Request.Body, &request); err != nil || strings.TrimSpace(request.Reason) == "" || len([]rune(strings.TrimSpace(request.Reason))) > 500 {
		respondD2SAdminAuditedFailure(c, "REQUEST_EXPORT_APPROVAL", exportType, "invalid_request_reason", http.StatusBadRequest, "INVALID_REQUEST_REASON")
		return
	}
	filterSnapshot, filterHash, err := d2sExportApprovalFingerprint(exportType, snapshot)
	if err != nil {
		respondD2SAdminAuditedFailure(c, "REQUEST_EXPORT_APPROVAL", exportType, "snapshot_failed", http.StatusInternalServerError, "EXPORT_SCOPE_FAILED")
		return
	}
	now := time.Now().UTC().Unix()
	approval := &model.D2SAdminExportApproval{
		ExportType: exportType, FilterHash: filterHash, FilterSnapshot: filterSnapshot,
		RequestReason: strings.TrimSpace(request.Reason), RequestedByID: c.GetInt("d2s_admin_id"),
		RequestedByEmail: c.GetString("d2s_admin_email"), RequestedAt: now, ExpiresAt: now + d2sAdminExportApprovalLifetime,
	}
	audit := &model.D2SAdminOperationAudit{
		OperatorID: c.GetInt("d2s_admin_id"), OperatorEmail: c.GetString("d2s_admin_email"),
		OperationType: "REQUEST_EXPORT_APPROVAL", OperationTime: now, TargetResource: exportType,
		SourceIP: c.ClientIP(), Result: "SUCCESS",
	}
	details, err := common.Marshal(map[string]any{
		"export_type": exportType, "filter_hash": filterHash, "filter_snapshot": snapshot,
		"reason": strings.TrimSpace(request.Reason),
	})
	if err != nil {
		respondD2SAdminAuditedFailure(c, "REQUEST_EXPORT_APPROVAL", exportType, "audit_serialize_failed", http.StatusInternalServerError, "AUDIT_SERIALIZE_FAILED")
		return
	}
	audit.DetailsJSON = string(details)
	if err := model.CreateD2SAdminExportApprovalWithAudit(approval, audit); err != nil {
		respondD2SAdminAuditedFailure(c, "REQUEST_EXPORT_APPROVAL", exportType, "approval_write_failed", http.StatusInternalServerError, "EXPORT_APPROVAL_WRITE_FAILED")
		return
	}
	c.JSON(http.StatusCreated, gin.H{"success": true, "data": approval})
}

func D2SAdminListExportApprovals(c *gin.Context) {
	page, pageSize := 1, 50
	var err error
	if raw := c.Query("page"); raw != "" {
		page, err = strconv.Atoi(raw)
		if err != nil || page < 1 {
			respondD2SAdminAuditedFailure(c, "VIEW_EXPORT_APPROVAL", "d2s_admin_export_approvals", "invalid_page", http.StatusBadRequest, "INVALID_PAGE")
			return
		}
	}
	if raw := c.Query("page_size"); raw != "" {
		pageSize, err = strconv.Atoi(raw)
		if err != nil || pageSize < 1 || pageSize > 100 {
			respondD2SAdminAuditedFailure(c, "VIEW_EXPORT_APPROVAL", "d2s_admin_export_approvals", "invalid_page_size", http.StatusBadRequest, "INVALID_PAGE_SIZE")
			return
		}
	}
	approvals, total, err := model.ListPendingD2SAdminExportApprovals(time.Now().UTC().Unix(), page, pageSize)
	if err != nil {
		respondD2SAdminAuditedFailure(c, "VIEW_EXPORT_APPROVAL", "d2s_admin_export_approvals", "query_failed", http.StatusInternalServerError, "EXPORT_APPROVAL_QUERY_FAILED")
		return
	}
	if approvals == nil {
		approvals = []model.D2SAdminExportApproval{}
	}
	if err := writeD2SAdminAudit(c, "VIEW_EXPORT_APPROVAL", "d2s_admin_export_approvals", "SUCCESS", map[string]any{"page": page, "page_size": pageSize, "total": total}); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "code": "AUDIT_WRITE_FAILED"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"items": approvals, "total": total, "page": page, "page_size": pageSize}})
}

func D2SAdminGetExportApproval(c *gin.Context) {
	approval, err := model.GetD2SAdminExportApproval(c.Param("id"))
	if err != nil {
		respondD2SAdminAuditedFailure(c, "VIEW_EXPORT_APPROVAL", c.Param("id"), "query_failed", http.StatusNotFound, "EXPORT_APPROVAL_NOT_FOUND")
		return
	}
	if approval.RequestedByID != c.GetInt("d2s_admin_id") && c.GetString("d2s_admin_role") != model.D2SAdminRoleSuperAdmin {
		respondD2SAdminAuditedFailure(c, "VIEW_EXPORT_APPROVAL", approval.ID, "not_owner", http.StatusForbidden, "EXPORT_APPROVAL_FORBIDDEN")
		return
	}
	if approval.Status == model.D2SExportApprovalPending && approval.ExpiresAt <= time.Now().UTC().Unix() {
		approval.Status = "EXPIRED"
	}
	if err := writeD2SAdminAudit(c, "VIEW_EXPORT_APPROVAL", approval.ID, "SUCCESS", map[string]any{"status": approval.Status, "export_type": approval.ExportType}); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "code": "AUDIT_WRITE_FAILED"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": approval})
}

func D2SAdminReviewExportApproval(c *gin.Context) {
	approval, err := model.GetD2SAdminExportApproval(c.Param("id"))
	if err != nil {
		respondD2SAdminAuditedFailure(c, "REVIEW_EXPORT_APPROVAL", c.Param("id"), "not_found", http.StatusNotFound, "EXPORT_APPROVAL_NOT_FOUND")
		return
	}
	var request d2sAdminExportApprovalDecision
	if err := common.DecodeJson(c.Request.Body, &request); err != nil {
		respondD2SAdminAuditedFailure(c, "REVIEW_EXPORT_APPROVAL", approval.ID, "invalid_request", http.StatusBadRequest, "INVALID_REQUEST")
		return
	}
	request.Decision = strings.ToUpper(strings.TrimSpace(request.Decision))
	request.Reason = strings.TrimSpace(request.Reason)
	if (request.Decision != model.D2SExportApprovalApproved && request.Decision != model.D2SExportApprovalRejected) || len([]rune(request.Reason)) == 0 || len([]rune(request.Reason)) > 500 {
		respondD2SAdminAuditedFailure(c, "REVIEW_EXPORT_APPROVAL", approval.ID, "invalid_decision", http.StatusBadRequest, "INVALID_DECISION")
		return
	}
	operation := "APPROVE_EXPORT"
	if request.Decision == model.D2SExportApprovalRejected {
		operation = "REJECT_EXPORT"
	}
	audit := &model.D2SAdminOperationAudit{
		OperatorID: c.GetInt("d2s_admin_id"), OperatorEmail: c.GetString("d2s_admin_email"),
		OperationType: operation, OperationTime: time.Now().UTC().Unix(), TargetResource: approval.ID,
		SourceIP: c.ClientIP(), Result: "SUCCESS",
	}
	auditDetails, err := common.Marshal(map[string]any{
		"requester_id": approval.RequestedByID, "export_type": approval.ExportType,
		"filter_hash": approval.FilterHash, "decision": request.Decision, "reason": request.Reason,
	})
	if err != nil {
		respondD2SAdminAuditedFailure(c, "REVIEW_EXPORT_APPROVAL", approval.ID, "audit_serialize_failed", http.StatusInternalServerError, "AUDIT_SERIALIZE_FAILED")
		return
	}
	audit.DetailsJSON = string(auditDetails)
	updated, err := model.DecideD2SAdminExportApprovalWithAudit(
		approval.ID, c.GetInt("d2s_admin_id"), c.GetString("d2s_admin_email"),
		request.Decision, request.Reason, time.Now().UTC().Unix(), audit,
	)
	if err != nil {
		status, code, reason := http.StatusConflict, "EXPORT_APPROVAL_CONFLICT", "decision_conflict"
		switch {
		case errors.Is(err, model.ErrD2SExportApprovalSelfApproval):
			status, code, reason = http.StatusForbidden, "EXPORT_SELF_APPROVAL_FORBIDDEN", "self_approval"
		case errors.Is(err, model.ErrD2SExportApprovalExpired):
			status, code, reason = http.StatusGone, "EXPORT_APPROVAL_EXPIRED", "expired"
		case errors.Is(err, model.ErrD2SExportApprovalNotFound):
			status, code, reason = http.StatusNotFound, "EXPORT_APPROVAL_NOT_FOUND", "not_found"
		}
		respondD2SAdminAuditedFailure(c, "REVIEW_EXPORT_APPROVAL", approval.ID, reason, status, code)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": updated})
}

func consumeD2SExportApproval(c *gin.Context, approvalID, exportType string, snapshot map[string]any) error {
	if strings.TrimSpace(approvalID) == "" {
		return model.ErrD2SExportApprovalNotApproved
	}
	_, filterHash, err := d2sExportApprovalFingerprint(exportType, snapshot)
	if err != nil {
		return err
	}
	details, err := common.Marshal(map[string]any{"export_type": exportType, "filter_hash": filterHash})
	if err != nil {
		return err
	}
	return model.ConsumeD2SAdminExportApprovalWithAudit(approvalID, exportType, filterHash,
		c.GetInt("d2s_admin_id"), time.Now().UTC().Unix(), &model.D2SAdminOperationAudit{
			OperatorID: c.GetInt("d2s_admin_id"), OperatorEmail: c.GetString("d2s_admin_email"),
			OperationType: "USE_EXPORT_APPROVAL", TargetResource: approvalID,
			SourceIP: c.ClientIP(), Result: "SUCCESS", DetailsJSON: string(details),
		})
}
