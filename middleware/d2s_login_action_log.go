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
	"github.com/gin-gonic/gin"
)

type d2sLoginResponseCapture struct {
	gin.ResponseWriter
	body bytes.Buffer
}

func (w *d2sLoginResponseCapture) Write(data []byte) (int, error) {
	return w.body.Write(data)
}

func (w *d2sLoginResponseCapture) WriteString(value string) (int, error) {
	return w.body.WriteString(value)
}

func (w *d2sLoginResponseCapture) WriteHeaderNow() {}

func (w *d2sLoginResponseCapture) Flush() {}

func (w *d2sLoginResponseCapture) writeResponse() error {
	w.ResponseWriter.WriteHeaderNow()
	if w.body.Len() == 0 {
		return nil
	}
	_, err := w.ResponseWriter.Write(w.body.Bytes())
	return err
}

func D2SLoginActionLog() gin.HandlerFunc {
	return func(c *gin.Context) {
		originalBody := c.Request.Body
		requestBody, _ := io.ReadAll(io.LimitReader(originalBody, 64*1024))
		c.Request.Body = io.NopCloser(io.MultiReader(bytes.NewReader(requestBody), originalBody))
		identity := struct {
			Email      string `json:"email"`
			Username   string `json:"username"`
			DeviceCode string `json:"device_code"`
		}{}
		_ = json.Unmarshal(requestBody, &identity)
		email := strings.TrimSpace(identity.Email)
		if email == "" && strings.Contains(identity.Username, "@") {
			email = identity.Username
		}
		capture := &d2sLoginResponseCapture{ResponseWriter: c.Writer}
		c.Writer = capture
		c.Next()
		c.Writer = capture.ResponseWriter
		if c.GetBool("d2s_login_event_recorded") {
			if err := capture.writeResponse(); err != nil {
				common.SysLog("failed to flush D2S login response: " + err.Error())
			}
			return
		}
		var payload struct {
			Success *bool  `json:"success"`
			Code    string `json:"code"`
			Data    struct {
				UserID     int    `json:"user_id"`
				DeviceHash string `json:"device_hash"`
			} `json:"data"`
		}
		_ = json.Unmarshal(capture.body.Bytes(), &payload)
		result := model.D2SActionResultFailure
		switch {
		case c.Writer.Status() == http.StatusForbidden || c.Writer.Status() == http.StatusLocked || c.Writer.Status() == http.StatusTooManyRequests:
			result = model.D2SActionResultBlocked
		case c.Writer.Status() >= http.StatusBadRequest:
			result = model.D2SActionResultFailure
		case payload.Success != nil && *payload.Success:
			result = model.D2SActionResultSuccess
		case strings.Contains(payload.Code, "BLOCKED") || strings.Contains(payload.Code, "LOCKED") || strings.Contains(payload.Code, "VERIFICATION_REQUIRED"):
			result = model.D2SActionResultBlocked
		}
		userID := c.GetInt("id")
		deviceFingerprint := payload.Data.DeviceHash
		if userID <= 0 && identity.DeviceCode != "" {
			if deviceCode, err := model.FindD2SDeviceCodeForAudit(identity.DeviceCode); err == nil && deviceCode != nil {
				userID = deviceCode.UserID
				if deviceFingerprint == "" {
					deviceFingerprint = deviceCode.DeviceHash
				}
			}
		}
		if userID <= 0 && payload.Data.UserID > 0 {
			userID = payload.Data.UserID
		}
		var deviceFingerprintPointer *string
		if len(deviceFingerprint) == 64 {
			deviceFingerprintPointer = &deviceFingerprint
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
		if address, ok := c.Request.Context().Value(http.LocalAddrContextKey).(net.Addr); ok && address != nil {
			if host, rawPort, err := net.SplitHostPort(address.String()); err == nil {
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
			UserID: userIDPointer, Email: email, ActionType: model.D2SActionLogin, EventTime: time.Now().UTC().Unix(),
			SourceIP: c.ClientIP(), SourcePort: sourcePort, DestinationIP: destinationIP,
			DestinationPort: destinationPort, UserAgent: c.Request.UserAgent(), DeviceFingerprint: deviceFingerprintPointer,
			RequestPath: path, ResultStatus: result,
		}); err != nil {
			common.SysLog("failed to append D2S login action log: " + err.Error())
			c.Writer.Header().Del("Set-Cookie")
			c.Writer.Header().Del("Content-Length")
			c.Writer.Header().Set("Cache-Control", "no-store")
			c.Writer.Header().Set("Content-Type", "application/json; charset=utf-8")
			c.Writer.WriteHeader(http.StatusServiceUnavailable)
			_, _ = c.Writer.Write([]byte(`{"success":false,"code":"LOGIN_AUDIT_UNAVAILABLE"}`))
			return
		}
		if err := capture.writeResponse(); err != nil {
			common.SysLog("failed to flush D2S login response: " + err.Error())
		}
	}
}
