package middleware

import (
	"bytes"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

type d2sActionCaptureWriter struct {
	gin.ResponseWriter
	body bytes.Buffer
}

func (w *d2sActionCaptureWriter) Write(data []byte) (int, error) {
	return w.body.Write(data)
}

func (w *d2sActionCaptureWriter) WriteString(value string) (int, error) {
	return w.body.WriteString(value)
}

func (w *d2sActionCaptureWriter) WriteHeaderNow() {}

func (w *d2sActionCaptureWriter) Flush() {}

func (w *d2sActionCaptureWriter) writeResponse() error {
	w.ResponseWriter.WriteHeaderNow()
	if w.body.Len() == 0 {
		return nil
	}
	_, err := w.ResponseWriter.Write(w.body.Bytes())
	return err
}

func D2SUserActionLog(actionType string) gin.HandlerFunc {
	return func(c *gin.Context) {
		originalBody := c.Request.Body
		requestBody, _ := io.ReadAll(io.LimitReader(originalBody, 64*1024))
		c.Request.Body = io.NopCloser(io.MultiReader(bytes.NewReader(requestBody), originalBody))
		requestIdentity := struct {
			Email string `json:"email"`
		}{}
		_ = json.Unmarshal(requestBody, &requestIdentity)
		email := strings.TrimSpace(requestIdentity.Email)
		if email == "" {
			email = strings.TrimSpace(c.Query("email"))
		}
		capture := &d2sActionCaptureWriter{ResponseWriter: c.Writer}
		c.Writer = capture
		c.Next()
		c.Writer = capture.ResponseWriter
		if c.GetBool("d2s_user_action_recorded") {
			if err := capture.writeResponse(); err != nil {
				common.SysLog("failed to flush D2S user action response: " + err.Error())
			}
			return
		}

		var response struct {
			Success *bool  `json:"success"`
			Code    string `json:"code"`
		}
		_ = json.Unmarshal(capture.body.Bytes(), &response)
		result := model.D2SActionResultSuccess
		switch {
		case c.Writer.Status() == http.StatusForbidden || c.Writer.Status() == http.StatusLocked || c.Writer.Status() == http.StatusTooManyRequests:
			result = model.D2SActionResultBlocked
		case c.Writer.Status() >= http.StatusBadRequest:
			result = model.D2SActionResultFailure
		case response.Success != nil && !*response.Success:
			result = model.D2SActionResultFailure
		}
		if strings.Contains(response.Code, "BLOCKED") || strings.Contains(response.Code, "LOCKED") || strings.Contains(response.Code, "VERIFICATION_REQUIRED") {
			result = model.D2SActionResultBlocked
		}
		userID := c.GetInt("id")
		if userID <= 0 && actionType == model.D2SActionLogout {
			if rawToken, err := c.Cookie(service.RefreshCookieName); err == nil {
				if sid, ok := service.RefreshTokenSID(rawToken); ok {
					if session, sessionErr := model.GetUserSessionBySID(sid); sessionErr == nil {
						userID = session.UserID
					}
				}
			}
		}
		if userID <= 0 && email != "" {
			var user model.User
			if err := model.DB.Select("id", "email").Where("email = ?", model.NormalizeEmail(email)).First(&user).Error; err == nil {
				userID = user.Id
				email = user.Email
			}
		}
		var userIDPointer *int
		if userID > 0 {
			userIDPointer = &userID
			if user, err := model.GetUserById(userID, false); err == nil && user != nil {
				email = user.Email
			}
		}
		sourcePort := common.ClientSourcePort(c.Request.RemoteAddr, c.RemoteIP(), c.ClientIP())
		var destinationIP *string
		var destinationPort *int
		if localAddr, ok := c.Request.Context().Value(http.LocalAddrContextKey).(net.Addr); ok && localAddr != nil {
			if host, rawPort, err := net.SplitHostPort(localAddr.String()); err == nil {
				destinationIP = &host
				if port, parseErr := strconv.Atoi(rawPort); parseErr == nil && port > 0 && port <= 65535 {
					destinationPort = &port
				}
			}
		}
		path := c.FullPath()
		if path == "" {
			path = c.Request.URL.Path
		}
		if err := model.RecordD2SUserAction(model.D2SActionLogInput{
			UserID: userIDPointer, Email: email, ActionType: actionType, EventTime: time.Now().UTC().Unix(),
			SourceIP: c.ClientIP(), SourcePort: sourcePort, DestinationIP: destinationIP,
			DestinationPort: destinationPort, UserAgent: c.Request.UserAgent(), RequestPath: path, ResultStatus: result,
		}); err != nil {
			common.SysLog("failed to append D2S user action log: " + err.Error())
			c.Writer.Header().Del("Set-Cookie")
			c.Writer.Header().Del("Content-Length")
			c.Writer.Header().Set("Cache-Control", "no-store")
			c.Writer.Header().Set("Content-Type", "application/json; charset=utf-8")
			c.Writer.WriteHeader(http.StatusServiceUnavailable)
			_, _ = c.Writer.Write([]byte(`{"success":false,"code":"USER_ACTION_AUDIT_UNAVAILABLE"}`))
			return
		}
		if err := capture.writeResponse(); err != nil {
			common.SysLog("failed to flush D2S user action response: " + err.Error())
		}
	}
}
