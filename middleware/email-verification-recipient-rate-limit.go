package middleware

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

const emailVerificationRecipientLimit = 2
const emailVerificationRecipientWindow int64 = 30

func EmailVerificationRecipientRateLimit() gin.HandlerFunc {
	inMemoryRateLimiter.Init(common.RateLimitKeyExpirationDuration)
	return func(c *gin.Context) {
		email := model.NormalizeEmail(strings.TrimSpace(c.Query("email")))
		if email == "" {
			c.Next()
			return
		}
		digest := sha256.Sum256([]byte(email))
		key := "email-verification-recipient:" + hex.EncodeToString(digest[:])
		allowed := false
		var ttl int64
		if common.RedisEnabled {
			var err error
			allowed, _, ttl, err = redisFixedWindowTake(c.Request.Context(), key, emailVerificationRecipientLimit, emailVerificationRecipientWindow)
			if err != nil {
				allowed = inMemoryRateLimiter.Request(key, emailVerificationRecipientLimit, emailVerificationRecipientWindow)
			}
		} else {
			allowed = inMemoryRateLimiter.Request(key, emailVerificationRecipientLimit, emailVerificationRecipientWindow)
		}
		if allowed {
			c.Next()
			return
		}
		if ttl <= 0 {
			ttl = emailVerificationRecipientWindow
		}
		c.Header("Retry-After", strconv.FormatInt(ttl, 10))
		c.JSON(http.StatusTooManyRequests, gin.H{"success": false, "code": "EMAIL_VERIFICATION_RATE_LIMITED"})
		c.Abort()
	}
}
