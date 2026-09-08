package controller

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/gin-gonic/gin"
)

type tencentSESTestSendRequest struct {
	Scene      string            `json:"scene"`
	TemplateID uint64            `json:"template_id"`
	To         string            `json:"to"`
	Subject    string            `json:"subject"`
	Data       map[string]string `json:"data"`
}

func GetTencentSESSettings(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "", "data": common.TencentSESConfigPublic()})
}

func GetTencentSESDeliveryLogs(c *gin.Context) {
	limit, _ := strconv.Atoi(c.Query("limit"))
	logs, err := model.ListEmailDeliveryLogs(limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "", "data": logs})
}

func TestTencentSESConnection(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 15*time.Second)
	defer cancel()
	if err := common.TestTencentSESConnection(ctx); err != nil {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "Tencent SES connection succeeded"})
}

func TestTencentSESSend(c *gin.Context) {
	var request tencentSESTestSendRequest
	if err := common.DecodeJson(c.Request.Body, &request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "无效的请求参数"})
		return
	}
	request.To = strings.TrimSpace(request.To)
	if request.To == "" || request.Scene == "" || request.Subject == "" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "收件人、业务场景和主题不能为空"})
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 20*time.Second)
	defer cancel()
	request.Data = fillTencentSESTestData(request.Scene, request.Data)
	result, err := common.SendTencentSESTemplate(ctx, common.TemplateEmailMessage{
		Scene:        request.Scene,
		TemplateID:   request.TemplateID,
		To:           []string{request.To},
		Subject:      request.Subject,
		TemplateData: request.Data,
	})
	model.RecordEmailDelivery(request.Scene, request.To, deliveryStatus(err), result.RequestID, result.MessageID, err)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": err.Error()})
		return
	}
	common.SysLog("Tencent SES test email sent: scene=" + request.Scene + ", request_id=" + result.RequestID)
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "测试邮件发送成功", "data": result})
}

func fillTencentSESTestData(scene string, data map[string]string) map[string]string {
	filled := make(map[string]string, len(data)+3)
	for key, value := range data {
		filled[key] = value
	}
	setDefault := func(key, value string) {
		if strings.TrimSpace(filled[key]) == "" {
			filled[key] = value
		}
	}

	switch scene {
	case "email_verification":
		setDefault("token", "123456")
		setDefault("code", "123456")
		setDefault("expire_minutes", fmt.Sprintf("%d", common.VerificationValidMinutes))
		setDefault("system_name", common.SystemName)
	case "password_reset":
		baseURL := strings.TrimRight(system_setting.ServerAddress, "/")
		if baseURL == "" {
			baseURL = "https://example.com"
		}
		setDefault("email", "test%40example.com")
		setDefault("token", "test-token")
		setDefault("reset_url", baseURL+"/user/reset?email="+filled["email"]+"&token="+filled["token"])
		setDefault("expire_minutes", fmt.Sprintf("%d", common.VerificationValidMinutes))
		setDefault("system_name", common.SystemName)
	case "system_notification":
		setDefault("title", "测试通知")
		setDefault("content", "这是一封腾讯云邮件推送测试邮件，用于验证模板、发信域名和服务配置。")
		setDefault("system_name", common.SystemName)
	}
	return filled
}

func deliveryStatus(err error) string {
	if err != nil {
		return "failed"
	}
	return "sent"
}
