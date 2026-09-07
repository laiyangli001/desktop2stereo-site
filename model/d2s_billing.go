package model

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	D2SRegionCN   = "CN"
	D2SRegionINTL = "INTL"

	D2SOrderProductLicense          = "license"
	D2SOrderProductPaidRevoke       = "paid_revoke"
	D2SOrderProductOfflineExtension = "offline_extension"
	D2SProviderBalance              = "balance"

	D2SOrderPending    = "pending"
	D2SOrderPaid       = "paid"
	D2SOrderCanceled   = "canceled"
	D2SOrderChargeback = "chargeback"
)

var (
	ErrD2SRegionMismatch       = errors.New("payment provider does not match the account region")
	ErrD2SPriceNotConfigured   = errors.New("product price is not configured")
	ErrD2SOrderInvalid         = errors.New("order is invalid")
	ErrD2SOrderNotFound        = errors.New("order not found")
	ErrD2SOrderState           = errors.New("order state does not allow this operation")
	ErrD2SPaidRevokeLimit      = errors.New("monthly paid revoke limit reached")
	ErrD2SPaidRevokePending    = errors.New("another paid revoke order is pending")
	ErrD2SInsufficientBalance  = errors.New("insufficient balance")
	ErrD2SWithdrawalNotAllowed = errors.New("withdrawal is not allowed")
	ErrD2SPaymentMismatch      = errors.New("payment amount, currency, or provider mismatch")
	ErrD2SCheckoutUnavailable  = errors.New("payment checkout is unavailable")
)

type D2SOrderQuote struct {
	Product      string `json:"product"`
	LicenseID    string `json:"license_id,omitempty"`
	Provider     string `json:"provider"`
	Region       string `json:"region"`
	Currency     string `json:"currency"`
	AmountMinor  int64  `json:"amount_minor"`
	RevokeNumber int    `json:"revoke_number,omitempty"`
}

func D2SProviderRegion(provider string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(provider)) {
	case "epay", "paymentfm", "alipay", "wechat", "waffo":
		return D2SRegionCN, true
	case "stripe", "creem", "waffo_pancake":
		return D2SRegionINTL, true
	default:
		return "", false
	}
}

func d2sCurrencyForRegion(region string) string {
	if region == D2SRegionCN {
		return "CNY"
	}
	return "USD"
}

func d2sConfiguredPrice(envName string) (int64, error) {
	raw := strings.TrimSpace(os.Getenv(envName))
	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || value <= 0 {
		return 0, ErrD2SPriceNotConfigured
	}
	return value, nil
}

func QuoteD2SOrder(userID int, product, licenseID, provider string, now int64) (*D2SOrderQuote, error) {
	if now <= 0 {
		now = time.Now().Unix()
	}
	profile, err := EnsureD2SProfileAndTrial(userID, now)
	if err != nil {
		return nil, err
	}
	provider = strings.ToLower(strings.TrimSpace(provider))
	region, ok := D2SProviderRegion(provider)
	if provider == D2SProviderBalance {
		if profile.Region == "" {
			return nil, ErrD2SRegionMismatch
		}
		region, ok = profile.Region, true
	}
	if !ok {
		return nil, ErrD2SOrderInvalid
	}
	if profile.Region != "" && profile.Region != region {
		return nil, ErrD2SRegionMismatch
	}
	quote := &D2SOrderQuote{
		Product: product, LicenseID: licenseID, Provider: provider, Region: region, Currency: d2sCurrencyForRegion(region),
	}
	switch product {
	case D2SOrderProductLicense:
		if licenseID != "" {
			return nil, ErrD2SOrderInvalid
		}
		if region == D2SRegionCN {
			quote.AmountMinor = 9900
		} else {
			quote.AmountMinor = 2990
		}
	case D2SOrderProductPaidRevoke:
		license, err := GetD2SLicense(userID, licenseID, nil)
		if err != nil {
			return nil, err
		}
		if license.Status != D2SLicenseStatusActive || (license.ExpiresAt > 0 && license.ExpiresAt <= now) {
			return nil, ErrD2SLicenseUnavailable
		}
		if license.Mode == D2SLicenseModePermanent || license.DeviceHash == "" {
			return nil, ErrD2SPermanentLocked
		}
		month := time.Unix(now, 0).UTC().Format("2006-01")
		var quota D2SPaidRevokeQuota
		if err := DB.Where("id = ?", license.ID+":"+month).First(&quota).Error; err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, err
		}
		quote.RevokeNumber = quota.UsedCount + 1
		if quote.RevokeNumber > 3 {
			return nil, ErrD2SPaidRevokeLimit
		}
		cnPrices := []int64{1990, 3490, 6990}
		intlPrices := []int64{299, 499, 999}
		if region == D2SRegionCN {
			quote.AmountMinor = cnPrices[quote.RevokeNumber-1]
		} else {
			quote.AmountMinor = intlPrices[quote.RevokeNumber-1]
		}
	case D2SOrderProductOfflineExtension:
		license, err := GetD2SLicense(userID, licenseID, nil)
		if err != nil {
			return nil, err
		}
		if license.Status != D2SLicenseStatusActive || (license.ExpiresAt > 0 && license.ExpiresAt <= now) || license.Mode == D2SLicenseModePermanent {
			return nil, ErrD2SLicenseUnavailable
		}
		if region == D2SRegionCN {
			quote.AmountMinor, err = d2sConfiguredPrice("D2S_OFFLINE_EXTENSION_CNY_MINOR")
		} else {
			quote.AmountMinor, err = d2sConfiguredPrice("D2S_OFFLINE_EXTENSION_USD_MINOR")
		}
		if err != nil {
			return nil, err
		}
	default:
		return nil, ErrD2SOrderInvalid
	}
	return quote, nil
}

func d2sBalanceAccountTx(tx *gorm.DB, userID int, currency string, now int64) (*D2SBalanceAccount, error) {
	var account D2SBalanceAccount
	err := lockForUpdate(tx).Where("user_id = ? AND currency = ?", userID, currency).First(&account).Error
	if err == nil {
		return &account, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	account = D2SBalanceAccount{ID: uuid.NewString(), UserID: userID, Currency: currency, UpdatedAt: now}
	if err := tx.Create(&account).Error; err != nil {
		return nil, err
	}
	return &account, nil
}

func CreateD2SOrder(userID int, quote *D2SOrderQuote, balanceMinor int64, idempotencyKey string, now int64) (*D2SOrder, error) {
	if quote == nil || strings.TrimSpace(idempotencyKey) == "" || len(idempotencyKey) > 128 || balanceMinor < 0 || balanceMinor > quote.AmountMinor {
		return nil, ErrD2SOrderInvalid
	}
	if quote.Provider == D2SProviderBalance && balanceMinor != quote.AmountMinor {
		return nil, ErrD2SOrderInvalid
	}
	if now <= 0 {
		now = time.Now().Unix()
	}
	verified, err := QuoteD2SOrder(userID, quote.Product, quote.LicenseID, quote.Provider, now)
	if err != nil {
		return nil, err
	}
	if verified.Product != quote.Product || verified.LicenseID != quote.LicenseID || verified.Provider != quote.Provider || verified.AmountMinor != quote.AmountMinor || verified.Currency != quote.Currency || verified.Region != quote.Region || verified.RevokeNumber != quote.RevokeNumber {
		return nil, ErrD2SOrderInvalid
	}
	var order D2SOrder
	err = d2STransaction(func(tx *gorm.DB) error {
		if balanceMinor == quote.AmountMinor {
			var profile D2SUserProfile
			if err := lockForUpdate(tx).Where("user_id = ?", userID).First(&profile).Error; err != nil {
				return err
			}
			if profile.Region == "" || profile.Region != quote.Region {
				return ErrD2SRegionMismatch
			}
		}
		order = D2SOrder{
			ID: uuid.NewString(), UserID: userID, Product: quote.Product, LicenseID: quote.LicenseID,
			Provider: quote.Provider, Region: quote.Region, Currency: quote.Currency, AmountMinor: quote.AmountMinor,
			BalanceMinor: balanceMinor, GatewayMinor: quote.AmountMinor - balanceMinor, RevokeNumber: quote.RevokeNumber,
			Status: D2SOrderPending, IdempotencyKey: idempotencyKey, CreatedAt: now, ExpiresAt: now + 1800,
		}
		created := tx.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "user_id"}, {Name: "idempotency_key"}},
			DoNothing: true,
		}).Create(&order)
		if created.Error != nil {
			return created.Error
		}
		if created.RowsAffected == 0 {
			order = D2SOrder{}
			if err := tx.Where("user_id = ? AND idempotency_key = ?", userID, idempotencyKey).First(&order).Error; err != nil {
				return err
			}
			if order.Product != quote.Product || order.LicenseID != quote.LicenseID ||
				order.Provider != quote.Provider || order.Region != quote.Region ||
				order.Currency != quote.Currency || order.AmountMinor != quote.AmountMinor ||
				order.BalanceMinor != balanceMinor || order.RevokeNumber != quote.RevokeNumber {
				return ErrD2SOrderInvalid
			}
			return nil
		}
		if quote.Product == D2SOrderProductPaidRevoke {
			// Serialize paid-revoke quote consumption per license. Without this
			// lock, concurrent orders can observe the same monthly counter and
			// both pass the pending-order check.
			if _, err := GetD2SLicense(userID, quote.LicenseID, lockForUpdate(tx)); err != nil {
				return err
			}
			var pending int64
			if err := tx.Model(&D2SOrder{}).Where("license_id = ? AND product = ? AND status = ? AND id <> ?", quote.LicenseID, D2SOrderProductPaidRevoke, D2SOrderPending, order.ID).Count(&pending).Error; err != nil {
				return err
			}
			if pending > 0 {
				return ErrD2SPaidRevokePending
			}
			pendingKey := quote.LicenseID
			if err := tx.Model(&D2SOrder{}).Where("id = ? AND status = ?", order.ID, D2SOrderPending).Update("pending_key", pendingKey).Error; err != nil {
				return err
			}
			order.PendingKey = &pendingKey
		}
		if balanceMinor > 0 {
			account, err := d2sBalanceAccountTx(tx, userID, quote.Currency, now)
			if err != nil {
				return err
			}
			if account.AvailableMinor < balanceMinor {
				return ErrD2SInsufficientBalance
			}
			if err := tx.Model(&D2SBalanceAccount{}).Where("id = ?", account.ID).Updates(map[string]any{
				"available_minor": gorm.Expr("available_minor - ?", balanceMinor),
				"reserved_minor":  gorm.Expr("reserved_minor + ?", balanceMinor),
				"updated_at":      now,
			}).Error; err != nil {
				return err
			}
			if err := tx.Create(&D2SBalanceTransaction{
				ID: uuid.NewString(), AccountID: account.ID, UserID: userID, Currency: quote.Currency,
				Kind: "reserve", AmountMinor: -balanceMinor, ReferenceID: idempotencyKey, CreatedAt: now,
			}).Error; err != nil {
				return err
			}
		}
		if order.GatewayMinor == 0 {
			if err := fulfillD2SOrderTx(tx, &order, now); err != nil {
				return err
			}
			return settleD2SReservedBalanceTx(tx, &order, true, now)
		}
		return nil
	})
	return &order, err
}

func settleD2SReservedBalanceTx(tx *gorm.DB, order *D2SOrder, consume bool, now int64) error {
	if order.BalanceMinor <= 0 {
		return nil
	}
	account, err := d2sBalanceAccountTx(tx, order.UserID, order.Currency, now)
	if err != nil {
		return err
	}
	updates := map[string]any{"reserved_minor": gorm.Expr("reserved_minor - ?", order.BalanceMinor), "updated_at": now}
	kind, amount := "consume", int64(0)
	if !consume {
		updates["available_minor"] = gorm.Expr("available_minor + ?", order.BalanceMinor)
		kind, amount = "release", order.BalanceMinor
	}
	result := tx.Model(&D2SBalanceAccount{}).Where("id = ? AND reserved_minor >= ?", account.ID, order.BalanceMinor).Updates(updates)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return ErrD2SOrderState
	}
	return tx.Create(&D2SBalanceTransaction{
		ID: uuid.NewString(), AccountID: account.ID, UserID: order.UserID, Currency: order.Currency,
		Kind: kind, AmountMinor: amount, OrderID: order.ID, CreatedAt: now,
	}).Error
}

// ExpireD2SPendingOrders cancels expired pending orders and releases any
// balance reservation in the same transaction as the state transition.
func ExpireD2SPendingOrders(now int64) error {
	if now <= 0 {
		now = time.Now().Unix()
	}
	var orderIDs []string
	if err := DB.Model(&D2SOrder{}).
		Where("status = ? AND expires_at <= ?", D2SOrderPending, now).
		Pluck("id", &orderIDs).Error; err != nil {
		return err
	}
	for _, orderID := range orderIDs {
		if err := d2STransaction(func(tx *gorm.DB) error {
			var order D2SOrder
			if err := lockForUpdate(tx).Where("id = ?", orderID).First(&order).Error; err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					return nil
				}
				return err
			}
			if order.Status != D2SOrderPending || order.ExpiresAt > now {
				return nil
			}
			if err := tx.Model(&D2SOrder{}).Where("id = ? AND status = ?", order.ID, D2SOrderPending).
				Updates(map[string]any{"status": D2SOrderCanceled, "pending_key": nil}).Error; err != nil {
				return err
			}
			order.Status = D2SOrderCanceled
			return settleD2SReservedBalanceTx(tx, &order, false, now)
		}); err != nil {
			return err
		}
	}
	return nil
}

func fulfillD2SOrderTx(tx *gorm.DB, order *D2SOrder, now int64) error {
	switch order.Product {
	case D2SOrderProductLicense:
		code, err := newD2SLicenseCode()
		if err != nil {
			return err
		}
		license := D2SLicense{
			ID: uuid.NewString(), LicenseCode: code, UserID: order.UserID, Product: D2SProductDesktop2Stereo,
			Kind: D2SLicenseKindPaid, Status: D2SLicenseStatusActive, Mode: D2SLicenseModeUnbound,
			OfflinePeriodDays: 7, SourceOrderID: order.ID, CreatedAt: now, UpdatedAt: now,
		}
		if err := tx.Create(&license).Error; err != nil {
			return err
		}
		if err := tx.Create(&D2SLicenseEvent{ID: uuid.NewString(), LicenseID: license.ID, UserID: order.UserID, EventType: "license_purchased", OrderID: order.ID, CreatedAt: now}).Error; err != nil {
			return err
		}
		order.LicenseID = license.ID
		if err := tx.Model(&D2SOrder{}).Where("id = ?", order.ID).Update("license_id", license.ID).Error; err != nil {
			return err
		}
		if err := awardD2SInviteTx(tx, order, now); err != nil {
			return err
		}
	case D2SOrderProductPaidRevoke:
		license, err := GetD2SLicense(order.UserID, order.LicenseID, lockForUpdate(tx))
		if err != nil {
			return err
		}
		if license.Mode == D2SLicenseModePermanent {
			return ErrD2SPermanentLocked
		}
		month := time.Unix(now, 0).UTC().Format("2006-01")
		quotaID := license.ID + ":" + month
		var quota D2SPaidRevokeQuota
		if err := lockForUpdate(tx).Where("id = ?", quotaID).First(&quota).Error; err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if quota.UsedCount+1 != order.RevokeNumber || order.RevokeNumber > 3 {
			return ErrD2SPaidRevokeLimit
		}
		quota = D2SPaidRevokeQuota{ID: quotaID, LicenseID: license.ID, PeriodMonth: month, UsedCount: order.RevokeNumber, UpdatedAt: now}
		if err := tx.Where("id = ?", quotaID).Assign(quota).FirstOrCreate(&quota).Error; err != nil {
			return err
		}
		if err := tx.Where("license_id = ?", license.ID).Delete(&D2SDeviceBinding{}).Error; err != nil {
			return err
		}
		if err := tx.Where("license_id = ?", license.ID).Delete(&D2SOnlineLease{}).Error; err != nil {
			return err
		}
		if err := tx.Model(&D2SLicense{}).Where("id = ?", license.ID).Updates(map[string]any{"device_hash": "", "fingerprint_version": 0, "mode": D2SLicenseModeUnbound, "updated_at": now}).Error; err != nil {
			return err
		}
		if err := tx.Create(&D2SLicenseEvent{ID: uuid.NewString(), LicenseID: license.ID, UserID: order.UserID, EventType: "paid_revoke", DeviceHash: license.DeviceHash, OrderID: order.ID, CreatedAt: now}).Error; err != nil {
			return err
		}
	case D2SOrderProductOfflineExtension:
		license, err := GetD2SLicense(order.UserID, order.LicenseID, lockForUpdate(tx))
		if err != nil {
			return err
		}
		base := license.OfflineValidUntil
		if base < now {
			base = now
		}
		next := base + 30*86400
		if license.ExpiresAt > 0 && next > license.ExpiresAt {
			next = license.ExpiresAt
		}
		if err := tx.Model(&D2SLicense{}).Where("id = ?", license.ID).Updates(map[string]any{"offline_valid_until": next, "updated_at": now}).Error; err != nil {
			return err
		}
		if err := tx.Create(&D2SLicenseEvent{ID: uuid.NewString(), LicenseID: license.ID, UserID: order.UserID, EventType: "offline_extension_paid", OrderID: order.ID, CreatedAt: now}).Error; err != nil {
			return err
		}
	default:
		return ErrD2SOrderInvalid
	}
	order.Status, order.CompletedAt = D2SOrderPaid, now
	return tx.Model(&D2SOrder{}).Where("id = ? AND status = ?", order.ID, D2SOrderPending).Updates(map[string]any{"status": D2SOrderPaid, "completed_at": now, "pending_key": nil}).Error
}

func awardD2SInviteTx(tx *gorm.DB, order *D2SOrder, now int64) error {
	var invitee User
	if err := tx.Select("id", "inviter_id").First(&invitee, order.UserID).Error; err != nil || invitee.InviterId <= 0 || invitee.InviterId == invitee.Id {
		return err
	}
	var existing int64
	if err := tx.Model(&D2SInviteReward{}).Where("invitee_user_id = ?", invitee.Id).Count(&existing).Error; err != nil || existing > 0 {
		return err
	}
	var inviterProfile D2SUserProfile
	if err := tx.Where("user_id = ? AND region = ?", invitee.InviterId, order.Region).First(&inviterProfile).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		return err
	}
	var paidLicenses int64
	if err := tx.Model(&D2SLicense{}).Where(
		"user_id = ? AND kind = ? AND status = ? AND (expires_at = 0 OR expires_at > ?)",
		invitee.InviterId, D2SLicenseKindPaid, D2SLicenseStatusActive, now,
	).Count(&paidLicenses).Error; err != nil || paidLicenses == 0 {
		return err
	}
	amount := int64(140)
	if order.Region == D2SRegionCN {
		amount = 1000
	}
	account, err := d2sBalanceAccountTx(tx, invitee.InviterId, order.Currency, now)
	if err != nil {
		return err
	}
	if err := tx.Model(&D2SBalanceAccount{}).Where("id = ?", account.ID).Updates(map[string]any{"available_minor": gorm.Expr("available_minor + ?", amount), "updated_at": now}).Error; err != nil {
		return err
	}
	reward := D2SInviteReward{ID: uuid.NewString(), InviterUserID: invitee.InviterId, InviteeUserID: invitee.Id, OrderID: order.ID, Currency: order.Currency, AmountMinor: amount, Status: "credited", CreatedAt: now}
	if err := tx.Create(&reward).Error; err != nil {
		return err
	}
	return tx.Create(&D2SBalanceTransaction{ID: uuid.NewString(), AccountID: account.ID, UserID: invitee.InviterId, Currency: order.Currency, Kind: "invite_reward", AmountMinor: amount, OrderID: order.ID, ReferenceID: reward.ID, CreatedAt: now}).Error
}

func ProcessD2SPaymentEvent(provider, eventID, orderID, eventType string, amountMinor int64, currency, payloadHash string, now int64) (*D2SOrder, error) {
	provider = strings.ToLower(strings.TrimSpace(provider))
	eventType = strings.ToLower(strings.TrimSpace(eventType))
	if _, ok := D2SProviderRegion(provider); !ok || strings.TrimSpace(eventID) == "" || strings.TrimSpace(orderID) == "" {
		return nil, ErrD2SOrderInvalid
	}
	if now <= 0 {
		now = time.Now().Unix()
	}
	var order D2SOrder
	err := d2STransaction(func(tx *gorm.DB) error {
		currency = strings.ToUpper(currency)
		event := D2SPaymentEvent{ID: uuid.NewString(), Provider: provider, ProviderEventID: eventID, OrderID: orderID, EventType: eventType, AmountMinor: amountMinor, Currency: currency, PayloadHash: payloadHash, ProcessedAt: now}
		created := tx.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "provider"}, {Name: "provider_event_id"}},
			DoNothing: true,
		}).Create(&event)
		if created.Error != nil {
			return created.Error
		}
		if created.RowsAffected == 0 {
			var existing D2SPaymentEvent
			if err := tx.Where("provider = ? AND provider_event_id = ?", provider, eventID).First(&existing).Error; err != nil {
				return err
			}
			if existing.OrderID != orderID || existing.EventType != eventType || existing.AmountMinor != amountMinor || existing.Currency != currency || existing.PayloadHash != payloadHash {
				return ErrD2SPaymentMismatch
			}
			return tx.Where("id = ?", existing.OrderID).First(&order).Error
		}
		if err := lockForUpdate(tx).Where("id = ?", orderID).First(&order).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrD2SOrderNotFound
			}
			return err
		}
		if order.Provider != provider || order.Currency != currency || order.GatewayMinor != amountMinor {
			return ErrD2SPaymentMismatch
		}
		providerRegion, ok := D2SProviderRegion(provider)
		if !ok || order.Region != providerRegion {
			return ErrD2SRegionMismatch
		}
		switch eventType {
		case "paid":
			if order.Status != D2SOrderPending || order.ExpiresAt <= now {
				return ErrD2SOrderState
			}
			var profile D2SUserProfile
			if err := lockForUpdate(tx).Where("user_id = ?", order.UserID).First(&profile).Error; err != nil {
				return err
			}
			if profile.Region != "" && profile.Region != order.Region {
				return ErrD2SRegionMismatch
			}
			if profile.Region == "" {
				if err := tx.Model(&D2SUserProfile{}).Where("user_id = ? AND region = ''", order.UserID).Updates(map[string]any{"region": order.Region, "region_locked_at": now, "updated_at": now}).Error; err != nil {
					return err
				}
			}
			if err := fulfillD2SOrderTx(tx, &order, now); err != nil {
				return err
			}
			return settleD2SReservedBalanceTx(tx, &order, true, now)
		case "canceled", "failed":
			if order.Status != D2SOrderPending {
				return ErrD2SOrderState
			}
			if err := tx.Model(&D2SOrder{}).Where("id = ?", order.ID).Updates(map[string]any{"status": D2SOrderCanceled, "pending_key": nil}).Error; err != nil {
				return err
			}
			order.Status = D2SOrderCanceled
			return settleD2SReservedBalanceTx(tx, &order, false, now)
		case "chargeback", "reversed":
			if order.Status != D2SOrderPaid {
				return ErrD2SOrderState
			}
			var paidEvents int64
			if err := tx.Model(&D2SPaymentEvent{}).Where("provider = ? AND order_id = ? AND event_type = ?", provider, order.ID, "paid").Count(&paidEvents).Error; err != nil {
				return err
			}
			if paidEvents == 0 {
				return ErrD2SOrderState
			}
			if order.LicenseID != "" {
				if err := tx.Model(&D2SLicense{}).Where("id = ?", order.LicenseID).Updates(map[string]any{"status": D2SLicenseStatusSuspended, "updated_at": now}).Error; err != nil {
					return err
				}
			}
			if err := reverseD2SInviteRewardTx(tx, &order, now); err != nil {
				return err
			}
			order.Status = D2SOrderChargeback
			return tx.Model(&D2SOrder{}).Where("id = ?", order.ID).Update("status", D2SOrderChargeback).Error
		default:
			return ErrD2SOrderInvalid
		}
	})
	return &order, err
}

func reverseD2SInviteRewardTx(tx *gorm.DB, order *D2SOrder, now int64) error {
	var reward D2SInviteReward
	if err := lockForUpdate(tx).Where("order_id = ? AND status = ?", order.ID, "credited").First(&reward).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		return err
	}
	account, err := d2sBalanceAccountTx(tx, reward.InviterUserID, reward.Currency, now)
	if err != nil {
		return err
	}
	if err := tx.Model(&D2SBalanceAccount{}).Where("id = ?", account.ID).Updates(map[string]any{"available_minor": gorm.Expr("available_minor - ?", reward.AmountMinor), "updated_at": now}).Error; err != nil {
		return err
	}
	if err := tx.Model(&D2SInviteReward{}).Where("id = ?", reward.ID).Update("status", "reversed").Error; err != nil {
		return err
	}
	return tx.Create(&D2SBalanceTransaction{ID: uuid.NewString(), AccountID: account.ID, UserID: reward.InviterUserID, Currency: reward.Currency, Kind: "invite_reversal", AmountMinor: -reward.AmountMinor, OrderID: order.ID, ReferenceID: reward.ID, CreatedAt: now}).Error
}

func CreateD2SWithdrawal(userID int, amountMinor int64, alipayAccount, realName string, now int64) (*D2SWithdrawalRequest, error) {
	if amountMinor < 5000 || strings.TrimSpace(alipayAccount) == "" || strings.TrimSpace(realName) == "" {
		return nil, ErrD2SWithdrawalNotAllowed
	}
	profile, err := EnsureD2SProfileAndTrial(userID, now)
	if err != nil {
		return nil, err
	}
	if profile.Region != D2SRegionCN {
		return nil, ErrD2SWithdrawalNotAllowed
	}
	if now <= 0 {
		now = time.Now().Unix()
	}
	row := &D2SWithdrawalRequest{ID: uuid.NewString(), UserID: userID, Currency: "CNY", AmountMinor: amountMinor, AlipayAccount: strings.TrimSpace(alipayAccount), RealName: strings.TrimSpace(realName), Status: "pending", CreatedAt: now, UpdatedAt: now}
	err = d2STransaction(func(tx *gorm.DB) error {
		account, err := d2sBalanceAccountTx(tx, userID, "CNY", now)
		if err != nil {
			return err
		}
		if account.AvailableMinor < amountMinor {
			return ErrD2SInsufficientBalance
		}
		if err := tx.Model(&D2SBalanceAccount{}).Where("id = ?", account.ID).Updates(map[string]any{"available_minor": gorm.Expr("available_minor - ?", amountMinor), "reserved_minor": gorm.Expr("reserved_minor + ?", amountMinor), "updated_at": now}).Error; err != nil {
			return err
		}
		if err := tx.Create(row).Error; err != nil {
			return err
		}
		return tx.Create(&D2SBalanceTransaction{ID: uuid.NewString(), AccountID: account.ID, UserID: userID, Currency: "CNY", Kind: "withdrawal_reserve", AmountMinor: -amountMinor, ReferenceID: row.ID, CreatedAt: now}).Error
	})
	return row, err
}

func ReviewD2SWithdrawal(adminID int, requestID, status, note string, now int64) (*D2SWithdrawalRequest, error) {
	if status != "paid" && status != "rejected" {
		return nil, ErrD2SWithdrawalNotAllowed
	}
	var row D2SWithdrawalRequest
	err := d2STransaction(func(tx *gorm.DB) error {
		if err := lockForUpdate(tx).Where("id = ?", requestID).First(&row).Error; err != nil {
			return err
		}
		if row.Status != "pending" {
			return ErrD2SOrderState
		}
		account, err := d2sBalanceAccountTx(tx, row.UserID, row.Currency, now)
		if err != nil {
			return err
		}
		updates := map[string]any{"reserved_minor": gorm.Expr("reserved_minor - ?", row.AmountMinor), "updated_at": now}
		kind, amount, paidAt := "withdrawal_paid", int64(0), int64(0)
		if status == "rejected" {
			updates["available_minor"] = gorm.Expr("available_minor + ?", row.AmountMinor)
			kind, amount = "withdrawal_release", row.AmountMinor
		} else {
			paidAt = now
		}
		result := tx.Model(&D2SBalanceAccount{}).Where("id = ? AND reserved_minor >= ?", account.ID, row.AmountMinor).Updates(updates)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrD2SOrderState
		}
		if err := tx.Model(&D2SWithdrawalRequest{}).Where("id = ?", row.ID).Updates(map[string]any{"status": status, "reviewed_by": adminID, "review_note": note, "updated_at": now, "paid_at": paidAt}).Error; err != nil {
			return err
		}
		row.Status, row.ReviewedBy, row.ReviewNote, row.UpdatedAt, row.PaidAt = status, adminID, note, now, paidAt
		return tx.Create(&D2SBalanceTransaction{ID: uuid.NewString(), AccountID: account.ID, UserID: row.UserID, Currency: row.Currency, Kind: kind, AmountMinor: amount, ReferenceID: row.ID, CreatedAt: now}).Error
	})
	return &row, err
}

func ReviewD2SManualUnbind(adminID int, requestID, status, note string, now int64) (*D2SManualUnbindRequest, error) {
	if status != "approved" && status != "rejected" {
		return nil, ErrD2SOrderInvalid
	}
	var request D2SManualUnbindRequest
	err := d2STransaction(func(tx *gorm.DB) error {
		if err := lockForUpdate(tx).Where("id = ?", requestID).First(&request).Error; err != nil {
			return err
		}
		if request.Status != "pending" {
			return ErrD2SOrderState
		}
		if status == "approved" {
			var approved int64
			if err := tx.Model(&D2SManualUnbindRequest{}).Where("user_id = ? AND status = ?", request.UserID, "approved").Count(&approved).Error; err != nil {
				return err
			}
			if approved > 0 {
				return fmt.Errorf("%w: lifetime manual unbind already used", ErrD2SPermanentLocked)
			}
			license, err := GetD2SLicense(request.UserID, request.LicenseID, lockForUpdate(tx))
			if err != nil {
				return err
			}
			if license.Mode != D2SLicenseModePermanent {
				return ErrD2SPermanentLocked
			}
			if err := tx.Where("license_id = ?", license.ID).Delete(&D2SDeviceBinding{}).Error; err != nil {
				return err
			}
			if err := tx.Model(&D2SLicense{}).Where("id = ?", license.ID).Updates(map[string]any{"device_hash": "", "fingerprint_version": 0, "mode": D2SLicenseModeUnbound, "permanent_bound_at": 0, "updated_at": now}).Error; err != nil {
				return err
			}
			if err := tx.Create(&D2SLicenseEvent{ID: uuid.NewString(), LicenseID: license.ID, UserID: request.UserID, EventType: "manual_unbind_approved", DeviceHash: license.DeviceHash, CreatedAt: now}).Error; err != nil {
				return err
			}
		}
		if err := tx.Model(&D2SManualUnbindRequest{}).Where("id = ?", request.ID).Updates(map[string]any{"status": status, "reviewed_by": adminID, "review_note": note, "updated_at": now}).Error; err != nil {
			return err
		}
		request.Status, request.ReviewedBy, request.ReviewNote, request.UpdatedAt = status, adminID, note, now
		return nil
	})
	return &request, err
}

func SetD2SUserRegion(adminID, userID int, region string, now int64) (*D2SUserProfile, error) {
	region = strings.ToUpper(strings.TrimSpace(region))
	if region != D2SRegionCN && region != D2SRegionINTL {
		return nil, ErrD2SRegionMismatch
	}
	var profile D2SUserProfile
	err := d2STransaction(func(tx *gorm.DB) error {
		if err := lockForUpdate(tx).Where("user_id = ?", userID).First(&profile).Error; err != nil {
			return err
		}
		var unsafeCount int64
		if err := tx.Model(&D2SBalanceAccount{}).Where("user_id = ? AND (available_minor <> 0 OR reserved_minor <> 0)", userID).Count(&unsafeCount).Error; err != nil {
			return err
		}
		if unsafeCount > 0 {
			return ErrD2SRegionMismatch
		}
		if err := tx.Model(&D2SOrder{}).Where("user_id = ? AND status = ?", userID, D2SOrderPending).Count(&unsafeCount).Error; err != nil {
			return err
		}
		if unsafeCount > 0 {
			return ErrD2SRegionMismatch
		}
		if err := tx.Model(&D2SWithdrawalRequest{}).Where("user_id = ? AND status = ?", userID, "pending").Count(&unsafeCount).Error; err != nil {
			return err
		}
		if unsafeCount > 0 {
			return ErrD2SRegionMismatch
		}
		profile.Region, profile.RegionLockedAt, profile.UpdatedAt = region, now, now
		return tx.Model(&D2SUserProfile{}).Where("user_id = ?", userID).Updates(map[string]any{"region": region, "region_locked_at": now, "updated_at": now}).Error
	})
	_ = adminID
	return &profile, err
}
