package model

import (
	"errors"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var ErrD2SAdminExists = errors.New("initial admin account already exists")

func CreateInitialD2SAdminAccount(email, passwordHash string, now int64) (*D2SAdminAccount, error) {
	email = NormalizeEmail(email)
	if email == "" || passwordHash == "" {
		return nil, ErrD2SAdminInvalid
	}
	if now <= 0 {
		now = time.Now().UTC().Unix()
	}
	account := &D2SAdminAccount{
		Email: email, PasswordHash: passwordHash, Role: D2SAdminRoleSuperAdmin,
		Status: D2SAdminStatusActive, CreatedAt: now, UpdatedAt: now,
	}
	err := DB.Transaction(func(tx *gorm.DB) error {
		var count int64
		if err := tx.Model(&D2SAdminAccount{}).Count(&count).Error; err != nil {
			return err
		}
		if count != 0 {
			return ErrD2SAdminExists
		}
		if err := tx.Create(account).Error; err != nil {
			return err
		}
		audit := D2SAdminOperationAudit{
			ID: uuid.NewString(), OperatorID: account.ID, OperatorEmail: account.Email,
			OperationType: "CREATE_BOOTSTRAP_ADMIN", OperationTime: now,
			TargetResource: "d2s_admin_accounts", Result: "SUCCESS",
			DetailsJSON: "{\"source\":\"one_time_environment_bootstrap\"}",
		}
		return tx.Create(&audit).Error
	})
	return account, err
}

func GetD2SAdminAccountByEmail(email string) (*D2SAdminAccount, error) {
	var account D2SAdminAccount
	err := DB.Where("email = ?", NormalizeEmail(email)).First(&account).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrD2SAdminInvalid
	}
	if err != nil {
		return nil, err
	}
	return &account, nil
}

func GetD2SAdminAccountByID(id int) (*D2SAdminAccount, error) {
	var account D2SAdminAccount
	err := DB.Where("id = ?", id).First(&account).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrD2SAdminInvalid
	}
	if err != nil {
		return nil, err
	}
	return &account, nil
}

func RecordD2SAdminLoginFailure(email string, now int64, maxFailures int, windowSeconds, lockSeconds int64) (*D2SAdminAccount, bool, error) {
	if now <= 0 {
		now = time.Now().UTC().Unix()
	}
	if maxFailures < 1 {
		maxFailures = 5
	}
	if windowSeconds < 1 {
		windowSeconds = 900
	}
	if lockSeconds < 1 {
		lockSeconds = 900
	}
	email = NormalizeEmail(email)
	var account D2SAdminAccount
	lockedNow := false
	err := DB.Transaction(func(tx *gorm.DB) error {
		if err := lockForUpdate(tx).Where("email = ?", email).First(&account).Error; err != nil {
			return err
		}
		if account.Status != D2SAdminStatusActive {
			return nil
		}
		if account.LockedUntil > now {
			lockedNow = true
			return nil
		}
		if account.LockedUntil > 0 {
			account.FailedAttempts = 0
			account.FailureWindowStartedAt = 0
			account.LockedUntil = 0
		}
		if account.FailureWindowStartedAt <= 0 || now < account.FailureWindowStartedAt || now-account.FailureWindowStartedAt >= windowSeconds {
			account.FailedAttempts = 0
			account.FailureWindowStartedAt = now
		}
		account.FailedAttempts++
		if account.FailedAttempts >= maxFailures {
			account.LockedUntil = now + lockSeconds
			lockedNow = true
		}
		account.UpdatedAt = now
		return tx.Model(&D2SAdminAccount{}).Where("id = ?", account.ID).Updates(map[string]any{
			"failed_attempts":           account.FailedAttempts,
			"failure_window_started_at": account.FailureWindowStartedAt,
			"locked_until":              account.LockedUntil,
			"updated_at":                now,
		}).Error
	})
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, false, ErrD2SAdminInvalid
	}
	if err != nil {
		return nil, false, err
	}
	return &account, lockedNow, nil
}

func RecordD2SAdminLoginSuccess(adminID int, now int64) error {
	if now <= 0 {
		now = time.Now().UTC().Unix()
	}
	return DB.Model(&D2SAdminAccount{}).Where("id = ? AND status = ?", adminID, D2SAdminStatusActive).Updates(map[string]any{
		"failed_attempts":           0,
		"failure_window_started_at": 0,
		"locked_until":              0,
		"last_login_at":             now,
		"updated_at":                now,
	}).Error
}

func CreateD2SAdminSession(session *D2SAdminSession) error {
	if session == nil || session.AdminID <= 0 || strings.TrimSpace(session.TokenHash) == "" || session.ExpiresAt <= session.CreatedAt {
		return ErrD2SAdminInvalid
	}
	return DB.Create(session).Error
}

func GetActiveD2SAdminSession(tokenHash string, now int64) (*D2SAdminSession, *D2SAdminAccount, error) {
	if now <= 0 {
		now = time.Now().UTC().Unix()
	}
	var session D2SAdminSession
	err := DB.Where("token_hash = ? AND revoked_at = 0 AND expires_at > ?", strings.TrimSpace(tokenHash), now).First(&session).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil, ErrD2SAdminInvalid
	}
	if err != nil {
		return nil, nil, err
	}
	account, err := GetD2SAdminAccountByID(session.AdminID)
	if err != nil {
		return nil, nil, err
	}
	if account.Status != D2SAdminStatusActive || account.LockedUntil > now {
		return nil, nil, ErrD2SAdminInvalid
	}
	return &session, account, nil
}

func RevokeD2SAdminSession(tokenHash string, now int64) error {
	if now <= 0 {
		now = time.Now().UTC().Unix()
	}
	return DB.Model(&D2SAdminSession{}).
		Where("token_hash = ? AND revoked_at = 0", strings.TrimSpace(tokenHash)).
		Update("revoked_at", now).Error
}

func RevokeAllD2SAdminSessions(adminID int, now int64) error {
	if adminID <= 0 {
		return ErrD2SAdminInvalid
	}
	if now <= 0 {
		now = time.Now().UTC().Unix()
	}
	return DB.Model(&D2SAdminSession{}).Where("admin_id = ? AND revoked_at = 0", adminID).Update("revoked_at", now).Error
}

func GetD2SUserSecurityControl(userID int) (*D2SUserSecurityControl, error) {
	if userID <= 0 {
		return nil, errors.New("invalid user id")
	}
	var control D2SUserSecurityControl
	err := DB.Where("user_id = ?", userID).First(&control).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return &D2SUserSecurityControl{UserID: userID}, nil
	}
	if err != nil {
		return nil, err
	}
	return &control, nil
}

func SetD2SUserSecurityControl(control *D2SUserSecurityControl) error {
	if control == nil || control.UserID <= 0 {
		return errors.New("invalid account security control")
	}
	now := time.Now().UTC().Unix()
	if control.UpdatedAt <= 0 {
		control.UpdatedAt = now
	}
	control.Reason = truncateD2SText(control.Reason, 512)
	return DB.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "user_id"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"login_blocked", "purchase_blocked", "reason", "updated_at", "updated_by",
		}),
	}).Create(control).Error
}

func SetD2SUserSecurityControlWithAudit(control *D2SUserSecurityControl, audit *D2SAdminOperationAudit) error {
	if control == nil || audit == nil || control.UserID <= 0 || audit.OperatorID <= 0 || strings.TrimSpace(audit.OperatorEmail) == "" {
		return ErrD2SAdminInvalid
	}
	now := time.Now().UTC().Unix()
	if control.UpdatedAt <= 0 {
		control.UpdatedAt = now
	}
	if audit.ID == "" {
		audit.ID = uuid.NewString()
	}
	if audit.OperationTime <= 0 {
		audit.OperationTime = now
	}
	audit.OperatorEmail = NormalizeEmail(audit.OperatorEmail)
	audit.SourceIP = truncateD2SText(audit.SourceIP, 64)
	audit.TargetResource = truncateD2SText(audit.TargetResource, 255)
	audit.DetailsJSON = truncateD2SText(audit.DetailsJSON, 8192)
	return DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "user_id"}},
			DoUpdates: clause.AssignmentColumns([]string{
				"login_blocked", "purchase_blocked", "reason", "updated_at", "updated_by",
			}),
		}).Create(control).Error; err != nil {
			return err
		}
		return tx.Create(audit).Error
	})
}

func IsD2SUserLoginBlocked(userID int) (bool, error) {
	control, err := GetD2SUserSecurityControl(userID)
	return control != nil && control.LoginBlocked, err
}

func IsD2SUserPurchaseBlocked(userID int) (bool, error) {
	control, err := GetD2SUserSecurityControl(userID)
	return control != nil && control.PurchaseBlocked, err
}

func CountD2SAdminAccounts() (int64, error) {
	var count int64
	err := DB.Model(&D2SAdminAccount{}).Count(&count).Error
	return count, err
}

func VerifyD2SAdminPassword(password, passwordHash string) bool {
	return common.ValidatePasswordAndHash(password, passwordHash)
}
