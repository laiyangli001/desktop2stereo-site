package model

import (
	"encoding/hex"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

const (
	D2SExportApprovalTypeUserActionLogs = "USER_ACTION_LOGS"
	D2SExportApprovalTypeRegistrations  = "REGISTRATIONS"
	D2SExportApprovalPending            = "PENDING"
	D2SExportApprovalApproved           = "APPROVED"
	D2SExportApprovalRejected           = "REJECTED"
	D2SExportApprovalConsumed           = "CONSUMED"
)

var (
	ErrD2SExportApprovalInvalid        = errors.New("invalid export approval request")
	ErrD2SExportApprovalNotFound       = errors.New("export approval not found")
	ErrD2SExportApprovalSelfApproval   = errors.New("requester cannot approve own export")
	ErrD2SExportApprovalExpired        = errors.New("export approval expired")
	ErrD2SExportApprovalNotPending     = errors.New("export approval is not pending")
	ErrD2SExportApprovalNotApproved    = errors.New("export approval is not approved")
	ErrD2SExportApprovalFilterMismatch = errors.New("export approval filters do not match")
)

func validD2SExportApprovalType(value string) bool {
	switch value {
	case D2SExportApprovalTypeUserActionLogs, D2SExportApprovalTypeRegistrations:
		return true
	default:
		return false
	}
}

func createD2SAdminOperationAudit(tx *gorm.DB, entry *D2SAdminOperationAudit) error {
	if tx == nil || entry == nil || entry.OperatorID <= 0 || strings.TrimSpace(entry.OperatorEmail) == "" || strings.TrimSpace(entry.OperationType) == "" {
		return ErrD2SExportApprovalInvalid
	}
	if entry.ID == "" {
		entry.ID = uuid.NewString()
	}
	if entry.OperationTime <= 0 {
		entry.OperationTime = time.Now().UTC().Unix()
	}
	entry.OperatorEmail = truncateD2SText(NormalizeEmail(entry.OperatorEmail), 254)
	entry.TargetResource = truncateD2SText(entry.TargetResource, 255)
	entry.SourceIP = truncateD2SText(entry.SourceIP, 64)
	entry.Result = strings.ToUpper(strings.TrimSpace(entry.Result))
	if entry.Result == "" {
		entry.Result = "SUCCESS"
	}
	entry.DetailsJSON = truncateD2SText(entry.DetailsJSON, 8192)
	return tx.Create(entry).Error
}

func RecordD2SAdminOperationAuditTx(tx *gorm.DB, entry *D2SAdminOperationAudit) error {
	return createD2SAdminOperationAudit(tx, entry)
}

func CreateD2SAdminExportApprovalWithAudit(approval *D2SAdminExportApproval, audit *D2SAdminOperationAudit) error {
	if approval == nil || !validD2SExportApprovalType(approval.ExportType) || len(approval.FilterHash) != 64 || approval.FilterSnapshot == "" || strings.TrimSpace(approval.RequestReason) == "" || approval.RequestedByID <= 0 || strings.TrimSpace(approval.RequestedByEmail) == "" || approval.RequestedAt <= 0 || approval.ExpiresAt <= approval.RequestedAt {
		return ErrD2SExportApprovalInvalid
	}
	if _, err := hex.DecodeString(approval.FilterHash); err != nil {
		return ErrD2SExportApprovalInvalid
	}
	if approval.ID == "" {
		approval.ID = uuid.NewString()
	}
	approval.ExportType = strings.ToUpper(strings.TrimSpace(approval.ExportType))
	approval.RequestedByEmail = truncateD2SText(NormalizeEmail(approval.RequestedByEmail), 254)
	approval.RequestReason = truncateD2SText(strings.TrimSpace(approval.RequestReason), 500)
	approval.Status = D2SExportApprovalPending
	return DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(approval).Error; err != nil {
			return err
		}
		return createD2SAdminOperationAudit(tx, audit)
	})
}

func DecideD2SAdminExportApprovalWithAudit(id string, approverID int, approverEmail, decision, reason string, now int64, audit *D2SAdminOperationAudit) (*D2SAdminExportApproval, error) {
	id = strings.TrimSpace(id)
	decision = strings.ToUpper(strings.TrimSpace(decision))
	reason = truncateD2SText(strings.TrimSpace(reason), 500)
	if id == "" || approverID <= 0 || strings.TrimSpace(approverEmail) == "" || (decision != D2SExportApprovalApproved && decision != D2SExportApprovalRejected) || (decision == D2SExportApprovalRejected && reason == "") {
		return nil, ErrD2SExportApprovalInvalid
	}
	if now <= 0 {
		now = time.Now().UTC().Unix()
	}
	var approval D2SAdminExportApproval
	err := DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("id = ?", id).First(&approval).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrD2SExportApprovalNotFound
			}
			return err
		}
		if approval.RequestedByID == approverID {
			return ErrD2SExportApprovalSelfApproval
		}
		if approval.Status != D2SExportApprovalPending {
			return ErrD2SExportApprovalNotPending
		}
		if approval.ExpiresAt <= now {
			return ErrD2SExportApprovalExpired
		}
		result := tx.Model(&D2SAdminExportApproval{}).
			Where("id = ? AND status = ? AND expires_at > ? AND requested_by_id <> ?", id, D2SExportApprovalPending, now, approverID).
			Updates(map[string]any{
				"status": decision, "reviewed_by_id": approverID,
				"reviewed_by_email": truncateD2SText(NormalizeEmail(approverEmail), 254),
				"reviewed_at":       now, "review_reason": reason,
			})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrD2SExportApprovalNotPending
		}
		if audit == nil {
			return ErrD2SExportApprovalInvalid
		}
		audit.OperationTime = now
		return createD2SAdminOperationAudit(tx, audit)
	})
	if err != nil {
		return nil, err
	}
	approval.Status = decision
	approval.ReviewedByID = approverID
	approval.ReviewedByEmail = NormalizeEmail(approverEmail)
	approval.ReviewedAt = now
	approval.ReviewReason = reason
	return &approval, nil
}

func ConsumeD2SAdminExportApprovalWithAudit(id, exportType, filterHash string, requesterID int, now int64, audit *D2SAdminOperationAudit) error {
	id = strings.TrimSpace(id)
	exportType = strings.ToUpper(strings.TrimSpace(exportType))
	if id == "" || !validD2SExportApprovalType(exportType) || len(filterHash) != 64 || requesterID <= 0 || audit == nil {
		return ErrD2SExportApprovalInvalid
	}
	if _, err := hex.DecodeString(filterHash); err != nil {
		return ErrD2SExportApprovalInvalid
	}
	if now <= 0 {
		now = time.Now().UTC().Unix()
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		var approval D2SAdminExportApproval
		if err := tx.Where("id = ?", id).First(&approval).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrD2SExportApprovalNotFound
			}
			return err
		}
		if approval.RequestedByID != requesterID || approval.ExportType != exportType || approval.FilterHash != filterHash {
			return ErrD2SExportApprovalFilterMismatch
		}
		if approval.ExpiresAt <= now {
			return ErrD2SExportApprovalExpired
		}
		if approval.Status != D2SExportApprovalApproved {
			return ErrD2SExportApprovalNotApproved
		}
		result := tx.Model(&D2SAdminExportApproval{}).
			Where("id = ? AND status = ? AND expires_at > ? AND requested_by_id = ? AND export_type = ? AND filter_hash = ?",
				id, D2SExportApprovalApproved, now, requesterID, exportType, filterHash).
			Updates(map[string]any{"status": D2SExportApprovalConsumed, "consumed_by_id": requesterID, "consumed_at": now})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrD2SExportApprovalNotApproved
		}
		audit.OperationTime = now
		return createD2SAdminOperationAudit(tx, audit)
	})
}

func GetD2SAdminExportApproval(id string) (*D2SAdminExportApproval, error) {
	var approval D2SAdminExportApproval
	err := DB.Where("id = ?", strings.TrimSpace(id)).First(&approval).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrD2SExportApprovalNotFound
	}
	if err != nil {
		return nil, err
	}
	return &approval, nil
}

func ListPendingD2SAdminExportApprovals(now int64, page, pageSize int) ([]D2SAdminExportApproval, int64, error) {
	if now <= 0 {
		now = time.Now().UTC().Unix()
	}
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 50
	}
	query := DB.Model(&D2SAdminExportApproval{}).Where("status = ? AND expires_at > ?", D2SExportApprovalPending, now)
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var approvals []D2SAdminExportApproval
	err := query.Order("requested_at ASC").Order("id ASC").Offset((page - 1) * pageSize).Limit(pageSize).Find(&approvals).Error
	return approvals, total, err
}
