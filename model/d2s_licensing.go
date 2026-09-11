package model

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	D2SProductDesktop2Stereo = "desktop2stereo"

	D2SLicenseKindTrial = "trial"
	D2SLicenseKindPaid  = "paid"

	D2SLicenseStatusActive    = "active"
	D2SLicenseStatusSuspended = "suspended"
	D2SLicenseStatusRevoked   = "revoked"

	D2SLicenseModeUnbound   = "unbound"
	D2SLicenseModeOnline    = "online"
	D2SLicenseModeOffline   = "offline"
	D2SLicenseModePermanent = "permanent"

	D2SDeviceCodePending  = "pending"
	D2SDeviceCodeApproved = "approved"
	D2SDeviceCodeClaiming = "claiming"
	D2SDeviceCodeConsumed = "consumed"
	D2SDeviceCodeCanceled = "canceled"
)

var (
	ErrD2SEmailVerificationRequired = errors.New("verified email is required")
	ErrD2SLicenseNotFound           = errors.New("license not found")
	ErrD2SLicenseUnavailable        = errors.New("license is unavailable")
	ErrD2SDeviceMismatch            = errors.New("device does not match the license binding")
	ErrD2SDeviceAlreadyBound        = errors.New("device is already bound to another license")
	ErrD2SLicenseAlreadyBound       = errors.New("license is already bound to another device")
	ErrD2SPermanentLocked           = errors.New("permanent license cannot be changed or revoked")
	ErrD2SRevokeCooldown            = errors.New("free revoke cooldown is active")
	ErrD2SLeaseConflict             = errors.New("license already has an active runtime lease")
	ErrD2SLeaseInvalid              = errors.New("runtime lease is invalid")
	ErrD2SDeviceCodeInvalid         = errors.New("device code is invalid")
	ErrD2SDeviceCodePending         = errors.New("authorization is pending")
	ErrD2SDeviceCodeExpired         = errors.New("device code expired")
	ErrD2SDeviceCodeConsumed        = errors.New("device code was already consumed")
	ErrD2SManualUnbindPending       = errors.New("manual unbind request is already pending")
	ErrD2SOfflinePeriodInvalid      = errors.New("offline authorization period is invalid")
)

const d2STransactionRetries = 5

func isD2STransientTransactionError(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	for _, marker := range []string{
		"database is locked",
		"database table is locked",
		"database is deadlocked",
		"deadlock found",
		"lock wait timeout",
		"deadlock detected",
		"could not serialize access",
		"serialization failure",
	} {
		if strings.Contains(message, marker) {
			return true
		}
	}
	return false
}

func d2STransaction(fn func(tx *gorm.DB) error) error {
	var lastErr error
	for attempt := 0; attempt < d2STransactionRetries; attempt++ {
		err := DB.Transaction(fn)
		lastErr = err
		if err == nil || !isD2STransientTransactionError(err) {
			return err
		}
		time.Sleep(time.Duration(attempt+1) * 20 * time.Millisecond)
	}
	return lastErr
}

type D2SUserProfile struct {
	UserID         int    `json:"user_id" gorm:"column:user_id;primaryKey"`
	Region         string `json:"region" gorm:"type:varchar(8);index"`
	RegionLockedAt int64  `json:"region_locked_at" gorm:"type:bigint;not null"`
	EmailVerified  bool   `json:"email_verified" gorm:"not null"`
	CreatedAt      int64  `json:"created_at" gorm:"type:bigint;not null"`
	UpdatedAt      int64  `json:"updated_at" gorm:"type:bigint;not null"`
}

func (D2SUserProfile) TableName() string { return "d2s_user_profiles" }

type D2SLicense struct {
	ID                 string `json:"id" gorm:"type:varchar(64);primaryKey"`
	LicenseCode        string `json:"license_code" gorm:"type:varchar(32);not null;uniqueIndex"`
	UserID             int    `json:"user_id" gorm:"not null;index:idx_d2s_licenses_user_created,priority:1;uniqueIndex:idx_d2s_license_identity,priority:1"`
	Product            string `json:"product" gorm:"type:varchar(64);not null"`
	Kind               string `json:"kind" gorm:"type:varchar(16);not null;index:idx_d2s_license_trial,priority:2;uniqueIndex:idx_d2s_license_identity,priority:2"`
	Status             string `json:"status" gorm:"type:varchar(24);not null;index"`
	Mode               string `json:"mode" gorm:"type:varchar(16);not null"`
	DeviceHash         string `json:"device_hash,omitempty" gorm:"type:varchar(64);index"`
	FingerprintVersion int    `json:"fingerprint_version" gorm:"not null"`
	OfflinePeriodDays  int    `json:"offline_period_days" gorm:"not null"`
	OfflineValidUntil  int64  `json:"offline_valid_until" gorm:"type:bigint;not null"`
	ActivatedAt        int64  `json:"activated_at" gorm:"type:bigint;not null"`
	ExpiresAt          int64  `json:"expires_at" gorm:"type:bigint;not null;index"`
	PermanentBoundAt   int64  `json:"permanent_bound_at" gorm:"type:bigint;not null"`
	SourceOrderID      string `json:"source_order_id,omitempty" gorm:"type:varchar(64);index;uniqueIndex:idx_d2s_license_identity,priority:3"`
	CreatedAt          int64  `json:"created_at" gorm:"type:bigint;not null;index:idx_d2s_licenses_user_created,priority:2"`
	UpdatedAt          int64  `json:"updated_at" gorm:"type:bigint;not null"`
}

func (D2SLicense) TableName() string { return "d2s_licenses" }

type D2SDeviceBinding struct {
	LicenseID          string `json:"license_id" gorm:"type:varchar(64);primaryKey"`
	UserID             int    `json:"user_id" gorm:"not null;index"`
	DeviceHash         string `json:"device_hash" gorm:"type:char(64);not null;uniqueIndex"`
	FingerprintVersion int    `json:"fingerprint_version" gorm:"not null"`
	BoundAt            int64  `json:"bound_at" gorm:"type:bigint;not null"`
	UpdatedAt          int64  `json:"updated_at" gorm:"type:bigint;not null"`
}

func (D2SDeviceBinding) TableName() string { return "d2s_device_bindings" }

type D2SLicenseEvent struct {
	ID         string `json:"id" gorm:"type:varchar(64);primaryKey"`
	LicenseID  string `json:"license_id" gorm:"type:varchar(64);not null;index:idx_d2s_license_events_license_created,priority:1"`
	UserID     int    `json:"user_id" gorm:"not null;index"`
	EventType  string `json:"event_type" gorm:"type:varchar(48);not null"`
	DeviceHash string `json:"device_hash,omitempty" gorm:"type:varchar(64)"`
	OrderID    string `json:"order_id,omitempty" gorm:"type:varchar(64);index"`
	Detail     string `json:"detail,omitempty" gorm:"type:text"`
	CreatedAt  int64  `json:"created_at" gorm:"type:bigint;not null;index:idx_d2s_license_events_license_created,priority:2"`
}

func (D2SLicenseEvent) TableName() string { return "d2s_license_events" }

type D2SDeviceCode struct {
	ID                 string `json:"id" gorm:"type:varchar(64);primaryKey"`
	DeviceCodeHash     string `json:"-" gorm:"type:char(64);not null;uniqueIndex"`
	UserCode           string `json:"user_code" gorm:"type:varchar(16);not null;uniqueIndex"`
	UserID             int    `json:"user_id" gorm:"not null;index"`
	DeviceHash         string `json:"device_hash" gorm:"type:char(64);not null;index"`
	FingerprintVersion int    `json:"fingerprint_version" gorm:"not null"`
	ClientName         string `json:"client_name" gorm:"type:varchar(128)"`
	Platform           string `json:"platform" gorm:"type:varchar(32)"`
	Status             string `json:"status" gorm:"type:varchar(16);not null;index"`
	CreatedAt          int64  `json:"created_at" gorm:"type:bigint;not null"`
	ExpiresAt          int64  `json:"expires_at" gorm:"type:bigint;not null;index"`
	ApprovedAt         int64  `json:"approved_at" gorm:"type:bigint;not null"`
	ConsumedAt         int64  `json:"consumed_at" gorm:"type:bigint;not null"`
}

func (D2SDeviceCode) TableName() string { return "d2s_device_codes" }

type D2SOfflineEntitlement struct {
	ID         string `json:"id" gorm:"type:varchar(64);primaryKey"`
	LicenseID  string `json:"license_id" gorm:"type:varchar(64);not null;index"`
	UserID     int    `json:"user_id" gorm:"not null;index"`
	DeviceHash string `json:"device_hash" gorm:"type:char(64);not null"`
	KeyID      string `json:"key_id" gorm:"type:varchar(64);not null"`
	IssuedAt   int64  `json:"issued_at" gorm:"type:bigint;not null"`
	ExpiresAt  int64  `json:"expires_at" gorm:"type:bigint;not null;index"`
	JWSHash    string `json:"-" gorm:"type:char(64);not null;uniqueIndex"`
}

func (D2SOfflineEntitlement) TableName() string { return "d2s_offline_entitlements" }

type D2SOnlineLease struct {
	LicenseID       string `json:"license_id" gorm:"type:varchar(64);primaryKey"`
	UserID          int    `json:"user_id" gorm:"not null;index"`
	DeviceHash      string `json:"device_hash" gorm:"type:char(64);not null"`
	LeaseTokenHash  string `json:"-" gorm:"type:char(64);not null;uniqueIndex"`
	CreatedAt       int64  `json:"created_at" gorm:"type:bigint;not null"`
	ExpiresAt       int64  `json:"expires_at" gorm:"type:bigint;not null;index"`
	LastHeartbeatAt int64  `json:"last_heartbeat_at" gorm:"type:bigint;not null"`
}

func (D2SOnlineLease) TableName() string { return "d2s_online_leases" }

type D2SFreeRevokeCooldown struct {
	LicenseID       string `json:"license_id" gorm:"type:varchar(64);primaryKey"`
	LastRevokedAt   int64  `json:"last_revoked_at" gorm:"type:bigint;not null"`
	NextAvailableAt int64  `json:"next_available_at" gorm:"type:bigint;not null;index"`
	PeriodDays      int    `json:"period_days" gorm:"not null"`
}

func (D2SFreeRevokeCooldown) TableName() string { return "d2s_free_revoke_cooldowns" }

type D2SPaidRevokeQuota struct {
	ID          string `json:"id" gorm:"type:varchar(96);primaryKey"`
	LicenseID   string `json:"license_id" gorm:"type:varchar(64);not null;index"`
	PeriodMonth string `json:"period_month" gorm:"type:char(7);not null"`
	UsedCount   int    `json:"used_count" gorm:"not null"`
	UpdatedAt   int64  `json:"updated_at" gorm:"type:bigint;not null"`
}

func (D2SPaidRevokeQuota) TableName() string { return "d2s_paid_revoke_quotas" }

type D2SManualUnbindRequest struct {
	ID         string `json:"id" gorm:"type:varchar(64);primaryKey"`
	LicenseID  string `json:"license_id" gorm:"type:varchar(64);not null;index"`
	UserID     int    `json:"user_id" gorm:"not null;index"`
	Reason     string `json:"reason" gorm:"type:text;not null"`
	ProofRef   string `json:"proof_ref" gorm:"type:varchar(255)"`
	Status     string `json:"status" gorm:"type:varchar(16);not null;index"`
	ReviewedBy int    `json:"reviewed_by" gorm:"not null"`
	ReviewNote string `json:"review_note" gorm:"type:text"`
	CreatedAt  int64  `json:"created_at" gorm:"type:bigint;not null"`
	UpdatedAt  int64  `json:"updated_at" gorm:"type:bigint;not null"`
}

func (D2SManualUnbindRequest) TableName() string { return "d2s_manual_unbind_requests" }

type D2SOrder struct {
	ID             string  `json:"id" gorm:"type:varchar(64);primaryKey"`
	UserID         int     `json:"user_id" gorm:"not null;index;uniqueIndex:idx_d2s_order_user_idempotency,priority:1"`
	Product        string  `json:"product" gorm:"type:varchar(32);not null;index"`
	LicenseID      string  `json:"license_id,omitempty" gorm:"type:varchar(64);index"`
	Provider       string  `json:"provider" gorm:"type:varchar(32);not null;index"`
	Region         string  `json:"region" gorm:"type:varchar(8);not null"`
	Currency       string  `json:"currency" gorm:"type:char(3);not null"`
	AmountMinor    int64   `json:"amount_minor" gorm:"type:bigint;not null"`
	BalanceMinor   int64   `json:"balance_minor" gorm:"type:bigint;not null"`
	GatewayMinor   int64   `json:"gateway_minor" gorm:"type:bigint;not null"`
	RevokeNumber   int     `json:"revoke_number,omitempty" gorm:"not null"`
	Status         string  `json:"status" gorm:"type:varchar(16);not null;index"`
	IdempotencyKey string  `json:"-" gorm:"type:varchar(128);not null;uniqueIndex:idx_d2s_order_user_idempotency,priority:2"`
	PendingKey     *string `json:"-" gorm:"type:varchar(64);uniqueIndex:idx_d2s_order_pending_key"`
	CreatedAt      int64   `json:"created_at" gorm:"type:bigint;not null"`
	ExpiresAt      int64   `json:"expires_at" gorm:"type:bigint;not null;index"`
	CompletedAt    int64   `json:"completed_at" gorm:"type:bigint;not null"`
}

func (D2SOrder) TableName() string { return "d2s_orders" }

type D2SPaymentEvent struct {
	ID              string `json:"id" gorm:"type:varchar(64);primaryKey"`
	Provider        string `json:"provider" gorm:"type:varchar(32);not null;uniqueIndex:idx_d2s_payment_provider_event,priority:1"`
	ProviderEventID string `json:"provider_event_id" gorm:"type:varchar(128);not null;uniqueIndex:idx_d2s_payment_provider_event,priority:2"`
	OrderID         string `json:"order_id" gorm:"type:varchar(64);not null;index"`
	EventType       string `json:"event_type" gorm:"type:varchar(32);not null"`
	AmountMinor     int64  `json:"amount_minor" gorm:"type:bigint;not null"`
	Currency        string `json:"currency" gorm:"type:char(3);not null"`
	PayloadHash     string `json:"payload_hash" gorm:"type:char(64);not null"`
	ProcessedAt     int64  `json:"processed_at" gorm:"type:bigint;not null"`
}

func (D2SPaymentEvent) TableName() string { return "d2s_payment_events" }

type D2SBalanceAccount struct {
	ID             string `json:"id" gorm:"type:varchar(64);primaryKey"`
	UserID         int    `json:"user_id" gorm:"not null;uniqueIndex:idx_d2s_balance_user_currency,priority:1"`
	Currency       string `json:"currency" gorm:"type:char(3);not null;uniqueIndex:idx_d2s_balance_user_currency,priority:2"`
	AvailableMinor int64  `json:"available_minor" gorm:"type:bigint;not null"`
	ReservedMinor  int64  `json:"reserved_minor" gorm:"type:bigint;not null"`
	UpdatedAt      int64  `json:"updated_at" gorm:"type:bigint;not null"`
}

func (D2SBalanceAccount) TableName() string { return "d2s_balance_accounts" }

type D2SBalanceTransaction struct {
	ID          string `json:"id" gorm:"type:varchar(64);primaryKey"`
	AccountID   string `json:"account_id" gorm:"type:varchar(64);not null;index"`
	UserID      int    `json:"user_id" gorm:"not null;index"`
	Currency    string `json:"currency" gorm:"type:char(3);not null"`
	Kind        string `json:"kind" gorm:"type:varchar(32);not null"`
	AmountMinor int64  `json:"amount_minor" gorm:"type:bigint;not null"`
	OrderID     string `json:"order_id,omitempty" gorm:"type:varchar(64);index"`
	ReferenceID string `json:"reference_id,omitempty" gorm:"type:varchar(128);index"`
	CreatedAt   int64  `json:"created_at" gorm:"type:bigint;not null"`
}

func (D2SBalanceTransaction) TableName() string { return "d2s_balance_transactions" }

type D2SInviteReward struct {
	ID            string `json:"id" gorm:"type:varchar(64);primaryKey"`
	InviterUserID int    `json:"inviter_user_id" gorm:"not null;index"`
	InviteeUserID int    `json:"invitee_user_id" gorm:"not null;uniqueIndex"`
	OrderID       string `json:"order_id" gorm:"type:varchar(64);not null;uniqueIndex"`
	Currency      string `json:"currency" gorm:"type:char(3);not null"`
	AmountMinor   int64  `json:"amount_minor" gorm:"type:bigint;not null"`
	Status        string `json:"status" gorm:"type:varchar(16);not null"`
	CreatedAt     int64  `json:"created_at" gorm:"type:bigint;not null"`
}

func (D2SInviteReward) TableName() string { return "d2s_invite_rewards" }

type D2SWithdrawalRequest struct {
	ID            string `json:"id" gorm:"type:varchar(64);primaryKey"`
	UserID        int    `json:"user_id" gorm:"not null;index"`
	Currency      string `json:"currency" gorm:"type:char(3);not null"`
	AmountMinor   int64  `json:"amount_minor" gorm:"type:bigint;not null"`
	AlipayAccount string `json:"alipay_account" gorm:"type:varchar(128);not null"`
	RealName      string `json:"real_name" gorm:"type:varchar(128);not null"`
	Status        string `json:"status" gorm:"type:varchar(16);not null;index"`
	ReviewedBy    int    `json:"reviewed_by" gorm:"not null"`
	ReviewNote    string `json:"review_note" gorm:"type:text"`
	CreatedAt     int64  `json:"created_at" gorm:"type:bigint;not null"`
	UpdatedAt     int64  `json:"updated_at" gorm:"type:bigint;not null"`
	PaidAt        int64  `json:"paid_at" gorm:"type:bigint;not null"`
}

func (D2SWithdrawalRequest) TableName() string { return "d2s_withdrawal_requests" }

type D2SSigningKey struct {
	KeyID     string `json:"key_id" gorm:"type:varchar(64);primaryKey"`
	Algorithm string `json:"algorithm" gorm:"type:varchar(16);not null"`
	PublicJWK string `json:"public_jwk" gorm:"type:text;not null"`
	Status    string `json:"status" gorm:"type:varchar(16);not null;index"`
	CreatedAt int64  `json:"created_at" gorm:"type:bigint;not null"`
	RetiredAt int64  `json:"retired_at" gorm:"type:bigint;not null"`
}

func (D2SSigningKey) TableName() string { return "d2s_signing_keys" }

func hashD2SSecret(secret string) string {
	digest := sha256.Sum256([]byte(secret))
	return hex.EncodeToString(digest[:])
}

func randomD2SSecret(byteCount int) (string, error) {
	value := make([]byte, byteCount)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	return hex.EncodeToString(value), nil
}

func newD2SLicenseCode() (string, error) {
	raw, err := randomD2SSecret(8)
	if err != nil {
		return "", err
	}
	return "D2S-" + strings.ToUpper(raw[:4]) + "-" + strings.ToUpper(raw[4:8]) + "-" + strings.ToUpper(raw[8:12]), nil
}

func EnsureD2SProfileAndTrial(userID int, now int64) (*D2SUserProfile, error) {
	if userID <= 0 {
		return nil, ErrD2SEmailVerificationRequired
	}
	if now <= 0 {
		now = time.Now().Unix()
	}
	var result D2SUserProfile
	err := d2STransaction(func(tx *gorm.DB) error {
		var user User
		if err := tx.Select("id", "email", "email_verified_at").First(&user, userID).Error; err != nil {
			return err
		}
		if strings.TrimSpace(user.Email) == "" || (common.EmailVerificationEnabled && user.EmailVerifiedAt <= 0) {
			return ErrD2SEmailVerificationRequired
		}
		if err := lockForUpdate(tx).Where("user_id = ?", userID).First(&result).Error; err != nil {
			if !errors.Is(err, gorm.ErrRecordNotFound) {
				return err
			}
			candidate := D2SUserProfile{UserID: userID, EmailVerified: !common.EmailVerificationEnabled || user.EmailVerifiedAt > 0, CreatedAt: now, UpdatedAt: now}
			if err := tx.Clauses(clause.OnConflict{
				Columns:   []clause.Column{{Name: "user_id"}},
				DoNothing: true,
			}).Create(&candidate).Error; err != nil {
				return err
			}
			if err := lockForUpdate(tx).Where("user_id = ?", userID).First(&result).Error; err != nil {
				return err
			}
		}
		expectedEmailVerified := !common.EmailVerificationEnabled || user.EmailVerifiedAt > 0
		if result.EmailVerified != expectedEmailVerified {
			if err := tx.Model(&D2SUserProfile{}).Where("user_id = ?", userID).Update("email_verified", expectedEmailVerified).Error; err != nil {
				return err
			}
			result.EmailVerified = expectedEmailVerified
		}
		var count int64
		if err := tx.Model(&D2SLicense{}).Where("user_id = ? AND kind = ?", userID, D2SLicenseKindTrial).Count(&count).Error; err != nil {
			return err
		}
		if count > 0 {
			return nil
		}
		code, err := newD2SLicenseCode()
		if err != nil {
			return err
		}
		trial := D2SLicense{
			ID: uuid.NewString(), LicenseCode: code, UserID: userID, Product: D2SProductDesktop2Stereo,
			Kind: D2SLicenseKindTrial, Status: D2SLicenseStatusActive, Mode: D2SLicenseModeUnbound,
			OfflinePeriodDays: 7, CreatedAt: now, UpdatedAt: now,
		}
		created := tx.Clauses(clause.OnConflict{
			Columns: []clause.Column{
				{Name: "user_id"}, {Name: "kind"}, {Name: "source_order_id"},
			},
			DoNothing: true,
		}).Create(&trial)
		if created.Error != nil {
			return created.Error
		}
		if created.RowsAffected == 0 {
			return nil
		}
		if err := tx.Create(&D2SLicenseEvent{
			ID: uuid.NewString(), LicenseID: trial.ID, UserID: userID, EventType: "trial_created", CreatedAt: now,
		}).Error; err != nil {
			return err
		}
		return nil
	})
	return &result, err
}

func ListD2SLicenses(userID int, now int64) ([]D2SLicense, error) {
	if _, err := EnsureD2SProfileAndTrial(userID, now); err != nil {
		return nil, err
	}
	var licenses []D2SLicense
	err := DB.Where("user_id = ?", userID).Order("created_at ASC").Find(&licenses).Error
	return licenses, err
}

func GetD2SLicense(userID int, licenseID string, tx *gorm.DB) (*D2SLicense, error) {
	if tx == nil {
		tx = DB
	}
	var license D2SLicense
	if err := tx.Where("id = ? AND user_id = ?", licenseID, userID).First(&license).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrD2SLicenseNotFound
		}
		return nil, err
	}
	return &license, nil
}

func validateD2SDeviceHash(deviceHash string) bool {
	if len(deviceHash) != 64 {
		return false
	}
	_, err := hex.DecodeString(deviceHash)
	return err == nil
}

func validateD2SOfflinePeriod(days int) bool {
	return days == 7 || days == 14 || days == 30
}

func BindD2SLicense(userID int, licenseID, deviceHash string, fingerprintVersion int, now int64) (*D2SLicense, error) {
	deviceHash = strings.ToLower(strings.TrimSpace(deviceHash))
	if !validateD2SDeviceHash(deviceHash) || fingerprintVersion <= 0 {
		return nil, ErrD2SDeviceMismatch
	}
	if now <= 0 {
		now = time.Now().Unix()
	}
	var result D2SLicense
	err := d2STransaction(func(tx *gorm.DB) error {
		license, err := GetD2SLicense(userID, licenseID, lockForUpdate(tx))
		if err != nil {
			return err
		}
		if license.Status != D2SLicenseStatusActive || (license.ExpiresAt > 0 && license.ExpiresAt <= now) {
			return ErrD2SLicenseUnavailable
		}
		if license.DeviceHash != "" && license.DeviceHash != deviceHash {
			return ErrD2SLicenseAlreadyBound
		}
		var existing D2SDeviceBinding
		if err := lockForUpdate(tx).Where("device_hash = ?", deviceHash).First(&existing).Error; err == nil && existing.LicenseID != license.ID {
			return ErrD2SDeviceAlreadyBound
		} else if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		binding := D2SDeviceBinding{
			LicenseID: license.ID, UserID: userID, DeviceHash: deviceHash,
			FingerprintVersion: fingerprintVersion, BoundAt: now, UpdatedAt: now,
		}
		if err := tx.Where("license_id = ?", license.ID).Assign(binding).FirstOrCreate(&binding).Error; err != nil {
			return err
		}
		mode := license.Mode
		if mode == D2SLicenseModeUnbound {
			mode = D2SLicenseModeOnline
		}
		activatedAt := license.ActivatedAt
		if activatedAt == 0 {
			activatedAt = now
		}
		updates := map[string]any{
			"device_hash": deviceHash, "fingerprint_version": fingerprintVersion, "mode": mode,
			"activated_at": activatedAt, "updated_at": now,
		}
		trialStarted := license.Kind == D2SLicenseKindTrial && license.ActivatedAt == 0 && license.ExpiresAt == 0
		if trialStarted {
			updates["expires_at"] = now + 30*86400
		}
		if err := tx.Model(&D2SLicense{}).Where("id = ?", license.ID).Updates(updates).Error; err != nil {
			return err
		}
		if trialStarted {
			if err := tx.Create(&D2SLicenseEvent{
				ID: uuid.NewString(), LicenseID: license.ID, UserID: userID, EventType: "trial_started", DeviceHash: deviceHash, CreatedAt: now,
			}).Error; err != nil {
				return err
			}
			license.ExpiresAt = now + 30*86400
		}
		if err := tx.Create(&D2SLicenseEvent{
			ID: uuid.NewString(), LicenseID: license.ID, UserID: userID, EventType: "device_bound", DeviceHash: deviceHash, CreatedAt: now,
		}).Error; err != nil {
			return err
		}
		license.DeviceHash, license.FingerprintVersion, license.Mode, license.ActivatedAt, license.UpdatedAt = deviceHash, fingerprintVersion, mode, activatedAt, now
		result = *license
		return nil
	})
	return &result, err
}

func CreateD2SManualUnbind(userID int, licenseID, reason, proofRef string, now int64) (*D2SManualUnbindRequest, error) {
	if strings.TrimSpace(reason) == "" {
		return nil, ErrD2SOrderInvalid
	}
	if now <= 0 {
		now = time.Now().Unix()
	}
	var result D2SManualUnbindRequest
	err := d2STransaction(func(tx *gorm.DB) error {
		license, err := GetD2SLicense(userID, licenseID, lockForUpdate(tx))
		if err != nil {
			return err
		}
		if license.Mode != D2SLicenseModePermanent {
			return ErrD2SLicenseUnavailable
		}
		var count int64
		if err := tx.Model(&D2SManualUnbindRequest{}).Where("user_id = ? AND status = ?", userID, "approved").Count(&count).Error; err != nil {
			return err
		}
		if count > 0 {
			return ErrD2SPermanentLocked
		}
		if err := tx.Model(&D2SManualUnbindRequest{}).Where("license_id = ? AND status = ?", licenseID, "pending").Count(&count).Error; err != nil {
			return err
		}
		if count > 0 {
			return ErrD2SManualUnbindPending
		}
		result = D2SManualUnbindRequest{
			ID: uuid.NewString(), LicenseID: licenseID, UserID: userID,
			Reason: strings.TrimSpace(reason), ProofRef: strings.TrimSpace(proofRef),
			Status: "pending", CreatedAt: now, UpdatedAt: now,
		}
		return tx.Create(&result).Error
	})
	return &result, err
}

func ChangeD2SLicenseMode(userID int, licenseID, deviceHash, nextMode, confirmation string, offlineDays int, now int64) (*D2SLicense, error) {
	if nextMode != D2SLicenseModeOnline && nextMode != D2SLicenseModeOffline && nextMode != D2SLicenseModePermanent {
		return nil, ErrD2SLicenseUnavailable
	}
	if now <= 0 {
		now = time.Now().Unix()
	}
	var result D2SLicense
	err := d2STransaction(func(tx *gorm.DB) error {
		license, err := GetD2SLicense(userID, licenseID, lockForUpdate(tx))
		if err != nil {
			return err
		}
		if license.Status != D2SLicenseStatusActive || (license.ExpiresAt > 0 && license.ExpiresAt <= now) {
			return ErrD2SLicenseUnavailable
		}
		if license.Mode == D2SLicenseModePermanent && nextMode != D2SLicenseModePermanent {
			return ErrD2SPermanentLocked
		}
		if license.DeviceHash == "" || license.DeviceHash != strings.ToLower(strings.TrimSpace(deviceHash)) {
			return ErrD2SDeviceMismatch
		}
		periodDays := offlineDays
		switch nextMode {
		case D2SLicenseModeOffline:
			if !validateD2SOfflinePeriod(periodDays) {
				return ErrD2SOfflinePeriodInvalid
			}
		case D2SLicenseModeOnline:
			periodDays = license.OfflinePeriodDays
			if !validateD2SOfflinePeriod(periodDays) {
				periodDays = 7
			}
		case D2SLicenseModePermanent:
			periodDays = 0
		}
		permanentAt := license.PermanentBoundAt
		if nextMode == D2SLicenseModePermanent {
			if confirmation != "PERMANENT" {
				return ErrD2SPermanentLocked
			}
			permanentAt = now
		}
		updates := map[string]any{"mode": nextMode, "offline_period_days": periodDays, "permanent_bound_at": permanentAt, "updated_at": now}
		if err := tx.Model(&D2SLicense{}).Where("id = ?", license.ID).Updates(updates).Error; err != nil {
			return err
		}
		if nextMode != D2SLicenseModeOnline {
			if err := tx.Where("license_id = ?", license.ID).Delete(&D2SOnlineLease{}).Error; err != nil {
				return err
			}
		}
		if err := tx.Create(&D2SLicenseEvent{
			ID: uuid.NewString(), LicenseID: license.ID, UserID: userID, EventType: "mode_changed", DeviceHash: license.DeviceHash,
			Detail: nextMode, CreatedAt: now,
		}).Error; err != nil {
			return err
		}
		license.Mode, license.OfflinePeriodDays, license.PermanentBoundAt, license.UpdatedAt = nextMode, periodDays, permanentAt, now
		result = *license
		return nil
	})
	return &result, err
}

func FreeRevokeD2SLicense(userID int, licenseID, deviceHash string, now int64) (int64, error) {
	if now <= 0 {
		now = time.Now().Unix()
	}
	nextAvailableAt := int64(0)
	err := d2STransaction(func(tx *gorm.DB) error {
		license, err := GetD2SLicense(userID, licenseID, lockForUpdate(tx))
		if err != nil {
			return err
		}
		if license.Mode == D2SLicenseModePermanent {
			return ErrD2SPermanentLocked
		}
		if license.Status != D2SLicenseStatusActive || license.DeviceHash == "" || license.DeviceHash != strings.ToLower(strings.TrimSpace(deviceHash)) {
			return ErrD2SDeviceMismatch
		}
		var cooldown D2SFreeRevokeCooldown
		if err := lockForUpdate(tx).Where("license_id = ?", license.ID).First(&cooldown).Error; err == nil && cooldown.NextAvailableAt > now {
			return ErrD2SRevokeCooldown
		} else if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		period := license.OfflinePeriodDays
		if period != 7 && period != 14 && period != 30 {
			period = 7
		}
		nextAvailableAt = now + int64(period)*86400
		cooldown = D2SFreeRevokeCooldown{LicenseID: license.ID, LastRevokedAt: now, NextAvailableAt: nextAvailableAt, PeriodDays: period}
		if err := tx.Where("license_id = ?", license.ID).Assign(cooldown).FirstOrCreate(&cooldown).Error; err != nil {
			return err
		}
		if err := tx.Where("license_id = ?", license.ID).Delete(&D2SDeviceBinding{}).Error; err != nil {
			return err
		}
		if err := tx.Where("license_id = ?", license.ID).Delete(&D2SOnlineLease{}).Error; err != nil {
			return err
		}
		if err := tx.Model(&D2SLicense{}).Where("id = ?", license.ID).Updates(map[string]any{
			"device_hash": "", "fingerprint_version": 0, "mode": D2SLicenseModeUnbound, "updated_at": now,
		}).Error; err != nil {
			return err
		}
		return tx.Create(&D2SLicenseEvent{
			ID: uuid.NewString(), LicenseID: license.ID, UserID: userID, EventType: "free_revoke", DeviceHash: deviceHash, CreatedAt: now,
		}).Error
	})
	return nextAvailableAt, err
}

const (
	D2SOnlineLeaseTTL          = 2 * time.Hour
	D2SOnlineHeartbeatInterval = 15 * time.Minute
)

func StartOrRenewD2SOnlineLease(userID int, licenseID, deviceHash, rawLeaseToken string, now int64) (string, int64, error) {
	if now <= 0 {
		now = time.Now().Unix()
	}
	expiresAt := now + int64(D2SOnlineLeaseTTL/time.Second)
	issuedToken := rawLeaseToken
	err := d2STransaction(func(tx *gorm.DB) error {
		license, err := GetD2SLicense(userID, licenseID, lockForUpdate(tx))
		if err != nil {
			return err
		}
		if license.Status != D2SLicenseStatusActive || license.Mode != D2SLicenseModeOnline || license.DeviceHash != strings.ToLower(strings.TrimSpace(deviceHash)) || (license.ExpiresAt > 0 && license.ExpiresAt <= now) {
			return ErrD2SLicenseUnavailable
		}
		var lease D2SOnlineLease
		err = lockForUpdate(tx).Where("license_id = ?", license.ID).First(&lease).Error
		if err == nil && lease.ExpiresAt > now {
			if rawLeaseToken == "" || lease.LeaseTokenHash != hashD2SSecret(rawLeaseToken) || lease.DeviceHash != license.DeviceHash {
				return ErrD2SLeaseConflict
			}
			return tx.Model(&D2SOnlineLease{}).Where("license_id = ?", license.ID).Updates(map[string]any{
				"expires_at": expiresAt, "last_heartbeat_at": now,
			}).Error
		}
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if issuedToken == "" {
			issuedToken, err = randomD2SSecret(32)
			if err != nil {
				return err
			}
		}
		lease = D2SOnlineLease{
			LicenseID: license.ID, UserID: userID, DeviceHash: license.DeviceHash,
			LeaseTokenHash: hashD2SSecret(issuedToken), CreatedAt: now, ExpiresAt: expiresAt, LastHeartbeatAt: now,
		}
		return tx.Where("license_id = ?", license.ID).Assign(lease).FirstOrCreate(&lease).Error
	})
	return issuedToken, expiresAt, err
}

func ReleaseD2SOnlineLease(userID int, licenseID, rawLeaseToken string) error {
	result := DB.Where("license_id = ? AND user_id = ? AND lease_token_hash = ?", licenseID, userID, hashD2SSecret(rawLeaseToken)).Delete(&D2SOnlineLease{})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return ErrD2SLeaseInvalid
	}
	return nil
}

func ReleaseAllD2SOnlineLeases(userID int) error {
	if userID <= 0 {
		return nil
	}
	return DB.Where("user_id = ?", userID).Delete(&D2SOnlineLease{}).Error
}

func DeleteExpiredD2SRuntimeArtifacts(now int64) error {
	if now <= 0 {
		now = time.Now().Unix()
	}
	return d2STransaction(func(tx *gorm.DB) error {
		if err := tx.Where("expires_at <= ?", now).Delete(&D2SOnlineLease{}).Error; err != nil {
			return err
		}
		return tx.Where("expires_at <= ?", now).Delete(&D2SDeviceCode{}).Error
	})
}

func CreateD2SDeviceCode(deviceHash string, fingerprintVersion int, clientName, platform string, now int64) (*D2SDeviceCode, string, error) {
	deviceHash = strings.ToLower(strings.TrimSpace(deviceHash))
	clientName = strings.TrimSpace(clientName)
	platform = strings.TrimSpace(platform)
	if !validateD2SDeviceHash(deviceHash) || fingerprintVersion <= 0 || len(clientName) > 128 || len(platform) > 32 {
		return nil, "", ErrD2SDeviceCodeInvalid
	}
	if now <= 0 {
		now = time.Now().Unix()
	}
	deviceSecret, err := randomD2SSecret(32)
	if err != nil {
		return nil, "", err
	}
	userSecret, err := randomD2SSecret(5)
	if err != nil {
		return nil, "", err
	}
	userCode := strings.ToUpper(userSecret[:4] + "-" + userSecret[4:8])
	row := &D2SDeviceCode{
		ID: uuid.NewString(), DeviceCodeHash: hashD2SSecret(deviceSecret), UserCode: userCode,
		DeviceHash: deviceHash, FingerprintVersion: fingerprintVersion,
		ClientName: clientName, Platform: platform,
		Status: D2SDeviceCodePending, CreatedAt: now, ExpiresAt: now + 600,
	}
	return row, deviceSecret, DB.Create(row).Error
}

func ApproveD2SDeviceCode(userID int, userCode string, now int64) (*D2SDeviceCode, error) {
	if now <= 0 {
		now = time.Now().Unix()
	}
	if _, err := EnsureD2SProfileAndTrial(userID, now); err != nil {
		return nil, err
	}
	var row D2SDeviceCode
	err := d2STransaction(func(tx *gorm.DB) error {
		if err := lockForUpdate(tx).Where("user_code = ?", strings.ToUpper(strings.TrimSpace(userCode))).First(&row).Error; err != nil {
			return ErrD2SDeviceCodeInvalid
		}
		if row.ExpiresAt <= now {
			return ErrD2SDeviceCodeExpired
		}
		if row.Status != D2SDeviceCodePending {
			return ErrD2SDeviceCodeConsumed
		}
		row.UserID, row.Status, row.ApprovedAt = userID, D2SDeviceCodeApproved, now
		return tx.Model(&D2SDeviceCode{}).Where("id = ? AND status = ?", row.ID, D2SDeviceCodePending).Updates(map[string]any{
			"user_id": userID, "status": D2SDeviceCodeApproved, "approved_at": now,
		}).Error
	})
	return &row, err
}

func ClaimD2SDeviceCode(deviceSecret string, now int64) (*D2SDeviceCode, error) {
	if now <= 0 {
		now = time.Now().Unix()
	}
	var row D2SDeviceCode
	err := d2STransaction(func(tx *gorm.DB) error {
		if err := lockForUpdate(tx).Where("device_code_hash = ?", hashD2SSecret(deviceSecret)).First(&row).Error; err != nil {
			return ErrD2SDeviceCodeInvalid
		}
		if row.ExpiresAt <= now {
			return ErrD2SDeviceCodeExpired
		}
		switch row.Status {
		case D2SDeviceCodePending:
			return ErrD2SDeviceCodePending
		case D2SDeviceCodeConsumed, D2SDeviceCodeCanceled, D2SDeviceCodeClaiming:
			return ErrD2SDeviceCodeConsumed
		case D2SDeviceCodeApproved:
			result := tx.Model(&D2SDeviceCode{}).Where("id = ? AND status = ?", row.ID, D2SDeviceCodeApproved).Update("status", D2SDeviceCodeClaiming)
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected != 1 {
				return ErrD2SDeviceCodeConsumed
			}
			row.Status = D2SDeviceCodeClaiming
			return nil
		default:
			return ErrD2SDeviceCodeInvalid
		}
	})
	return &row, err
}

func FinishD2SDeviceCodeClaim(id string, success bool, now int64) error {
	status := D2SDeviceCodeApproved
	updates := map[string]any{"status": status}
	if success {
		status = D2SDeviceCodeConsumed
		updates = map[string]any{"status": status, "consumed_at": now}
	}
	return DB.Model(&D2SDeviceCode{}).Where("id = ? AND status = ?", id, D2SDeviceCodeClaiming).Updates(updates).Error
}

func CancelD2SDeviceCode(deviceSecret string, now int64) error {
	result := DB.Model(&D2SDeviceCode{}).Where("device_code_hash = ? AND status = ? AND expires_at > ?", hashD2SSecret(deviceSecret), D2SDeviceCodePending, now).Update("status", D2SDeviceCodeCanceled)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return ErrD2SDeviceCodeInvalid
	}
	return nil
}
