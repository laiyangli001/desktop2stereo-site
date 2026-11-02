package controller

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type d2SBatchSelector struct {
	IPCIDR        string `json:"ip_cidr"`
	EmailDomain   string `json:"email_domain"`
	RegisterFrom  int64  `json:"register_from"`
	RegisterUntil int64  `json:"register_until"`
}

type d2SBatchActions struct {
	LoginBlocked    *bool  `json:"login_blocked"`
	PurchaseBlocked *bool  `json:"purchase_blocked"`
	Reason          string `json:"reason"`
}

type d2SBatchPreviewRequest struct {
	Selector d2SBatchSelector `json:"selector"`
	Actions  d2SBatchActions  `json:"actions"`
}

type d2SBatchApplyRequest struct {
	PreviewID         string `json:"preview_id"`
	Confirmed         bool   `json:"confirmed"`
	ConfirmationCount int64  `json:"confirmation_count"`
}

var errD2SBatchSelectionChanged = errors.New("batch selection changed since preview")

func validateD2SBatchRequest(request d2SBatchPreviewRequest) (*net.IPNet, error) {
	selector := request.Selector
	if selector.IPCIDR == "" && selector.EmailDomain == "" && selector.RegisterFrom <= 0 && selector.RegisterUntil <= 0 {
		return nil, fmt.Errorf("at least one filter is required")
	}
	if selector.RegisterFrom > 0 && selector.RegisterUntil > 0 && selector.RegisterFrom > selector.RegisterUntil {
		return nil, fmt.Errorf("invalid registration time window")
	}
	var ipNet *net.IPNet
	if selector.IPCIDR != "" {
		_, parsed, err := net.ParseCIDR(strings.TrimSpace(selector.IPCIDR))
		if err != nil {
			return nil, fmt.Errorf("invalid IP range")
		}
		ipNet = parsed
	}
	if selector.EmailDomain != "" {
		domain := strings.ToLower(strings.TrimSpace(strings.TrimPrefix(selector.EmailDomain, "@")))
		if len(domain) > 253 || strings.ContainsAny(domain, "%_/@ ") || !strings.Contains(domain, ".") {
			return nil, fmt.Errorf("invalid email domain")
		}
		request.Selector.EmailDomain = domain
	}
	if request.Actions.LoginBlocked == nil && request.Actions.PurchaseBlocked == nil {
		return nil, fmt.Errorf("at least one action is required")
	}
	return ipNet, nil
}

func collectD2SBatchUserIDs(selector d2SBatchSelector, ipNet *net.IPNet) ([]int, error) {
	return collectD2SBatchUserIDsFromQuery(model.DB, selector, ipNet)
}

func collectD2SBatchUserIDsFromQuery(query *gorm.DB, selector d2SBatchSelector, ipNet *net.IPNet) ([]int, error) {
	query = query.Model(&model.D2SRegistrationRecord{})
	if selector.RegisterFrom > 0 {
		query = query.Where("register_time >= ?", selector.RegisterFrom)
	}
	if selector.RegisterUntil > 0 {
		query = query.Where("register_time <= ?", selector.RegisterUntil)
	}
	if selector.EmailDomain != "" {
		query = query.Where("email LIKE ?", "%@"+strings.ToLower(selector.EmailDomain))
	}
	var records []model.D2SRegistrationRecord
	if err := query.Order("register_time DESC").Limit(50001).Find(&records).Error; err != nil {
		return nil, err
	}
	if len(records) > 50000 {
		return nil, fmt.Errorf("selection exceeds the 50000 record safety bound")
	}
	seen := make(map[int]struct{})
	userIDs := make([]int, 0, len(records))
	for _, record := range records {
		if ipNet != nil && !ipNet.Contains(net.ParseIP(record.RegisterIP)) {
			continue
		}
		if _, exists := seen[record.UserID]; exists {
			continue
		}
		seen[record.UserID] = struct{}{}
		userIDs = append(userIDs, record.UserID)
	}
	return userIDs, nil
}

func sameD2SBatchUserIDs(left, right []int) bool {
	if len(left) != len(right) {
		return false
	}
	members := make(map[int]struct{}, len(left))
	for _, userID := range left {
		members[userID] = struct{}{}
	}
	for _, userID := range right {
		if _, exists := members[userID]; !exists {
			return false
		}
	}
	return true
}

func D2SAdminBatchEmergencyPreview(c *gin.Context) {
	var request d2SBatchPreviewRequest
	if err := common.DecodeJson(c.Request.Body, &request); err != nil {
		respondD2SAdminAuditedFailure(c, "BATCH_EMERGENCY_PREVIEW", "user_accounts:batch", "invalid_request", http.StatusBadRequest, "INVALID_REQUEST")
		return
	}
	ipNet, err := validateD2SBatchRequest(request)
	if err != nil {
		respondD2SAdminAuditedFailure(c, "BATCH_EMERGENCY_PREVIEW", "user_accounts:batch", "invalid_selector_or_action", http.StatusBadRequest, "INVALID_SELECTOR_OR_ACTION")
		return
	}
	request.Selector.EmailDomain = strings.ToLower(strings.TrimSpace(strings.TrimPrefix(request.Selector.EmailDomain, "@")))
	userIDs, err := collectD2SBatchUserIDs(request.Selector, ipNet)
	if err != nil {
		respondD2SAdminAuditedFailure(c, "BATCH_EMERGENCY_PREVIEW", "user_accounts:batch", "selection_failed", http.StatusUnprocessableEntity, "BATCH_SELECTION_TOO_BROAD")
		return
	}
	if len(userIDs) > 10000 {
		respondD2SAdminAuditedFailureDetails(c, "BATCH_EMERGENCY_PREVIEW", "user_accounts:batch", map[string]any{"reason": "match_count_limit", "match_count": len(userIDs)}, http.StatusUnprocessableEntity, "BATCH_MATCH_LIMIT_EXCEEDED")
		return
	}
	selectorJSON, err := common.Marshal(request.Selector)
	if err != nil {
		respondD2SAdminAuditedFailure(c, "BATCH_EMERGENCY_PREVIEW", "user_accounts:batch", "preview_serialize_failed", http.StatusInternalServerError, "PREVIEW_CREATE_FAILED")
		return
	}
	actionJSON, err := common.Marshal(request.Actions)
	if err != nil {
		respondD2SAdminAuditedFailure(c, "BATCH_EMERGENCY_PREVIEW", "user_accounts:batch", "preview_serialize_failed", http.StatusInternalServerError, "PREVIEW_CREATE_FAILED")
		return
	}
	userIDsJSON, err := common.Marshal(userIDs)
	if err != nil {
		respondD2SAdminAuditedFailure(c, "BATCH_EMERGENCY_PREVIEW", "user_accounts:batch", "preview_serialize_failed", http.StatusInternalServerError, "PREVIEW_CREATE_FAILED")
		return
	}
	now := time.Now().UTC().Unix()
	preview := model.D2SAdminBatchPreview{
		ID: uuid.NewString(), AdminID: c.GetInt("d2s_admin_id"), SelectorJSON: string(selectorJSON),
		ActionJSON: string(actionJSON), UserIDsJSON: string(userIDsJSON), MatchCount: int64(len(userIDs)),
		CreatedAt: now, ExpiresAt: now + 300,
	}
	details, err := common.Marshal(map[string]any{
		"selector": request.Selector, "match_count": preview.MatchCount, "expires_at": preview.ExpiresAt,
	})
	if err != nil {
		respondD2SAdminAuditedFailure(c, "BATCH_EMERGENCY_PREVIEW", "user_accounts:batch", "audit_serialize_failed", http.StatusInternalServerError, "PREVIEW_CREATE_FAILED")
		return
	}
	audit := model.D2SAdminOperationAudit{
		ID: uuid.NewString(), OperatorID: c.GetInt("d2s_admin_id"), OperatorEmail: c.GetString("d2s_admin_email"),
		OperationType: "BATCH_EMERGENCY_PREVIEW", OperationTime: now, TargetResource: "user_accounts:batch",
		SourceIP: c.ClientIP(), Result: "SUCCESS", DetailsJSON: string(details),
	}
	if err := model.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&preview).Error; err != nil {
			return err
		}
		return tx.Create(&audit).Error
	}); err != nil {
		respondD2SAdminAuditedFailure(c, "BATCH_EMERGENCY_PREVIEW", "user_accounts:batch", "preview_store_or_audit_failed", http.StatusInternalServerError, "PREVIEW_CREATE_FAILED")
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{
		"preview_id": preview.ID, "match_count": preview.MatchCount, "expires_at": preview.ExpiresAt,
	}})
}

func D2SAdminBatchEmergencyApply(c *gin.Context) {
	var request d2SBatchApplyRequest
	if err := common.DecodeJson(c.Request.Body, &request); err != nil || !request.Confirmed || request.ConfirmationCount < 0 {
		respondD2SAdminAuditedFailure(c, "EMERGENCY_LOCK", "user_accounts:batch", "explicit_confirmation_required", http.StatusBadRequest, "EXPLICIT_CONFIRMATION_REQUIRED")
		return
	}
	now := time.Now().UTC().Unix()
	var count int64
	var previewCount, currentCount int64
	var operationType string
	err := model.DB.Transaction(func(tx *gorm.DB) error {
		var preview model.D2SAdminBatchPreview
		if err := tx.Where("id = ? AND admin_id = ? AND consumed_at = 0 AND expires_at > ?", strings.TrimSpace(request.PreviewID), c.GetInt("d2s_admin_id"), now).First(&preview).Error; err != nil {
			return err
		}
		previewCount = preview.MatchCount
		if preview.MatchCount != request.ConfirmationCount {
			return fmt.Errorf("confirmation count mismatch")
		}
		var selector d2SBatchSelector
		var actions d2SBatchActions
		var userIDs []int
		if err := common.Unmarshal([]byte(preview.SelectorJSON), &selector); err != nil {
			return err
		}
		if err := common.Unmarshal([]byte(preview.ActionJSON), &actions); err != nil {
			return err
		}
		if err := common.Unmarshal([]byte(preview.UserIDsJSON), &userIDs); err != nil {
			return err
		}
		selectionRequest := d2SBatchPreviewRequest{Selector: selector, Actions: actions}
		ipNet, err := validateD2SBatchRequest(selectionRequest)
		if err != nil {
			return err
		}
		selector = selectionRequest.Selector
		selector.EmailDomain = strings.ToLower(strings.TrimSpace(strings.TrimPrefix(selector.EmailDomain, "@")))
		currentUserIDs, err := collectD2SBatchUserIDsFromQuery(tx, selector, ipNet)
		if err != nil {
			return err
		}
		currentCount = int64(len(currentUserIDs))
		if currentCount != preview.MatchCount || !sameD2SBatchUserIDs(currentUserIDs, userIDs) {
			return errD2SBatchSelectionChanged
		}
		for _, userID := range currentUserIDs {
			control := model.D2SUserSecurityControl{UserID: userID}
			if err := tx.Where("user_id = ?", userID).First(&control).Error; err != nil && err != gorm.ErrRecordNotFound {
				return err
			}
			if actions.LoginBlocked != nil {
				control.LoginBlocked = *actions.LoginBlocked
			}
			if actions.PurchaseBlocked != nil {
				control.PurchaseBlocked = *actions.PurchaseBlocked
			}
			control.Reason = strings.TrimSpace(actions.Reason)
			control.UpdatedAt = now
			control.UpdatedBy = c.GetInt("d2s_admin_id")
			if err := tx.Clauses(clause.OnConflict{
				Columns:   []clause.Column{{Name: "user_id"}},
				DoUpdates: clause.AssignmentColumns([]string{"login_blocked", "purchase_blocked", "reason", "updated_at", "updated_by"}),
			}).Create(&control).Error; err != nil {
				return err
			}
		}
		consumed := tx.Model(&model.D2SAdminBatchPreview{}).Where("id = ? AND consumed_at = 0", preview.ID).Update("consumed_at", now)
		if consumed.Error != nil {
			return consumed.Error
		}
		if consumed.RowsAffected != 1 {
			return fmt.Errorf("batch preview already consumed")
		}
		count = currentCount
		operationType = "EMERGENCY_UNLOCK"
		if (actions.LoginBlocked != nil && *actions.LoginBlocked) || (actions.PurchaseBlocked != nil && *actions.PurchaseBlocked) {
			operationType = "EMERGENCY_LOCK"
		}
		details, err := common.Marshal(map[string]any{
			"selector": selector, "login_blocked": actions.LoginBlocked,
			"purchase_blocked": actions.PurchaseBlocked, "affected_accounts": count,
		})
		if err != nil {
			return err
		}
		return tx.Create(&model.D2SAdminOperationAudit{
			ID: uuid.NewString(), OperatorID: c.GetInt("d2s_admin_id"),
			OperatorEmail: c.GetString("d2s_admin_email"), OperationType: operationType,
			OperationTime: now, TargetResource: "user_accounts:batch", SourceIP: c.ClientIP(),
			Result: "SUCCESS", DetailsJSON: string(details),
		}).Error
	})
	if err != nil {
		reason := "batch_apply_failed"
		if errors.Is(err, errD2SBatchSelectionChanged) {
			reason = "selection_changed_since_preview"
		}
		respondD2SAdminAuditedFailureDetails(c, "EMERGENCY_LOCK", "user_accounts:batch", map[string]any{
			"reason": reason, "preview_id": strings.TrimSpace(request.PreviewID),
			"confirmation_count":  request.ConfirmationCount,
			"preview_match_count": previewCount, "current_match_count": currentCount,
		}, http.StatusConflict, "BATCH_PREVIEW_INVALID_OR_EXPIRED")
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"affected_accounts": count, "operation_type": operationType}})
}
