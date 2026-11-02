package middleware

import (
	"crypto/subtle"
	"encoding/hex"
	"net/http"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

func authenticateD2SAdminSession(c *gin.Context) string {
	rawToken, err := c.Cookie(service.D2SAdminSessionCookieName)
	if err != nil {
		return "ADMIN_AUTH_REQUIRED"
	}
	session, account, err := service.ValidateD2SAdminSession(rawToken, 0)
	if err != nil {
		return "ADMIN_AUTH_INVALID"
	}
	c.Set("d2s_admin_id", account.ID)
	c.Set("d2s_admin_email", account.Email)
	c.Set("d2s_admin_role", account.Role)
	c.Set("d2s_admin_session_hash", session.TokenHash)
	return ""
}

func D2SAdminAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		if code := authenticateD2SAdminSession(c); code != "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"success": false, "code": code})
			return
		}
		c.Next()
	}
}

func D2SAdminReconciliationAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		if code := authenticateD2SAdminSession(c); code == "" {
			c.Next()
			return
		}

		expectedToken, err := hex.DecodeString(os.Getenv("D2S_RECONCILIATION_TOKEN"))
		if err != nil || len(expectedToken) != 32 {
			c.AbortWithStatusJSON(http.StatusServiceUnavailable, gin.H{"success": false, "code": "RECONCILIATION_AUTH_UNAVAILABLE"})
			return
		}
		scheme, provided, ok := strings.Cut(strings.TrimSpace(c.GetHeader("Authorization")), " ")
		if !ok || !strings.EqualFold(scheme, "Bearer") {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"success": false, "code": "ADMIN_AUTH_REQUIRED"})
			return
		}
		providedToken, err := hex.DecodeString(strings.TrimSpace(provided))
		if err != nil || len(providedToken) != 32 || subtle.ConstantTimeCompare(expectedToken, providedToken) != 1 {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"success": false, "code": "ADMIN_AUTH_INVALID"})
			return
		}

		c.Set("d2s_admin_id", 0)
		c.Set("d2s_admin_email", "system:payment-reconciliation")
		c.Set("d2s_admin_role", model.D2SAdminRoleSuperAdmin)
		c.Next()
	}
}

func D2SAdminRequireRole(roles ...string) gin.HandlerFunc {
	return func(c *gin.Context) {
		role := c.GetString("d2s_admin_role")
		if role == model.D2SAdminRoleSuperAdmin || slices.Contains(roles, role) {
			c.Next()
			return
		}

		target := c.FullPath()
		if target == "" {
			target = c.Request.URL.Path
		}
		details, err := common.Marshal(map[string]any{
			"role": role, "required_roles": roles, "method": c.Request.Method,
		})
		if err != nil {
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"success": false, "code": "ADMIN_AUDIT_UNAVAILABLE"})
			return
		}
		operatorEmail := c.GetString("d2s_admin_email")
		if strings.TrimSpace(operatorEmail) == "" {
			operatorEmail = "unknown"
		}
		auditErr := model.RecordD2SAdminOperationAudit(&model.D2SAdminOperationAudit{
			OperatorID: c.GetInt("d2s_admin_id"), OperatorEmail: operatorEmail,
			OperationType: "ADMIN_PERMISSION_DENIED", OperationTime: time.Now().UTC().Unix(),
			TargetResource: target, SourceIP: c.ClientIP(), Result: "FAILURE",
			DetailsJSON: string(details),
		})
		if auditErr != nil {
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"success": false, "code": "ADMIN_AUDIT_UNAVAILABLE"})
			return
		}
		c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"success": false, "code": "ADMIN_PERMISSION_DENIED"})
	}
}
