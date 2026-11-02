package model

import (
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

const (
	D2SSupportOpen       = "OPEN"
	D2SSupportInProgress = "IN_PROGRESS"
	D2SSupportResolved   = "RESOLVED"
	D2SSupportClosed     = "CLOSED"
)

var ErrD2SSupportTicketNotFound = errors.New("support ticket not found")

type D2SSupportTicket struct {
	ID             string `json:"id" gorm:"type:varchar(64);primaryKey"`
	UserID         int    `json:"user_id" gorm:"not null;index:idx_d2s_support_user_created,priority:1"`
	Category       string `json:"category" gorm:"type:varchar(32);not null;index"`
	ContactEmail   string `json:"contact_email" gorm:"type:varchar(254);not null;index"`
	OrderID        string `json:"order_id" gorm:"type:varchar(64);index"`
	Subject        string `json:"subject" gorm:"type:varchar(200);not null"`
	Message        string `json:"message" gorm:"type:text;not null"`
	SourceIP       string `json:"source_ip" gorm:"type:varchar(64)"`
	UserAgent      string `json:"user_agent" gorm:"type:varchar(512)"`
	Status         string `json:"status" gorm:"type:varchar(20);not null;index"`
	AdminReply     string `json:"admin_reply" gorm:"type:text"`
	CreatedAt      int64  `json:"created_at" gorm:"type:bigint;not null;index:idx_d2s_support_user_created,priority:2"`
	UpdatedAt      int64  `json:"updated_at" gorm:"type:bigint;not null"`
	HandledByID    int    `json:"handled_by_id" gorm:"not null;default:0"`
	HandledByEmail string `json:"handled_by_email" gorm:"type:varchar(254)"`
}

type D2SUserSupportTicket struct {
	ID         string `json:"id"`
	Category   string `json:"category"`
	OrderID    string `json:"order_id"`
	Subject    string `json:"subject"`
	Message    string `json:"message"`
	Status     string `json:"status"`
	AdminReply string `json:"admin_reply"`
	CreatedAt  int64  `json:"created_at"`
	UpdatedAt  int64  `json:"updated_at"`
}

func (D2SSupportTicket) TableName() string { return "d2s_support_tickets" }

func CreateD2SSupportTicket(ticket *D2SSupportTicket) error {
	if ticket == nil || ticket.UserID <= 0 || strings.TrimSpace(ticket.ContactEmail) == "" || !ValidD2SSupportSubmissionLengths(ticket.Subject, ticket.Message) {
		return errors.New("invalid support ticket")
	}
	now := time.Now().UTC().Unix()
	ticket.ID = uuid.NewString()
	ticket.ContactEmail = NormalizeEmail(ticket.ContactEmail)
	ticket.OrderID = truncateD2SText(strings.TrimSpace(ticket.OrderID), 64)
	ticket.Subject = strings.TrimSpace(ticket.Subject)
	ticket.Message = strings.TrimSpace(ticket.Message)
	ticket.SourceIP = truncateD2SText(strings.TrimSpace(ticket.SourceIP), 64)
	ticket.UserAgent = truncateD2SText(ticket.UserAgent, 512)
	ticket.Status = D2SSupportOpen
	ticket.CreatedAt = now
	ticket.UpdatedAt = now
	return DB.Create(ticket).Error
}

func ListD2SSupportTickets(status string, page, pageSize int) ([]D2SSupportTicket, int64, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 50
	}
	query := DB.Model(&D2SSupportTicket{})
	if status = strings.ToUpper(strings.TrimSpace(status)); status != "" {
		switch status {
		case D2SSupportOpen, D2SSupportInProgress, D2SSupportResolved, D2SSupportClosed:
			query = query.Where("status = ?", status)
		default:
			return nil, 0, errors.New("invalid support ticket status")
		}
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var tickets []D2SSupportTicket
	err := query.Order("created_at DESC").Order("id DESC").Offset((page - 1) * pageSize).Limit(pageSize).Find(&tickets).Error
	return tickets, total, err
}

func ListD2SUserSupportTickets(userID, page, pageSize int) ([]D2SUserSupportTicket, int64, error) {
	if userID <= 0 {
		return nil, 0, errors.New("invalid support ticket owner")
	}
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 50
	}
	query := DB.Model(&D2SSupportTicket{}).Where("user_id = ?", userID)
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var tickets []D2SUserSupportTicket
	err := query.Select("id, category, order_id, subject, message, status, admin_reply, created_at, updated_at").
		Order("created_at DESC").Order("id DESC").Offset((page - 1) * pageSize).Limit(pageSize).Find(&tickets).Error
	return tickets, total, err
}

func UpdateD2SSupportTicketWithAudit(id, status, reply string, adminID int, adminEmail, sourceIP, details string) (*D2SSupportTicket, error) {
	status = strings.ToUpper(strings.TrimSpace(status))
	switch status {
	case D2SSupportOpen, D2SSupportInProgress, D2SSupportResolved, D2SSupportClosed:
	default:
		return nil, errors.New("invalid support ticket status")
	}
	var ticket D2SSupportTicket
	err := DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("id = ?", strings.TrimSpace(id)).First(&ticket).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrD2SSupportTicketNotFound
			}
			return err
		}
		ticket.Status = status
		ticket.AdminReply = strings.TrimSpace(reply)
		ticket.UpdatedAt = time.Now().UTC().Unix()
		ticket.HandledByID = adminID
		ticket.HandledByEmail = NormalizeEmail(adminEmail)
		if err := tx.Save(&ticket).Error; err != nil {
			return err
		}
		return tx.Create(&D2SAdminOperationAudit{
			ID: uuid.NewString(), OperatorID: adminID, OperatorEmail: NormalizeEmail(adminEmail),
			OperationType: "UPDATE_SUPPORT_TICKET", OperationTime: ticket.UpdatedAt,
			TargetResource: "d2s_support_tickets:" + ticket.ID, SourceIP: truncateD2SText(sourceIP, 64),
			Result: "SUCCESS", DetailsJSON: truncateD2SText(details, 8192),
		}).Error
	})
	return &ticket, err
}

func ValidD2SSupportSubmissionLengths(subject, message string) bool {
	return utf8.RuneCountInString(strings.TrimSpace(subject)) >= 1 &&
		utf8.RuneCountInString(strings.TrimSpace(subject)) <= 200 &&
		utf8.RuneCountInString(strings.TrimSpace(message)) >= 10 &&
		utf8.RuneCountInString(strings.TrimSpace(message)) <= 5000
}

func ValidD2SSupportReplyLength(reply string) bool {
	return utf8.RuneCountInString(reply) <= 5000
}
