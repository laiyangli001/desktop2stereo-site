package middleware

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

// D2SSystemPaymentIdentity adapts an authenticated system root to payment audits.
// RootAuth must run first; independent security-admin cookies are never accepted.
func D2SSystemPaymentIdentity() gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.GetInt("id") <= 0 || c.GetInt("role") < common.RoleRootUser {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"success": false, "code": "AUTH_INSUFFICIENT_PRIVILEGE"})
			return
		}
		user, err := model.GetUserById(c.GetInt("id"), false)
		if err != nil || user.Role < common.RoleRootUser || user.Status != common.UserStatusEnabled {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"success": false, "code": "AUTH_INSUFFICIENT_PRIVILEGE"})
			return
		}
		email := strings.TrimSpace(user.Email)
		if email == "" {
			email = "system-admin:" + strconv.Itoa(user.Id)
		}
		c.Set("d2s_admin_id", user.Id)
		c.Set("d2s_admin_email", email)
		c.Set("d2s_operator_realm", "system_admin")
		c.Next()
	}
}
