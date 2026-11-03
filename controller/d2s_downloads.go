package controller

import (
	"errors"
	"net"
	"net/http"
	"strconv"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

var d2sDownloadTargets = map[string]struct{}{
	"release": {},
	"quark":   {},
	"baidu":   {},
}

func D2STrackDownload(c *gin.Context) {
	target := c.Param("target")
	if _, ok := d2sDownloadTargets[target]; !ok {
		c.JSON(http.StatusBadRequest, gin.H{
			"version": 1, "success": false, "request_id": d2sRequestID(c),
			"error": gin.H{"code": "download_target_invalid"},
		})
		return
	}

	userID := c.GetInt("id")
	var user *model.User
	if userID > 0 {
		if blocked, err := model.IsD2SUserLoginBlocked(userID); err != nil {
			d2sError(c, err)
			return
		} else if blocked {
			c.JSON(http.StatusLocked, gin.H{
				"version": 1, "success": false, "request_id": d2sRequestID(c),
				"error": gin.H{"code": "account_login_blocked"},
			})
			return
		}

		var err error
		user, err = model.GetUserById(userID, false)
		if err != nil {
			d2sError(c, err)
			return
		}
		if user == nil {
			d2sError(c, errors.New("download account was not found"))
			return
		}
		if user.EmailVerifiedAt <= 0 {
			d2sError(c, model.ErrD2SEmailVerificationRequired)
			return
		}
	}

	if err := recordD2SDownloadClick(c, user, target); err != nil {
		d2sError(c, err)
		return
	}
	c.Set("d2s_user_action_recorded", true)
	d2sSuccess(c, http.StatusOK, gin.H{"target": target, "logged": true})
}

func recordD2SDownloadClick(c *gin.Context, user *model.User, target string) error {
	var userID *int
	email := ""
	if user != nil {
		userIDValue := user.Id
		userID = &userIDValue
		email = user.Email
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

	// Device fingerprint is unavailable in the current browser/server contract.
	return model.RecordD2SUserAction(model.D2SActionLogInput{
		UserID: userID, Email: email, ActionType: model.D2SActionDownload,
		EventTime: time.Now().UTC().Unix(), SourceIP: c.ClientIP(), SourcePort: sourcePort,
		DestinationIP: destinationIP, DestinationPort: destinationPort,
		UserAgent: c.Request.UserAgent(), RequestPath: "/api/v1/downloads/" + target + "/track",
		ResultStatus: model.D2SActionResultSuccess,
	})
}
