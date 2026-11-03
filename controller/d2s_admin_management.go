package controller

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

type d2sAdminLoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

func writeD2SAdminAudit(c *gin.Context, operationType, target, result string, details map[string]any) error {
	audit, err := d2SAdminAuditEntry(c, operationType, target, result, details)
	if err != nil {
		return err
	}
	return model.RecordD2SAdminOperationAudit(audit)
}

func d2SAdminAuditEntry(c *gin.Context, operationType, target, result string, details map[string]any) (*model.D2SAdminOperationAudit, error) {
	detailsJSON, err := common.Marshal(details)
	if err != nil {
		return nil, err
	}
	return &model.D2SAdminOperationAudit{
		OperatorID: c.GetInt("d2s_admin_id"), OperatorEmail: c.GetString("d2s_admin_email"),
		OperationType: operationType, OperationTime: time.Now().UTC().Unix(),
		TargetResource: target, SourceIP: c.ClientIP(), Result: result, DetailsJSON: string(detailsJSON),
	}, nil
}

func respondD2SAdminAuditedFailure(c *gin.Context, operationType, target, reason string, status int, code string) {
	respondD2SAdminAuditedFailureDetails(c, operationType, target, map[string]any{"reason": reason}, status, code)
}

func respondD2SAdminAuditedFailureDetails(c *gin.Context, operationType, target string, details map[string]any, status int, code string) {
	if err := writeD2SAdminAudit(c, operationType, target, "FAILURE", details); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "code": "AUDIT_WRITE_FAILED"})
		return
	}
	c.JSON(status, gin.H{"success": false, "code": code})
}

func setD2SAdminSessionCookie(c *gin.Context, token string, maxAge int) {
	c.SetSameSite(http.SameSiteStrictMode)
	c.SetCookie(service.D2SAdminSessionCookieName, token, maxAge, "/api/v1/admin", "", common.SessionCookieSecure, true)
}

func clearD2SAdminSessionCookie(c *gin.Context) {
	setD2SAdminSessionCookie(c, "", -1)
}

func recordD2SAdminLoginInputFailure(c *gin.Context, email, reason string, now int64) error {
	operatorEmail := model.NormalizeEmail(email)
	if len(operatorEmail) > 254 || common.Validate.Var(operatorEmail, "required,email") != nil {
		operatorEmail = "unknown"
	}
	detailsJSON, err := common.Marshal(map[string]string{"reason": reason})
	if err != nil {
		return err
	}
	return model.RecordD2SAdminOperationAudit(&model.D2SAdminOperationAudit{
		OperatorEmail: operatorEmail, OperationType: "LOGIN_ADMIN", OperationTime: now,
		TargetResource: "admin_login", SourceIP: c.ClientIP(), Result: "FAILURE", DetailsJSON: string(detailsJSON),
	})
}

func D2SAdminLogin(c *gin.Context) {
	now := time.Now().UTC().Unix()
	var request d2sAdminLoginRequest
	if err := common.DecodeJson(c.Request.Body, &request); err != nil {
		if auditErr := recordD2SAdminLoginInputFailure(c, "", "invalid_request", now); auditErr != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"success": false, "code": "AUDIT_WRITE_FAILED"})
			return
		}
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "code": "INVALID_REQUEST"})
		return
	}
	request.Email = model.NormalizeEmail(request.Email)
	emailValid := len(request.Email) <= 254 && common.Validate.Var(request.Email, "required,email") == nil
	if !emailValid || len(request.Password) == 0 || len(request.Password) > 72 {
		auditEmail := ""
		if emailValid {
			auditEmail = request.Email
		}
		if auditErr := recordD2SAdminLoginInputFailure(c, auditEmail, "invalid_request", now); auditErr != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"success": false, "code": "AUDIT_WRITE_FAILED"})
			return
		}
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "code": "INVALID_REQUEST"})
		return
	}
	account, rawToken, err := service.AuthenticateD2SAdmin(request.Email, request.Password, c.ClientIP(), c.Request.UserAgent(), now)
	if err != nil {
		result := "FAILURE"
		status := http.StatusUnauthorized
		if errors.Is(err, service.ErrD2SAdminLoginLocked) {
			result = "BLOCKED"
			status = http.StatusLocked
		}
		auditErr := model.RecordD2SAdminOperationAudit(&model.D2SAdminOperationAudit{
			OperatorEmail: request.Email, OperationType: "LOGIN_ADMIN", OperationTime: now,
			SourceIP: c.ClientIP(), Result: result, DetailsJSON: "{\"reason\":\"authentication_failed\"}",
		})
		if auditErr != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"success": false, "code": "AUDIT_WRITE_FAILED"})
			return
		}
		messageCode := "ADMIN_AUTH_INVALID"
		if status == http.StatusLocked {
			messageCode = "ADMIN_AUTH_LOCKED"
		}
		c.JSON(status, gin.H{"success": false, "code": messageCode})
		return
	}
	if err := model.RecordD2SAdminOperationAudit(&model.D2SAdminOperationAudit{
		OperatorID: account.ID, OperatorEmail: account.Email,
		OperationType: "LOGIN_ADMIN", OperationTime: now, SourceIP: c.ClientIP(),
		Result: "SUCCESS", DetailsJSON: "{\"method\":\"password\"}",
	}); err != nil {
		_ = service.RevokeD2SAdminSession(rawToken, now)
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "code": "AUDIT_WRITE_FAILED"})
		return
	}
	setD2SAdminSessionCookie(c, rawToken, int(service.D2SAdminSessionLifetime.Seconds()))
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    gin.H{"admin": gin.H{"id": account.ID, "email": account.Email, "role": account.Role}},
	})
}

func D2SAdminSession(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"admin": gin.H{
				"id": c.GetInt("d2s_admin_id"), "email": c.GetString("d2s_admin_email"),
				"role": c.GetString("d2s_admin_role"),
			},
		},
	})
}

func D2SAdminLogout(c *gin.Context) {
	rawToken, _ := c.Cookie(service.D2SAdminSessionCookieName)
	err := service.RevokeD2SAdminSession(rawToken, time.Now().UTC().Unix())
	result := "SUCCESS"
	if err != nil {
		result = "FAILURE"
	}
	auditErr := writeD2SAdminAudit(c, "LOGOUT_ADMIN", "d2s_admin_sessions", result, map[string]any{"session": "current"})
	clearD2SAdminSessionCookie(c)
	if auditErr != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "code": "AUDIT_WRITE_FAILED"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "code": "ADMIN_LOGOUT_FAILED"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}

func d2sAdminRequestEmail(c *gin.Context) string {
	return model.NormalizeEmail(strings.TrimSpace(c.GetString("d2s_admin_email")))
}
