package model

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
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
	D2SMinimumAuditRetentionDays = 366
	D2SMaximumAuditRetentionDays = 2_147_483_647

	D2SActionRegister      = "REGISTER"
	D2SActionLogin         = "LOGIN"
	D2SActionLogout        = "LOGOUT"
	D2SActionPurchase      = "PURCHASE"
	D2SActionDownload      = "DOWNLOAD"
	D2SActionPasswordReset = "PASSWORD_RESET"
	D2SActionProfileUpdate = "PROFILE_UPDATE"

	D2SActionResultSuccess = "SUCCESS"
	D2SActionResultFailure = "FAILURE"
	D2SActionResultBlocked = "BLOCKED"

	D2SAdminRoleSuperAdmin      = "super_admin"
	D2SAdminRoleAuditReader     = "audit_reader"
	D2SAdminRoleAccountOperator = "account_operator"
	D2SAdminStatusActive        = "active"
	D2SAdminStatusDisabled      = "disabled"
)

var (
	ErrD2SAppendOnlyRecord         = errors.New("audit record is append-only")
	ErrD2SAuditRetention           = errors.New("audit retention must be between 366 and 2147483647 days")
	ErrD2SAuditCleanupUnavailable  = errors.New("dedicated audit cleanup database is unavailable")
	ErrD2SAuditCleanupFailed       = errors.New("audit cleanup transaction failed")
	ErrD2SAuditCleanupFailureAudit = errors.New("audit cleanup failed and failure audit could not be persisted")
	ErrD2SAdminInvalid             = errors.New("admin credentials are invalid")
	ErrD2SAdminLocked              = errors.New("admin account is temporarily locked")
	ErrD2SAdminForbidden           = errors.New("admin role is not permitted")
)

type D2SUserActionLog struct {
	ID                string  `json:"id" gorm:"type:varchar(64);primaryKey"`
	UserID            *int    `json:"user_id" gorm:"index:idx_d2s_action_user_time,priority:1"`
	Email             string  `json:"email" gorm:"type:varchar(254);index"`
	ActionType        string  `json:"action_type" gorm:"type:varchar(32);not null;index:idx_d2s_action_type_time,priority:1"`
	EventTime         int64   `json:"event_time" gorm:"type:bigint;not null;index:idx_d2s_action_user_time,priority:2;index:idx_d2s_action_type_time,priority:2;index:idx_d2s_action_ip_time,priority:2"`
	SourceIP          string  `json:"source_ip" gorm:"type:varchar(64);not null;index:idx_d2s_action_ip_time,priority:1"`
	SourcePort        *int    `json:"source_port" gorm:"type:int"`
	DestinationIP     *string `json:"destination_ip" gorm:"type:varchar(64)"`
	DestinationPort   *int    `json:"destination_port" gorm:"type:int"`
	UserAgent         string  `json:"user_agent" gorm:"type:varchar(512)"`
	DeviceFingerprint *string `json:"device_fingerprint" gorm:"type:char(64)"`
	RequestPath       string  `json:"request_path" gorm:"type:varchar(2048)"`
	ResultStatus      string  `json:"result_status" gorm:"type:varchar(16);not null;index"`
	RecordHash        string  `json:"record_hash" gorm:"type:char(64);not null"`
}

func (D2SUserActionLog) TableName() string           { return "d2s_user_action_logs" }
func (D2SUserActionLog) BeforeUpdate(*gorm.DB) error { return ErrD2SAppendOnlyRecord }
func (entry *D2SUserActionLog) BeforeDelete(tx *gorm.DB) error {
	if allowed, ok := tx.Get("d2s_audit_retention_cleanup"); ok && allowed == true {
		return nil
	}
	return ErrD2SAppendOnlyRecord
}

type D2SRegistrationRecord struct {
	ID                string `json:"id" gorm:"type:varchar(64);primaryKey"`
	UserID            int    `json:"user_id" gorm:"not null;uniqueIndex"`
	Email             string `json:"email" gorm:"type:varchar(254);not null;index"`
	RegisterIP        string `json:"register_ip" gorm:"type:varchar(64);not null"`
	RegisterTime      int64  `json:"register_time" gorm:"type:bigint;not null;index"`
	RegisterUserAgent string `json:"register_user_agent" gorm:"type:varchar(512)"`
	EmailVerified     bool   `json:"email_verified" gorm:"not null"`
	EmailVerifiedAt   int64  `json:"email_verified_at" gorm:"type:bigint;not null"`
}

func (D2SRegistrationRecord) TableName() string           { return "d2s_registration_records" }
func (D2SRegistrationRecord) BeforeUpdate(*gorm.DB) error { return ErrD2SAppendOnlyRecord }
func (entry *D2SRegistrationRecord) BeforeDelete(tx *gorm.DB) error {
	if allowed, ok := tx.Get("d2s_audit_retention_cleanup"); ok && allowed == true {
		return nil
	}
	return ErrD2SAppendOnlyRecord
}

type D2SAdminAccount struct {
	ID                     int    `json:"id" gorm:"primaryKey;autoIncrement"`
	Email                  string `json:"email" gorm:"type:varchar(254);uniqueIndex;not null"`
	PasswordHash           string `json:"-" gorm:"type:varchar(128);not null"`
	Role                   string `json:"role" gorm:"type:varchar(32);not null;index"`
	Status                 string `json:"status" gorm:"type:varchar(16);not null;index"`
	FailedAttempts         int    `json:"-" gorm:"not null;default:0"`
	FailureWindowStartedAt int64  `json:"-" gorm:"type:bigint;not null;default:0"`
	LockedUntil            int64  `json:"-" gorm:"type:bigint;not null;default:0"`
	CreatedAt              int64  `json:"created_at" gorm:"type:bigint;not null"`
	UpdatedAt              int64  `json:"updated_at" gorm:"type:bigint;not null"`
	LastLoginAt            int64  `json:"last_login_at" gorm:"type:bigint;not null;default:0"`
}

func (D2SAdminAccount) TableName() string { return "d2s_admin_accounts" }

type D2SAdminSession struct {
	TokenHash string `json:"-" gorm:"type:char(64);primaryKey"`
	AdminID   int    `json:"admin_id" gorm:"not null;index"`
	CreatedAt int64  `json:"created_at" gorm:"type:bigint;not null"`
	ExpiresAt int64  `json:"expires_at" gorm:"type:bigint;not null;index"`
	RevokedAt int64  `json:"revoked_at" gorm:"type:bigint;not null;default:0"`
	SourceIP  string `json:"source_ip" gorm:"type:varchar(64)"`
	UserAgent string `json:"user_agent" gorm:"type:varchar(512)"`
}

func (D2SAdminSession) TableName() string { return "d2s_admin_sessions" }

type D2SAdminOperationAudit struct {
	ID             string `json:"id" gorm:"type:varchar(64);primaryKey"`
	OperatorID     int    `json:"operator_id" gorm:"not null;index"`
	OperatorEmail  string `json:"operator_email" gorm:"type:varchar(254);not null;index"`
	OperationType  string `json:"operation_type" gorm:"type:varchar(64);not null;index"`
	OperationTime  int64  `json:"operation_time" gorm:"type:bigint;not null;index"`
	TargetResource string `json:"target_resource" gorm:"type:varchar(255)"`
	SourceIP       string `json:"source_ip" gorm:"type:varchar(64)"`
	Result         string `json:"result" gorm:"type:varchar(16);not null;index"`
	DetailsJSON    string `json:"details" gorm:"type:text"`
}

func (D2SAdminOperationAudit) TableName() string           { return "d2s_admin_operation_audits" }
func (D2SAdminOperationAudit) BeforeUpdate(*gorm.DB) error { return ErrD2SAppendOnlyRecord }
func (entry *D2SAdminOperationAudit) BeforeDelete(tx *gorm.DB) error {
	if allowed, ok := tx.Get("d2s_audit_retention_cleanup"); ok && allowed == true {
		return nil
	}
	return ErrD2SAppendOnlyRecord
}

type D2SAdminExportApproval struct {
	ID               string `json:"id" gorm:"type:varchar(64);primaryKey"`
	ExportType       string `json:"export_type" gorm:"type:varchar(32);not null;index"`
	FilterHash       string `json:"filter_hash" gorm:"type:char(64);not null"`
	FilterSnapshot   string `json:"filter_snapshot" gorm:"type:text;not null"`
	RequestReason    string `json:"request_reason" gorm:"type:varchar(500);not null"`
	RequestedByID    int    `json:"requested_by_id" gorm:"not null;index"`
	RequestedByEmail string `json:"requested_by_email" gorm:"type:varchar(254);not null"`
	RequestedAt      int64  `json:"requested_at" gorm:"type:bigint;not null;index"`
	ExpiresAt        int64  `json:"expires_at" gorm:"type:bigint;not null;index"`
	Status           string `json:"status" gorm:"type:varchar(16);not null;index"`
	ReviewedByID     int    `json:"reviewed_by_id" gorm:"not null;default:0"`
	ReviewedByEmail  string `json:"reviewed_by_email" gorm:"type:varchar(254)"`
	ReviewedAt       int64  `json:"reviewed_at" gorm:"type:bigint;not null;default:0"`
	ReviewReason     string `json:"review_reason" gorm:"type:varchar(500)"`
	ConsumedByID     int    `json:"consumed_by_id" gorm:"not null;default:0"`
	ConsumedAt       int64  `json:"consumed_at" gorm:"type:bigint;not null;default:0"`
}

func (D2SAdminExportApproval) TableName() string { return "d2s_admin_export_approvals" }

type D2SUserSecurityControl struct {
	UserID          int    `json:"user_id" gorm:"primaryKey"`
	LoginBlocked    bool   `json:"login_blocked" gorm:"not null;default:false"`
	PurchaseBlocked bool   `json:"purchase_blocked" gorm:"not null;default:false"`
	Reason          string `json:"reason" gorm:"type:varchar(512)"`
	UpdatedAt       int64  `json:"updated_at" gorm:"type:bigint;not null"`
	UpdatedBy       int    `json:"updated_by" gorm:"not null"`
}

func (D2SUserSecurityControl) TableName() string { return "d2s_user_security_controls" }

type D2SIdentityVerification struct {
	UserID                      int    `json:"user_id" gorm:"primaryKey"`
	IDDocumentType              string `json:"id_document_type" gorm:"type:varchar(40)"`
	IDDocumentNumberHash        string `json:"id_document_number_hash" gorm:"type:char(64)"`
	IDDocumentNumberHashVersion string `json:"id_document_number_hash_version" gorm:"type:varchar(32);not null;default:''"`
	IDDocumentCountry           string `json:"id_document_country" gorm:"type:varchar(8)"`
	Status                      string `json:"id_verification_status" gorm:"type:varchar(16);not null;default:'NOT_REQUIRED'"`
	VerifiedAt                  int64  `json:"id_verified_at" gorm:"type:bigint;not null;default:0"`
	VerifiedBy                  int    `json:"id_verified_by" gorm:"not null;default:0"`
}

func (D2SIdentityVerification) TableName() string { return "d2s_identity_verifications" }

type D2SAuditRetentionRun struct {
	ID            string `json:"id" gorm:"type:varchar(64);primaryKey"`
	OperatorID    int    `json:"operator_id" gorm:"not null;index"`
	OperatorEmail string `json:"operator_email" gorm:"type:varchar(254);not null"`
	SourceIP      string `json:"source_ip" gorm:"type:varchar(64)"`
	RetentionDays int    `json:"retention_days" gorm:"not null"`
	RangeStart    int64  `json:"range_start" gorm:"type:bigint;not null"`
	CutoffAt      int64  `json:"cutoff_at" gorm:"type:bigint;not null;index"`
	RecordCount   int64  `json:"record_count" gorm:"not null"`
	Result        string `json:"result" gorm:"type:varchar(16);not null;default:'SUCCESS';index"`
	DetailsJSON   string `json:"details" gorm:"type:text"`
	CreatedAt     int64  `json:"created_at" gorm:"type:bigint;not null;index"`
}

func (D2SAuditRetentionRun) TableName() string           { return "d2s_audit_retention_runs" }
func (D2SAuditRetentionRun) BeforeUpdate(*gorm.DB) error { return ErrD2SAppendOnlyRecord }
func (D2SAuditRetentionRun) BeforeDelete(*gorm.DB) error { return ErrD2SAppendOnlyRecord }

type D2SAdminBatchPreview struct {
	ID           string `json:"id" gorm:"type:varchar(64);primaryKey"`
	AdminID      int    `json:"admin_id" gorm:"not null;index"`
	SelectorJSON string `json:"-" gorm:"type:text;not null"`
	ActionJSON   string `json:"-" gorm:"type:text;not null"`
	UserIDsJSON  string `json:"-" gorm:"type:text;not null"`
	MatchCount   int64  `json:"match_count" gorm:"not null"`
	CreatedAt    int64  `json:"created_at" gorm:"type:bigint;not null"`
	ExpiresAt    int64  `json:"expires_at" gorm:"type:bigint;not null;index"`
	ConsumedAt   int64  `json:"consumed_at" gorm:"type:bigint;not null;default:0"`
}

func (D2SAdminBatchPreview) TableName() string { return "d2s_admin_batch_previews" }

type D2SActionLogInput struct {
	UserID            *int
	Email             string
	ActionType        string
	EventTime         int64
	SourceIP          string
	SourcePort        *int
	DestinationIP     *string
	DestinationPort   *int
	UserAgent         string
	DeviceFingerprint *string
	RequestPath       string
	ResultStatus      string
}

var d2sActionTypes = map[string]struct{}{
	D2SActionRegister: {}, D2SActionLogin: {}, D2SActionLogout: {}, D2SActionPurchase: {},
	D2SActionDownload: {}, D2SActionPasswordReset: {}, D2SActionProfileUpdate: {},
}

func truncateD2SText(value string, maxBytes int) string {
	value = strings.ToValidUTF8(value, "")
	if len(value) <= maxBytes {
		return value
	}
	for len(value) > maxBytes {
		_, size := utf8.DecodeLastRuneInString(value)
		if size == 0 {
			return ""
		}
		value = value[:len(value)-size]
	}
	return value
}

func d2sActionRecordHash(entry D2SUserActionLog) string {
	userID := ""
	if entry.UserID != nil {
		userID = fmt.Sprintf("%d", *entry.UserID)
	}
	payload := "d2s-user-action-v1\x00" + userID + "\x00" + entry.ActionType + "\x00" +
		fmt.Sprintf("%d", entry.EventTime) + "\x00" + strings.TrimSpace(entry.SourceIP) + "\x00" + entry.ResultStatus
	sum := sha256.Sum256([]byte(payload))
	return hex.EncodeToString(sum[:])
}

func RecordD2SUserAction(input D2SActionLogInput) error {
	return recordD2SUserAction(DB, input)
}

func recordD2SUserAction(tx *gorm.DB, input D2SActionLogInput) error {
	if tx == nil {
		return errors.New("audit database transaction is required")
	}
	if _, okay := d2sActionTypes[input.ActionType]; !okay {
		return errors.New("invalid user action type")
	}
	if input.ResultStatus != D2SActionResultSuccess && input.ResultStatus != D2SActionResultFailure && input.ResultStatus != D2SActionResultBlocked {
		return errors.New("invalid user action result")
	}
	if input.EventTime <= 0 {
		input.EventTime = time.Now().UTC().Unix()
	}
	entry := D2SUserActionLog{
		ID: uuid.NewString(), UserID: input.UserID, Email: truncateD2SText(NormalizeEmail(input.Email), 254),
		ActionType: input.ActionType, EventTime: input.EventTime, SourceIP: truncateD2SText(strings.TrimSpace(input.SourceIP), 64),
		SourcePort: validD2SPort(input.SourcePort), DestinationIP: truncateD2SPointerText(input.DestinationIP, 64),
		DestinationPort: validD2SPort(input.DestinationPort), UserAgent: truncateD2SText(input.UserAgent, 512),
		DeviceFingerprint: normalizeD2SFingerprint(input.DeviceFingerprint), RequestPath: truncateD2SText(input.RequestPath, 2048),
		ResultStatus: input.ResultStatus,
	}
	entry.RecordHash = d2sActionRecordHash(entry)
	return tx.Create(&entry).Error
}

func validD2SPort(port *int) *int {
	if port == nil || *port < 1 || *port > 65535 {
		return nil
	}
	value := *port
	return &value
}

func truncateD2SPointerText(value *string, maxBytes int) *string {
	if value == nil {
		return nil
	}
	truncated := truncateD2SText(*value, maxBytes)
	return &truncated
}

func normalizeD2SFingerprint(value *string) *string {
	if value == nil {
		return nil
	}
	fingerprint := strings.ToLower(strings.TrimSpace(*value))
	if len(fingerprint) != 64 {
		return nil
	}
	if _, err := hex.DecodeString(fingerprint); err != nil {
		return nil
	}
	return &fingerprint
}

func VerifyD2SUserActionLogHash(id string) (bool, error) {
	var entry D2SUserActionLog
	if err := DB.Where("id = ?", strings.TrimSpace(id)).First(&entry).Error; err != nil {
		return false, err
	}
	expected := d2sActionRecordHash(entry)
	if len(expected) != len(entry.RecordHash) {
		return false, nil
	}
	return subtle.ConstantTimeCompare([]byte(expected), []byte(entry.RecordHash)) == 1, nil
}

func RecordD2SRegistrationRecord(entry *D2SRegistrationRecord) error {
	return recordD2SRegistrationRecord(DB, entry)
}

func recordD2SRegistrationRecord(tx *gorm.DB, entry *D2SRegistrationRecord) error {
	if tx == nil || entry == nil || entry.UserID <= 0 || strings.TrimSpace(entry.Email) == "" {
		return errors.New("invalid registration record")
	}
	if entry.ID == "" {
		entry.ID = uuid.NewString()
	}
	entry.Email = truncateD2SText(NormalizeEmail(entry.Email), 254)
	entry.RegisterIP = truncateD2SText(strings.TrimSpace(entry.RegisterIP), 64)
	entry.RegisterUserAgent = truncateD2SText(entry.RegisterUserAgent, 512)
	if entry.RegisterTime <= 0 {
		entry.RegisterTime = time.Now().UTC().Unix()
	}
	return tx.Create(entry).Error
}

func RecordD2SAdminOperationAudit(entry *D2SAdminOperationAudit) error {
	if entry == nil || entry.OperatorID < 0 || strings.TrimSpace(entry.OperatorEmail) == "" || strings.TrimSpace(entry.OperationType) == "" {
		return errors.New("invalid admin audit record")
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
	return DB.Create(entry).Error
}

func ListD2SUserActionLogs(startTime, endTime int64, userID *int, email, sourceIP, actionType string, page, pageSize int) ([]D2SUserActionLog, int64, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 50
	}
	if pageSize > 500 {
		pageSize = 500
	}
	query := DB.Model(&D2SUserActionLog{})
	if startTime > 0 {
		query = query.Where("event_time >= ?", startTime)
	}
	if endTime > 0 {
		query = query.Where("event_time <= ?", endTime)
	}
	if userID != nil {
		query = query.Where("user_id = ?", *userID)
	}
	if normalized := NormalizeEmail(email); normalized != "" {
		query = query.Where("email = ?", normalized)
	}
	if sourceIP = strings.TrimSpace(sourceIP); sourceIP != "" {
		query = query.Where("source_ip = ?", sourceIP)
	}
	if actionType = strings.TrimSpace(actionType); actionType != "" {
		if _, okay := d2sActionTypes[actionType]; !okay {
			return nil, 0, errors.New("invalid action type")
		}
		query = query.Where("action_type = ?", actionType)
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var entries []D2SUserActionLog
	err := query.Order("event_time DESC").Order("id DESC").Offset((page - 1) * pageSize).Limit(pageSize).Find(&entries).Error
	return entries, total, err
}

func ListD2SRegistrationRecords(startTime, endTime int64, page, pageSize int) ([]D2SRegistrationRecord, int64, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 50
	}
	if pageSize > 500 {
		pageSize = 500
	}
	query := DB.Model(&D2SRegistrationRecord{})
	if startTime > 0 {
		query = query.Where("register_time >= ?", startTime)
	}
	if endTime > 0 {
		query = query.Where("register_time <= ?", endTime)
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var entries []D2SRegistrationRecord
	err := query.Order("register_time DESC").Order("id DESC").Offset((page - 1) * pageSize).Limit(pageSize).Find(&entries).Error
	return entries, total, err
}

func d2sRetentionCountAndMin(tx *gorm.DB, table any, column string, cutoff int64) (int64, int64, error) {
	var count int64
	if err := tx.Model(table).Where(column+" < ?", cutoff).Count(&count).Error; err != nil {
		return 0, 0, err
	}
	if count == 0 {
		return 0, 0, nil
	}
	var first int64
	if err := tx.Model(table).Select("MIN("+column+")").Where(column+" < ?", cutoff).Scan(&first).Error; err != nil {
		return 0, 0, err
	}
	return count, first, nil
}

// d2sAuditRetentionCutoffs keeps every audit table at least 366 days and
// user network logs through the longer of the requested period or six calendar months.
func d2sAuditRetentionCutoffs(retentionDays int, now int64) (int64, int64) {
	requestedCutoff := now - int64(retentionDays)*86400
	nowTime := time.Unix(now, 0).UTC()
	targetMonth := time.Date(nowTime.Year(), nowTime.Month()-6, 1, nowTime.Hour(), nowTime.Minute(), nowTime.Second(), 0, time.UTC)
	targetMonthLastDay := targetMonth.AddDate(0, 1, -1).Day()
	targetDay := nowTime.Day()
	if targetDay > targetMonthLastDay {
		targetDay = targetMonthLastDay
	}
	sixCalendarMonthsAgo := time.Date(targetMonth.Year(), targetMonth.Month(), targetDay, nowTime.Hour(), nowTime.Minute(), nowTime.Second(), 0, time.UTC).Unix()
	if sixCalendarMonthsAgo < requestedCutoff {
		return requestedCutoff, sixCalendarMonthsAgo
	}
	return requestedCutoff, requestedCutoff
}

// PruneD2SAuditData is explicit and is never scheduled by default.
func PruneD2SAuditData(retentionDays int, operatorID int, operatorEmail, sourceIP string, now int64) (*D2SAuditRetentionRun, error) {
	if retentionDays < D2SMinimumAuditRetentionDays || int64(retentionDays) > D2SMaximumAuditRetentionDays {
		return nil, ErrD2SAuditRetention
	}
	if operatorID <= 0 || strings.TrimSpace(operatorEmail) == "" {
		return nil, ErrD2SAdminInvalid
	}
	if now <= 0 {
		now = time.Now().UTC().Unix()
	}
	cleanupDB := D2SAuditCleanupDB
	if cleanupDB == nil && DB != nil && DB.Dialector.Name() == "sqlite" {
		cleanupDB = DB
	}
	if cleanupDB == nil {
		return nil, ErrD2SAuditCleanupUnavailable
	}

	requestedCutoff, userActionCutoff := d2sAuditRetentionCutoffs(retentionDays, now)
	tableCutoffs := map[string]int64{
		"d2s_user_action_logs":       userActionCutoff,
		"d2s_registration_records":   requestedCutoff,
		"d2s_admin_operation_audits": requestedCutoff,
	}
	run := &D2SAuditRetentionRun{ID: uuid.NewString(), OperatorID: operatorID, OperatorEmail: NormalizeEmail(operatorEmail), SourceIP: truncateD2SText(sourceIP, 64), RetentionDays: retentionDays, CutoffAt: userActionCutoff, CreatedAt: now, Result: "SUCCESS"}
	counts := make(map[string]int64, 3)
	tableRanges := make(map[string]map[string]int64, 3)
	stage := "count_records"
	err := cleanupDB.Transaction(func(tx *gorm.DB) error {
		var earliest int64
		tables := []struct {
			model  any
			column string
			name   string
			cutoff int64
		}{
			{&D2SUserActionLog{}, "event_time", "d2s_user_action_logs", userActionCutoff},
			{&D2SRegistrationRecord{}, "register_time", "d2s_registration_records", requestedCutoff},
			{&D2SAdminOperationAudit{}, "operation_time", "d2s_admin_operation_audits", requestedCutoff},
		}
		for _, table := range tables {
			count, first, err := d2sRetentionCountAndMin(tx, table.model, table.column, table.cutoff)
			if err != nil {
				return err
			}
			counts[table.name] = count
			if first == 0 {
				first = table.cutoff
			}
			tableRanges[table.name] = map[string]int64{"range_start": first, "range_end": table.cutoff, "record_count": count}
			run.RecordCount += count
			if count > 0 && (earliest == 0 || first < earliest) {
				earliest = first
			}
		}
		run.RangeStart = earliest
		if run.RangeStart == 0 {
			run.RangeStart = run.CutoffAt
		}
		details := map[string]any{
			"range_start": run.RangeStart, "range_end": run.CutoffAt, "record_count": run.RecordCount,
			"table_counts": counts, "table_cutoffs": tableCutoffs, "table_ranges": tableRanges,
			"requested_cutoff_at": requestedCutoff, "retention_days": retentionDays, "result": run.Result,
		}
		auditJSON, err := common.Marshal(details)
		if err != nil {
			return err
		}
		run.DetailsJSON = string(auditJSON)
		stage = "write_success_audit"
		if err := tx.Create(run).Error; err != nil {
			return err
		}
		for _, table := range tables {
			stage = "delete_" + table.name
			cleanupTx := tx.Session(&gorm.Session{NewDB: true}).Set("d2s_audit_retention_cleanup", true)
			if err := cleanupTx.Where(table.column+" < ?", table.cutoff).Delete(table.model).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		run.Result = "FAILURE"
		candidateCount := run.RecordCount
		run.RecordCount = 0
		if run.RangeStart == 0 {
			run.RangeStart = run.CutoffAt
		}
		failureDetails, marshalErr := common.Marshal(map[string]any{
			"range_start": run.RangeStart, "range_end": run.CutoffAt, "record_count": run.RecordCount,
			"candidate_record_count": candidateCount, "table_counts": counts, "table_cutoffs": tableCutoffs, "table_ranges": tableRanges,
			"requested_cutoff_at": requestedCutoff, "retention_days": retentionDays, "result": run.Result,
			"failed_stage": stage, "failure_code": "CLEANUP_TRANSACTION_FAILED",
		})
		if marshalErr == nil {
			run.DetailsJSON = string(failureDetails)
		}
		auditErr := cleanupDB.Transaction(func(tx *gorm.DB) error {
			return tx.Create(run).Error
		})
		if auditErr != nil {
			return nil, fmt.Errorf("%w", ErrD2SAuditCleanupFailureAudit)
		}
		return nil, ErrD2SAuditCleanupFailed
	}
	return run, nil
}
