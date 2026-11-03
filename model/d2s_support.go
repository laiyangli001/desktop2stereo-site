package model

import (
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/QuantumNous/new-api/common"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

const (
	D2SSupportOpen                  = "OPEN"
	D2SSupportInProgress            = "IN_PROGRESS"
	D2SSupportResolved              = "RESOLVED"
	D2SSupportClosed                = "CLOSED"
	D2SSupportTicketRetentionMonths = 6
	d2sSupportCleanupBatchSize      = 500
	d2sSupportCleanupSystemEmail    = "system@d2s.invalid"
)

var (
	ErrD2SSupportTicketNotFound      = errors.New("support ticket not found")
	ErrD2SSupportOwnerUnavailable    = errors.New("support tickets require an active verified account")
	ErrD2SSupportTicketCleanupFailed = errors.New("support ticket retention cleanup failed")
)

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
	Status         string `json:"status" gorm:"type:varchar(20);not null;index:idx_d2s_support_status_closed,priority:1"`
	ClosedAt       int64  `json:"-" gorm:"type:bigint;not null;default:0;index:idx_d2s_support_status_closed,priority:2"`
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
	return DB.Transaction(func(tx *gorm.DB) error {
		var user User
		if err := lockForUpdate(tx).Where("id = ?", ticket.UserID).First(&user).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrD2SSupportOwnerUnavailable
			}
			return err
		}
		if user.Status != common.UserStatusEnabled || user.EmailVerifiedAt <= 0 {
			return ErrD2SSupportOwnerUnavailable
		}
		return tx.Create(ticket).Error
	})
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

func UpdateD2SSupportTicketWithAudit(id, status, reply string, adminID int, adminEmail, sourceIP string) (*D2SSupportTicket, error) {
	status = strings.ToUpper(strings.TrimSpace(status))
	switch status {
	case D2SSupportOpen, D2SSupportInProgress, D2SSupportResolved, D2SSupportClosed:
	default:
		return nil, errors.New("invalid support ticket status")
	}
	var ticket D2SSupportTicket
	err := DB.Transaction(func(tx *gorm.DB) error {
		if err := lockForUpdate(tx).Where("id = ?", strings.TrimSpace(id)).First(&ticket).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrD2SSupportTicketNotFound
			}
			return err
		}
		ticket.UpdatedAt = time.Now().UTC().Unix()
		if status == D2SSupportClosed && ticket.Status != D2SSupportClosed {
			ticket.ClosedAt = ticket.UpdatedAt
		} else if status != D2SSupportClosed {
			ticket.ClosedAt = 0
		}
		ticket.Status = status
		ticket.AdminReply = strings.TrimSpace(reply)
		ticket.HandledByID = adminID
		ticket.HandledByEmail = NormalizeEmail(adminEmail)
		if err := tx.Save(&ticket).Error; err != nil {
			return err
		}
		return tx.Create(&D2SAdminOperationAudit{
			ID: uuid.NewString(), OperatorID: adminID, OperatorEmail: NormalizeEmail(adminEmail),
			OperationType: "UPDATE_SUPPORT_TICKET", OperationTime: ticket.UpdatedAt,
			SourceIP: truncateD2SText(sourceIP, 64), Result: "SUCCESS",
		}).Error
	})
	return &ticket, err
}

func d2sSupportTicketRetentionCutoff(now time.Time) int64 {
	now = now.UTC()
	targetMonth := time.Date(now.Year(), now.Month()-D2SSupportTicketRetentionMonths, 1, now.Hour(), now.Minute(), now.Second(), 0, time.UTC)
	lastDay := targetMonth.AddDate(0, 1, -1).Day()
	day := now.Day()
	if day > lastDay {
		day = lastDay
	}
	return time.Date(targetMonth.Year(), targetMonth.Month(), day, now.Hour(), now.Minute(), now.Second(), 0, time.UTC).Unix()
}

// PruneD2SSupportTickets permanently removes closed tickets after six calendar months.
func PruneD2SSupportTickets(now int64) (int64, error) {
	if DB == nil {
		return 0, errors.New("support ticket database is unavailable")
	}
	if now <= 0 {
		now = time.Now().UTC().Unix()
	}
	cutoff := d2sSupportTicketRetentionCutoff(time.Unix(now, 0))
	if err := DB.Model(&D2SSupportTicket{}).
		Where("status = ? AND closed_at = 0 AND updated_at > 0", D2SSupportClosed).
		Update("closed_at", gorm.Expr("updated_at")).Error; err != nil {
		return 0, fmt.Errorf("backfill closed support ticket timestamps: %w", err)
	}
	var deletedTotal int64
	for {
		var candidateCount int64
		var rangeStart int64
		stage := "select_eligible_tickets"
		err := DB.Transaction(func(tx *gorm.DB) error {
			var tickets []D2SSupportTicket
			if err := lockForUpdate(tx).
				Where("status = ? AND closed_at > 0 AND closed_at <= ?", D2SSupportClosed, cutoff).
				Order("closed_at ASC").Order("id ASC").Limit(d2sSupportCleanupBatchSize).Find(&tickets).Error; err != nil {
				return err
			}
			candidateCount = int64(len(tickets))
			if candidateCount == 0 {
				return nil
			}
			rangeStart = tickets[0].ClosedAt
			ids := make([]string, 0, len(tickets))
			for _, ticket := range tickets {
				ids = append(ids, ticket.ID)
			}
			details, err := common.Marshal(map[string]any{
				"cutoff_at": cutoff, "range_start": rangeStart, "range_end": cutoff,
				"record_count": candidateCount, "result": "SUCCESS",
			})
			if err != nil {
				stage = "marshal_success_audit"
				return err
			}
			stage = "write_success_audit"
			audit := D2SAdminOperationAudit{
				ID: uuid.NewString(), OperatorEmail: d2sSupportCleanupSystemEmail,
				OperationType: "CLEANUP_SUPPORT_TICKETS", OperationTime: now,
				Result: "SUCCESS", DetailsJSON: string(details),
			}
			if err := tx.Create(&audit).Error; err != nil {
				return err
			}
			stage = "delete_tickets"
			result := tx.Where("id IN ?", ids).Delete(&D2SSupportTicket{})
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected != candidateCount {
				return errors.New("support ticket cleanup count changed inside transaction")
			}
			return nil
		})
		if err != nil {
			failureDetails, marshalErr := common.Marshal(map[string]any{
				"cutoff_at": cutoff, "range_start": rangeStart, "range_end": cutoff,
				"record_count": 0, "candidate_record_count": candidateCount,
				"failed_stage": stage, "failure_code": "CLEANUP_TRANSACTION_FAILED", "result": "FAILURE",
			})
			if marshalErr != nil {
				return deletedTotal, fmt.Errorf("%w: cleanup failed and failure details could not be encoded", ErrD2SSupportTicketCleanupFailed)
			}
			failureAudit := D2SAdminOperationAudit{
				ID: uuid.NewString(), OperatorEmail: d2sSupportCleanupSystemEmail,
				OperationType: "CLEANUP_SUPPORT_TICKETS", OperationTime: now,
				Result: "FAILURE", DetailsJSON: string(failureDetails),
			}
			auditErr := DB.Transaction(func(tx *gorm.DB) error { return tx.Create(&failureAudit).Error })
			if auditErr != nil {
				return deletedTotal, fmt.Errorf("%w: cleanup failed and failure audit could not be persisted", ErrD2SSupportTicketCleanupFailed)
			}
			return deletedTotal, fmt.Errorf("%w: cleanup transaction failed at %s", ErrD2SSupportTicketCleanupFailed, stage)
		}
		if candidateCount == 0 {
			return deletedTotal, nil
		}
		deletedTotal += candidateCount
	}
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
