package controller

import (
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

func d2SUserActionLogInput(c *gin.Context, userID *int, email, actionType, result string) model.D2SActionLogInput {
	sourcePort := common.ClientSourcePort(c.Request.RemoteAddr, c.RemoteIP(), c.ClientIP())
	var destinationIP *string
	var destinationPort *int
	if localAddr, ok := c.Request.Context().Value(http.LocalAddrContextKey).(net.Addr); ok && localAddr != nil {
		if host, portText, err := net.SplitHostPort(localAddr.String()); err == nil {
			destinationIP = &host
			if port, parseErr := strconv.Atoi(portText); parseErr == nil && port > 0 && port <= 65535 {
				destinationPort = &port
			}
		}
	}
	requestPath := c.FullPath()
	if requestPath == "" {
		requestPath = c.Request.URL.Path
	}
	return model.D2SActionLogInput{
		UserID: userID, Email: email, ActionType: actionType, EventTime: time.Now().UTC().Unix(),
		SourceIP: c.ClientIP(), SourcePort: sourcePort, DestinationIP: destinationIP, DestinationPort: destinationPort,
		UserAgent: c.Request.UserAgent(), DeviceFingerprint: nil, RequestPath: requestPath, ResultStatus: result,
	}
}

func recordD2SUserAction(c *gin.Context, userID *int, email, actionType, result string) error {
	return model.RecordD2SUserAction(d2SUserActionLogInput(c, userID, email, actionType, result))
}

type d2SEmergencyControlRequest struct {
	LoginBlocked    *bool  `json:"login_blocked"`
	PurchaseBlocked *bool  `json:"purchase_blocked"`
	ForceLogout     bool   `json:"force_logout"`
	Reason          string `json:"reason"`
}

func D2SAdminSetEmergencyControl(c *gin.Context) {
	operationType := "EMERGENCY_LOCK"
	userID, err := strconv.Atoi(c.Param("id"))
	if err != nil || userID <= 0 {
		respondD2SAdminAuditedFailure(c, operationType, c.Param("id"), "invalid_user_id", http.StatusBadRequest, "INVALID_USER_ID")
		return
	}
	var request d2SEmergencyControlRequest
	if err := common.DecodeJson(c.Request.Body, &request); err != nil || (request.LoginBlocked == nil && request.PurchaseBlocked == nil && !request.ForceLogout) {
		respondD2SAdminAuditedFailure(c, operationType, strconv.Itoa(userID), "invalid_request", http.StatusBadRequest, "INVALID_REQUEST")
		return
	}
	if !request.ForceLogout && (request.LoginBlocked == nil || !*request.LoginBlocked) && (request.PurchaseBlocked == nil || !*request.PurchaseBlocked) {
		operationType = "EMERGENCY_UNLOCK"
	}
	user, err := model.GetUserById(userID, false)
	if err != nil || user == nil {
		respondD2SAdminAuditedFailure(c, operationType, strconv.Itoa(userID), "user_not_found", http.StatusNotFound, "USER_NOT_FOUND")
		return
	}
	control, err := model.GetD2SUserSecurityControl(userID)
	if err != nil {
		respondD2SAdminAuditedFailure(c, operationType, strconv.Itoa(userID), "control_read_failed", http.StatusInternalServerError, "CONTROL_READ_FAILED")
		return
	}
	if request.LoginBlocked != nil {
		control.LoginBlocked = *request.LoginBlocked
	}
	if request.PurchaseBlocked != nil {
		control.PurchaseBlocked = *request.PurchaseBlocked
	}
	control.Reason = strings.TrimSpace(request.Reason)
	control.UpdatedBy = c.GetInt("d2s_admin_id")
	control.UpdatedAt = time.Now().UTC().Unix()
	if control.LoginBlocked || control.PurchaseBlocked || request.ForceLogout {
		operationType = "EMERGENCY_LOCK"
	}
	details := map[string]any{
		"login_blocked": control.LoginBlocked, "purchase_blocked": control.PurchaseBlocked,
		"force_logout_requested": request.ForceLogout, "reason": control.Reason,
	}
	detailsJSON, err := common.Marshal(details)
	if err != nil {
		respondD2SAdminAuditedFailure(c, operationType, strconv.Itoa(userID), "audit_serialize_failed", http.StatusInternalServerError, "AUDIT_SERIALIZE_FAILED")
		return
	}
	auditResult := "SUCCESS"
	if request.ForceLogout {
		auditResult = "STARTED"
	}
	audit := &model.D2SAdminOperationAudit{
		OperatorID: c.GetInt("d2s_admin_id"), OperatorEmail: c.GetString("d2s_admin_email"),
		OperationType: operationType, OperationTime: control.UpdatedAt,
		TargetResource: strconv.Itoa(userID), SourceIP: c.ClientIP(), Result: auditResult, DetailsJSON: string(detailsJSON),
	}
	if err := model.SetD2SUserSecurityControlWithAudit(control, audit); err != nil {
		respondD2SAdminAuditedFailure(c, operationType, strconv.Itoa(userID), "control_write_failed", http.StatusInternalServerError, "CONTROL_WRITE_FAILED")
		return
	}
	revokedSessions := int64(0)
	if request.ForceLogout {
		revokedSessions, err = model.RevokeAllUserSessions(userID, "d2s_emergency_force_logout")
		if err == nil {
			_, err = model.BumpUserAuthVersion(userID)
		}
		if err == nil {
			err = model.ReleaseAllD2SOnlineLeases(userID)
		}
		if err != nil {
			auditErr := writeD2SAdminAudit(c, operationType, strconv.Itoa(userID), "FAILURE", map[string]any{
				"phase": "force_logout", "force_logout": true, "revoked_sessions": revokedSessions,
			})
			if auditErr != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"success": false, "code": "ADMIN_AUDIT_WRITE_FAILED"})
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{"success": false, "code": "FORCE_LOGOUT_FAILED"})
			return
		}
		if err := writeD2SAdminAudit(c, operationType, strconv.Itoa(userID), "SUCCESS", map[string]any{
			"phase": "force_logout", "force_logout": true, "revoked_sessions": revokedSessions,
		}); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"success": false, "code": "ADMIN_AUDIT_WRITE_FAILED"})
			return
		}
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{
		"user_id": userID, "login_blocked": control.LoginBlocked, "purchase_blocked": control.PurchaseBlocked,
		"revoked_sessions": revokedSessions,
	}})
}

func d2sRejectPurchasesDisabled(c *gin.Context) bool {
	if common.GetEnvOrDefaultBool("D2S_PURCHASES_ENABLED", false) {
		return false
	}
	d2sError(c, model.ErrD2SPurchasesDisabled)
	return true
}

func d2sRejectBlockedPurchase(c *gin.Context, action string) bool {
	userID := c.GetInt("id")
	blocked, err := model.IsD2SUserPurchaseBlocked(userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "code": "ACCOUNT_CONTROL_CHECK_FAILED"})
		return true
	}
	if !blocked {
		return false
	}
	c.JSON(http.StatusLocked, gin.H{"success": false, "code": "ACCOUNT_PURCHASE_BLOCKED", "action": action})
	return true
}
