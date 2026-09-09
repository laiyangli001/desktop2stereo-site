package model

import (
	"strings"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

type EmailDeliveryLog struct {
	ID        int64  `json:"id" gorm:"primaryKey"`
	CreatedAt int64  `json:"created_at" gorm:"index"`
	Scene     string `json:"scene" gorm:"index;size:64"`
	Recipient string `json:"recipient" gorm:"size:320"`
	Status    string `json:"status" gorm:"index;size:32"`
	RequestID string `json:"request_id,omitempty" gorm:"size:128"`
	MessageID string `json:"message_id,omitempty" gorm:"size:256"`
	Error     string `json:"error,omitempty" gorm:"size:1024"`
}

func RecordEmailDelivery(scene string, recipient string, status string, requestID string, messageID string, sendErr error) {
	entry := &EmailDeliveryLog{
		CreatedAt: common.GetTimestamp(),
		Scene:     strings.TrimSpace(scene),
		Recipient: common.MaskEmail(strings.TrimSpace(recipient)),
		Status:    strings.TrimSpace(status),
		RequestID: strings.TrimSpace(requestID),
		MessageID: strings.TrimSpace(messageID),
	}
	if sendErr != nil {
		entry.Error = common.MaskSensitiveInfo(sendErr.Error())
	}
	if err := DB.Create(entry).Error; err != nil {
		common.SysLog("failed to record email delivery log: " + err.Error())
	}
}

func ListEmailDeliveryLogs(limit int) ([]*EmailDeliveryLog, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	var logs []*EmailDeliveryLog
	err := DB.Order("created_at DESC").Limit(limit).Find(&logs).Error
	return logs, err
}

type EmailDeliveryLogPage struct {
	Logs       []*EmailDeliveryLog `json:"logs"`
	Total      int64               `json:"total"`
	Page       int                 `json:"page"`
	PageSize   int                 `json:"page_size"`
	TotalPages int                 `json:"total_pages"`
}

func PageEmailDeliveryLogs(page, pageSize int) (EmailDeliveryLogPage, error) {
	if page < 1 {
		page = 1
	}
	if pageSize <= 0 || pageSize > 20 {
		pageSize = 20
	}

	var total int64
	if err := DB.Model(&EmailDeliveryLog{}).Count(&total).Error; err != nil {
		return EmailDeliveryLogPage{}, err
	}
	result := EmailDeliveryLogPage{Total: total, Page: page, PageSize: pageSize}
	if total > 0 {
		result.TotalPages = int((total + int64(pageSize) - 1) / int64(pageSize))
	} else {
		result.TotalPages = 1
	}

	var logs []*EmailDeliveryLog
	err := DB.Order("created_at DESC").Limit(pageSize).
		Offset((page - 1) * pageSize).Find(&logs).Error
	result.Logs = logs
	return result, err
}

func DeleteEmailDeliveryLog(id int64) error {
	return DB.Delete(&EmailDeliveryLog{}, id).Error
}

func DeleteAllEmailDeliveryLogs() error {
	return DB.Session(&gorm.Session{AllowGlobalUpdate: true}).Delete(&EmailDeliveryLog{}).Error
}
