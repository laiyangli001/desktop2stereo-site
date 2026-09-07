package model

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestD2STransientTransactionErrors(t *testing.T) {
	for _, message := range []string{
		"database is locked",
		"deadlock found when trying to get lock",
		"ERROR: deadlock detected",
		"could not serialize access due to concurrent update",
	} {
		assert.True(t, isD2STransientTransactionError(errors.New(message)), message)
	}
	assert.False(t, isD2STransientTransactionError(ErrD2SOrderState))
}

func useD2STestDB(t *testing.T) {
	t.Helper()
	previousDB, previousLogDB := DB, LOG_DB
	previousMainType, previousLogType := common.MainDatabaseType(), common.LogDatabaseType()
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	DB, LOG_DB = db, db
	require.NoError(t, db.AutoMigrate(
		&User{}, &D2SUserProfile{}, &D2SLicense{}, &D2SDeviceBinding{}, &D2SLicenseEvent{},
		&D2SDeviceCode{}, &D2SOfflineEntitlement{}, &D2SOnlineLease{}, &D2SFreeRevokeCooldown{},
		&D2SPaidRevokeQuota{}, &D2SManualUnbindRequest{}, &D2SOrder{}, &D2SPaymentEvent{},
		&D2SBalanceAccount{}, &D2SBalanceTransaction{}, &D2SInviteReward{}, &D2SWithdrawalRequest{},
		&D2SSigningKey{},
	))
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = sqlDB.Close()
		DB, LOG_DB = previousDB, previousLogDB
		common.SetDatabaseTypes(previousMainType, previousLogType)
	})
}

func createD2STestUser(t *testing.T, username string) User {
	t.Helper()
	user := User{Username: username, Password: "unused-hash", Email: username + "@example.com", AffCode: username, Role: common.RoleCommonUser, Status: common.UserStatusEnabled, Group: "default", AuthVersion: 1}
	require.NoError(t, DB.Create(&user).Error)
	return user
}

func TestD2STrialProvisioningIsIdempotent(t *testing.T) {
	useD2STestDB(t)
	user := createD2STestUser(t, "d2s-trial")
	const now = int64(2_000_000_000)

	first, err := ListD2SLicenses(user.Id, now)
	require.NoError(t, err)
	second, err := ListD2SLicenses(user.Id, now+10)
	require.NoError(t, err)

	require.Len(t, first, 1)
	require.Len(t, second, 1)
	assert.Equal(t, first[0].ID, second[0].ID)
	assert.Equal(t, D2SLicenseKindTrial, first[0].Kind)
	assert.Equal(t, D2SLicenseModeUnbound, first[0].Mode)
	assert.Equal(t, now+30*86400, first[0].ExpiresAt)
}

func TestD2SExpiredPendingOrderReleasesBalanceReservation(t *testing.T) {
	useD2STestDB(t)
	user := createD2STestUser(t, "d2s-expired-order")
	const now = int64(2_000_010_000)
	quote, err := QuoteD2SOrder(user.Id, D2SOrderProductLicense, "", "epay", now)
	require.NoError(t, err)
	require.NoError(t, DB.Create(&D2SBalanceAccount{
		ID: "expired-order-balance", UserID: user.Id, Currency: "CNY", AvailableMinor: 2000, UpdatedAt: now,
	}).Error)
	order, err := CreateD2SOrder(user.Id, quote, 1000, "expired-order", now)
	require.NoError(t, err)
	require.NoError(t, ExpireD2SPendingOrders(order.ExpiresAt+1))

	var stored D2SOrder
	require.NoError(t, DB.First(&stored, "id = ?", order.ID).Error)
	assert.Equal(t, D2SOrderCanceled, stored.Status)
	var account D2SBalanceAccount
	require.NoError(t, DB.Where("id = ?", "expired-order-balance").First(&account).Error)
	assert.EqualValues(t, 2000, account.AvailableMinor)
	assert.Zero(t, account.ReservedMinor)
	var releaseCount int64
	require.NoError(t, DB.Model(&D2SBalanceTransaction{}).Where("order_id = ? AND kind = ?", order.ID, "release").Count(&releaseCount).Error)
	assert.EqualValues(t, 1, releaseCount)
}

func TestD2SSQLiteConcurrentCriticalPaths(t *testing.T) {
	useD2STestDB(t)
	user := createD2STestUser(t, "d2s-sqlite-concurrent")
	const now = int64(2_000_050_000)
	const workers = 4

	var wait sync.WaitGroup
	trialErrors := make(chan error, workers)
	for i := 0; i < workers; i++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			_, err := ListD2SLicenses(user.Id, now)
			trialErrors <- err
		}()
	}
	wait.Wait()
	close(trialErrors)
	for err := range trialErrors {
		require.NoError(t, err)
	}
	var trialCount int64
	require.NoError(t, DB.Model(&D2SLicense{}).Where("user_id = ? AND kind = ?", user.Id, D2SLicenseKindTrial).Count(&trialCount).Error)
	assert.EqualValues(t, 1, trialCount)

	quote, err := QuoteD2SOrder(user.Id, D2SOrderProductLicense, "", "stripe", now+1)
	require.NoError(t, err)
	orderResults := make(chan *D2SOrder, workers)
	orderErrors := make(chan error, workers)
	for i := 0; i < workers; i++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			order, callErr := CreateD2SOrder(user.Id, quote, 0, "sqlite-concurrent-order", now+2)
			orderResults <- order
			orderErrors <- callErr
		}()
	}
	wait.Wait()
	close(orderResults)
	close(orderErrors)
	var orderID string
	for order := range orderResults {
		if orderID == "" && order != nil {
			orderID = order.ID
		} else if order != nil {
			assert.Equal(t, orderID, order.ID)
		}
	}
	for err := range orderErrors {
		require.NoError(t, err)
	}
	require.NotEmpty(t, orderID)

	eventErrors := make(chan error, workers)
	for i := 0; i < workers; i++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			_, err := ProcessD2SPaymentEvent("stripe", "sqlite-concurrent-event", orderID, "paid", quote.AmountMinor, quote.Currency, "sqlite-concurrent-payload", now+3)
			eventErrors <- err
		}()
	}
	wait.Wait()
	close(eventErrors)
	for err := range eventErrors {
		require.NoError(t, err)
	}
	var eventCount, paidCount int64
	require.NoError(t, DB.Model(&D2SPaymentEvent{}).Where("provider = ? AND provider_event_id = ?", "stripe", "sqlite-concurrent-event").Count(&eventCount).Error)
	require.NoError(t, DB.Model(&D2SOrder{}).Where("id = ? AND status = ?", orderID, D2SOrderPaid).Count(&paidCount).Error)
	assert.EqualValues(t, 1, eventCount)
	assert.EqualValues(t, 1, paidCount)
}

func TestD2SBindingCooldownAndLeaseContracts(t *testing.T) {
	useD2STestDB(t)
	userA := createD2STestUser(t, "d2s-bind-a")
	userB := createD2STestUser(t, "d2s-bind-b")
	const now = int64(2_000_100_000)
	licensesA, err := ListD2SLicenses(userA.Id, now)
	require.NoError(t, err)
	licensesB, err := ListD2SLicenses(userB.Id, now)
	require.NoError(t, err)
	device := strings.Repeat("a", 64)

	bound, err := BindD2SLicense(userA.Id, licensesA[0].ID, device, 1, now)
	require.NoError(t, err)
	assert.Equal(t, D2SLicenseModeOnline, bound.Mode)
	_, err = BindD2SLicense(userB.Id, licensesB[0].ID, device, 1, now)
	assert.ErrorIs(t, err, ErrD2SDeviceAlreadyBound)

	token, expiresAt, err := StartOrRenewD2SOnlineLease(userA.Id, bound.ID, device, "", now)
	require.NoError(t, err)
	assert.NotEmpty(t, token)
	assert.Equal(t, now+900, expiresAt)
	_, _, err = StartOrRenewD2SOnlineLease(userA.Id, bound.ID, device, "", now+1)
	assert.ErrorIs(t, err, ErrD2SLeaseConflict)
	_, renewedAt, err := StartOrRenewD2SOnlineLease(userA.Id, bound.ID, device, token, now+300)
	require.NoError(t, err)
	assert.Equal(t, now+1200, renewedAt)
	require.NoError(t, ReleaseAllD2SOnlineLeases(userA.Id))
	var activeLeases int64
	require.NoError(t, DB.Model(&D2SOnlineLease{}).Where("user_id = ?", userA.Id).Count(&activeLeases).Error)
	assert.Zero(t, activeLeases)

	_, err = ChangeD2SLicenseMode(userA.Id, bound.ID, device, D2SLicenseModeOffline, "", 14, now+1)
	require.NoError(t, err)
	next, err := FreeRevokeD2SLicense(userA.Id, bound.ID, device, now+2)
	require.NoError(t, err)
	assert.Equal(t, now+2+14*86400, next)
	_, err = BindD2SLicense(userA.Id, bound.ID, device, 1, now+3)
	require.NoError(t, err)
	_, err = FreeRevokeD2SLicense(userA.Id, bound.ID, device, now+4)
	assert.ErrorIs(t, err, ErrD2SRevokeCooldown)
}

func TestD2SDeviceCodeIsSingleUse(t *testing.T) {
	useD2STestDB(t)
	user := createD2STestUser(t, "d2s-device-code")
	const now = int64(2_000_200_000)
	row, secret, err := CreateD2SDeviceCode(strings.Repeat("b", 64), 1, "Windows launcher", "windows", now)
	require.NoError(t, err)
	_, err = ClaimD2SDeviceCode(secret, now+1)
	assert.ErrorIs(t, err, ErrD2SDeviceCodePending)
	approved, err := ApproveD2SDeviceCode(user.Id, row.UserCode, now+2)
	require.NoError(t, err)
	assert.Equal(t, user.Id, approved.UserID)
	claimed, err := ClaimD2SDeviceCode(secret, now+3)
	require.NoError(t, err)
	require.NoError(t, FinishD2SDeviceCodeClaim(claimed.ID, true, now+3))
	_, err = ClaimD2SDeviceCode(secret, now+4)
	assert.ErrorIs(t, err, ErrD2SDeviceCodeConsumed)
}

func TestD2SDeviceCodeRejectsMetadataExceedingColumnLimits(t *testing.T) {
	useD2STestDB(t)
	deviceHash := strings.Repeat("b", 64)

	_, _, err := CreateD2SDeviceCode(deviceHash, 1, strings.Repeat("n", 129), "windows", 2_000_100_000)
	assert.ErrorIs(t, err, ErrD2SDeviceCodeInvalid)

	_, _, err = CreateD2SDeviceCode(deviceHash, 1, "Windows launcher", strings.Repeat("p", 33), 2_000_100_000)
	assert.ErrorIs(t, err, ErrD2SDeviceCodeInvalid)
}

func TestD2SExpiredRuntimeArtifactsAreCleaned(t *testing.T) {
	useD2STestDB(t)
	const now = int64(2_000_250_000)
	require.NoError(t, DB.Create(&D2SDeviceCode{ID: "expired-code", DeviceCodeHash: strings.Repeat("1", 64), UserCode: "AAAA-BBBB", DeviceHash: strings.Repeat("b", 64), FingerprintVersion: 1, Status: D2SDeviceCodePending, CreatedAt: now - 700, ExpiresAt: now - 100}).Error)
	require.NoError(t, DB.Create(&D2SDeviceCode{ID: "active-code", DeviceCodeHash: strings.Repeat("2", 64), UserCode: "CCCC-DDDD", DeviceHash: strings.Repeat("c", 64), FingerprintVersion: 1, Status: D2SDeviceCodePending, CreatedAt: now, ExpiresAt: now + 600}).Error)
	require.NoError(t, DB.Create(&D2SOnlineLease{LicenseID: "expired-license", UserID: 1, DeviceHash: strings.Repeat("d", 64), LeaseTokenHash: strings.Repeat("3", 64), CreatedAt: now - 1000, ExpiresAt: now - 1}).Error)

	require.NoError(t, DeleteExpiredD2SRuntimeArtifacts(now))
	var deviceCodeCount, leaseCount int64
	require.NoError(t, DB.Model(&D2SDeviceCode{}).Count(&deviceCodeCount).Error)
	require.NoError(t, DB.Model(&D2SOnlineLease{}).Count(&leaseCount).Error)
	assert.EqualValues(t, 1, deviceCodeCount)
	assert.Zero(t, leaseCount)
}

func TestD2SOrderLocksRegionAndCreatesPaidLicense(t *testing.T) {
	useD2STestDB(t)
	user := createD2STestUser(t, "d2s-order")
	const now = int64(2_000_300_000)
	quote, err := QuoteD2SOrder(user.Id, D2SOrderProductLicense, "", "stripe", now)
	require.NoError(t, err)
	assert.Equal(t, int64(2990), quote.AmountMinor)

	order, err := CreateD2SOrder(user.Id, quote, 0, "order-idempotency-1", now)
	require.NoError(t, err)
	assert.Equal(t, D2SOrderPending, order.Status)
	paid, err := ProcessD2SPaymentEvent("stripe", "evt-1", order.ID, "paid", 2990, "USD", "payload-1", now+10)
	require.NoError(t, err)
	assert.Equal(t, D2SOrderPaid, paid.Status)
	assert.NotEmpty(t, paid.LicenseID)
	var profile D2SUserProfile
	require.NoError(t, DB.First(&profile, "user_id = ?", user.Id).Error)
	assert.Equal(t, D2SRegionINTL, profile.Region)
	var paidLicenses int64
	require.NoError(t, DB.Model(&D2SLicense{}).Where("user_id = ? AND kind = ?", user.Id, D2SLicenseKindPaid).Count(&paidLicenses).Error)
	assert.EqualValues(t, 1, paidLicenses)

	idempotent, err := ProcessD2SPaymentEvent("stripe", "evt-1", order.ID, "paid", 2990, "USD", "payload-1", now+20)
	require.NoError(t, err)
	assert.Equal(t, paid.ID, idempotent.ID)
	_, err = QuoteD2SOrder(user.Id, D2SOrderProductLicense, "", "epay", now+30)
	assert.ErrorIs(t, err, ErrD2SRegionMismatch)

	otherUser := createD2STestUser(t, "d2s-order-other")
	otherQuote, err := QuoteD2SOrder(otherUser.Id, D2SOrderProductLicense, "", "stripe", now+40)
	require.NoError(t, err)
	otherOrder, err := CreateD2SOrder(otherUser.Id, otherQuote, 0, "order-idempotency-1", now+40)
	require.NoError(t, err, "idempotency keys are scoped per user")
	assert.NotEqual(t, order.ID, otherOrder.ID)
}

func TestD2SOrderIdempotencyRejectsChangedRequest(t *testing.T) {
	useD2STestDB(t)
	user := createD2STestUser(t, "d2s-order-idempotency")
	const now = int64(2_000_350_000)

	quote, err := QuoteD2SOrder(user.Id, D2SOrderProductLicense, "", "stripe", now)
	require.NoError(t, err)
	first, err := CreateD2SOrder(user.Id, quote, 0, "  same-request  ", now)
	require.NoError(t, err)

	_, err = CreateD2SOrder(user.Id, quote, 1, "same-request", now+1)
	assert.ErrorIs(t, err, ErrD2SOrderInvalid)

	repeated, err := CreateD2SOrder(user.Id, quote, 0, "same-request", now+2)
	require.NoError(t, err)
	assert.Equal(t, first.ID, repeated.ID)
	assert.Equal(t, "same-request", repeated.IdempotencyKey)
}

func TestD2SChargebackSuspendsLicenseAndIsIdempotent(t *testing.T) {
	useD2STestDB(t)
	user := createD2STestUser(t, "d2s-chargeback")
	const now = int64(2_000_360_000)
	quote, err := QuoteD2SOrder(user.Id, D2SOrderProductLicense, "", "stripe", now)
	require.NoError(t, err)
	order, err := CreateD2SOrder(user.Id, quote, 0, "chargeback-order", now)
	require.NoError(t, err)
	paid, err := ProcessD2SPaymentEvent("stripe", "chargeback-paid", order.ID, "paid", quote.AmountMinor, "USD", "chargeback-paid-payload", now+1)
	require.NoError(t, err)

	chargeback, err := ProcessD2SPaymentEvent("stripe", "chargeback-1", order.ID, "chargeback", quote.AmountMinor, "USD", "chargeback-payload", now+2)
	require.NoError(t, err)
	assert.Equal(t, D2SOrderChargeback, chargeback.Status)
	var license D2SLicense
	require.NoError(t, DB.First(&license, "id = ?", paid.LicenseID).Error)
	assert.Equal(t, D2SLicenseStatusSuspended, license.Status)

	repeated, err := ProcessD2SPaymentEvent("stripe", "chargeback-1", order.ID, "chargeback", quote.AmountMinor, "USD", "chargeback-payload", now+3)
	require.NoError(t, err)
	assert.Equal(t, chargeback.ID, repeated.ID)
	_, err = ProcessD2SPaymentEvent("stripe", "chargeback-1", order.ID, "chargeback", quote.AmountMinor-1, "USD", "chargeback-payload", now+4)
	assert.ErrorIs(t, err, ErrD2SPaymentMismatch)
}

func TestD2SReversalRequiresOriginalPaidEvent(t *testing.T) {
	useD2STestDB(t)
	user := createD2STestUser(t, "d2s-reversal-without-paid")
	const now = int64(2_000_365_000)
	quote, err := QuoteD2SOrder(user.Id, D2SOrderProductLicense, "", "stripe", now)
	require.NoError(t, err)
	order, err := CreateD2SOrder(user.Id, quote, 0, "reversal-without-paid", now)
	require.NoError(t, err)
	order.Status = D2SOrderPaid
	order.LicenseID = "missing-paid-license"
	require.NoError(t, DB.Model(&D2SOrder{}).Where("id = ?", order.ID).Updates(map[string]any{"status": D2SOrderPaid, "license_id": order.LicenseID}).Error)

	_, err = ProcessD2SPaymentEvent("stripe", "orphan-reversal", order.ID, "reversed", quote.AmountMinor, quote.Currency, "orphan-reversal-payload", now+1)
	assert.ErrorIs(t, err, ErrD2SOrderState)
	var eventCount int64
	require.NoError(t, DB.Model(&D2SPaymentEvent{}).Where("provider_event_id = ?", "orphan-reversal").Count(&eventCount).Error)
	assert.Zero(t, eventCount)
}

func TestD2SPaymentEventLifecycleMatrix(t *testing.T) {
	tests := []struct {
		name       string
		eventType  string
		firstEvent string
		wantStatus string
	}{
		{name: "paid", eventType: "paid", wantStatus: D2SOrderPaid},
		{name: "canceled", eventType: "canceled", wantStatus: D2SOrderCanceled},
		{name: "failed", eventType: "failed", wantStatus: D2SOrderCanceled},
		{name: "reversed after paid", eventType: "reversed", firstEvent: "paid", wantStatus: D2SOrderChargeback},
		{name: "chargeback after paid", eventType: "chargeback", firstEvent: "paid", wantStatus: D2SOrderChargeback},
	}
	for index, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			useD2STestDB(t)
			user := createD2STestUser(t, fmt.Sprintf("d2s-lifecycle-%d", index))
			const now = int64(2_000_700_000)
			quote, err := QuoteD2SOrder(user.Id, D2SOrderProductLicense, "", "stripe", now)
			require.NoError(t, err)
			order, err := CreateD2SOrder(user.Id, quote, 0, "lifecycle-order", now)
			require.NoError(t, err)
			if test.firstEvent != "" {
				order, err = ProcessD2SPaymentEvent("stripe", "lifecycle-paid", order.ID, test.firstEvent, quote.AmountMinor, quote.Currency, "lifecycle-paid-payload", now+1)
				require.NoError(t, err)
			}
			result, err := ProcessD2SPaymentEvent("stripe", "lifecycle-final", order.ID, test.eventType, quote.AmountMinor, quote.Currency, "lifecycle-final-payload", now+2)
			require.NoError(t, err)
			assert.Equal(t, test.wantStatus, result.Status)
		})
	}
}

func TestD2SPaymentEventSharedValidationAcrossOpenProviders(t *testing.T) {
	tests := []struct {
		provider string
		region   string
		currency string
	}{
		{provider: "epay", region: D2SRegionCN, currency: "CNY"},
		{provider: "paymentfm", region: D2SRegionCN, currency: "CNY"},
		{provider: "alipay", region: D2SRegionCN, currency: "CNY"},
		{provider: "wechat", region: D2SRegionCN, currency: "CNY"},
		{provider: "waffo", region: D2SRegionCN, currency: "CNY"},
		{provider: "stripe", region: D2SRegionINTL, currency: "USD"},
		{provider: "creem", region: D2SRegionINTL, currency: "USD"},
		{provider: "waffo_pancake", region: D2SRegionINTL, currency: "USD"},
	}

	for index, test := range tests {
		t.Run(test.provider, func(t *testing.T) {
			useD2STestDB(t)
			user := createD2STestUser(t, fmt.Sprintf("d2s-provider-validation-%d", index))
			const now = int64(2_000_710_000)
			_, err := EnsureD2SProfileAndTrial(user.Id, now)
			require.NoError(t, err)
			order := D2SOrder{
				ID: "shared-validation-" + test.provider, UserID: user.Id,
				Product: D2SOrderProductLicense, Provider: test.provider,
				Region: test.region, Currency: test.currency, AmountMinor: 2990,
				GatewayMinor: 2990, Status: D2SOrderPending, CreatedAt: now, ExpiresAt: now + 1800,
			}
			require.NoError(t, DB.Create(&order).Error)

			_, err = ProcessD2SPaymentEvent(test.provider, "shared-validation-paid", order.ID, "paid", 2990, test.currency, "shared-validation-payload", now+1)
			require.NoError(t, err)
			_, err = ProcessD2SPaymentEvent(test.provider, "shared-validation-paid", order.ID, "paid", 2990, test.currency, "shared-validation-payload", now+2)
			require.NoError(t, err)
			_, err = ProcessD2SPaymentEvent(test.provider, "shared-validation-paid", order.ID, "paid", 2991, test.currency, "shared-validation-payload", now+3)
			assert.ErrorIs(t, err, ErrD2SPaymentMismatch)

			var stored D2SOrder
			require.NoError(t, DB.First(&stored, "id = ?", order.ID).Error)
			assert.Equal(t, D2SOrderPaid, stored.Status)
		})
	}
}

func TestD2SProviderRegionExcludesUnsupportedLaunchProviders(t *testing.T) {
	_, ok := D2SProviderRegion("paypal")
	assert.False(t, ok)
	_, ok = D2SProviderRegion("paddle")
	assert.False(t, ok)
}

func TestD2SPaidRevokePendingKeyReleasesAfterCancellation(t *testing.T) {
	useD2STestDB(t)
	user := createD2STestUser(t, "d2s-paid-revoke-pending")
	const now = int64(2_000_375_000)
	licenses, err := ListD2SLicenses(user.Id, now)
	require.NoError(t, err)
	device := strings.Repeat("e", 64)
	license, err := BindD2SLicense(user.Id, licenses[0].ID, device, 1, now+1)
	require.NoError(t, err)

	quote, err := QuoteD2SOrder(user.Id, D2SOrderProductPaidRevoke, license.ID, "stripe", now+2)
	require.NoError(t, err)
	first, err := CreateD2SOrder(user.Id, quote, 0, "paid-revoke-1", now+2)
	require.NoError(t, err)

	secondQuote, err := QuoteD2SOrder(user.Id, D2SOrderProductPaidRevoke, license.ID, "stripe", now+3)
	require.NoError(t, err)
	_, err = CreateD2SOrder(user.Id, secondQuote, 0, "paid-revoke-2", now+3)
	assert.ErrorIs(t, err, ErrD2SPaidRevokePending)

	_, err = ProcessD2SPaymentEvent("stripe", "paid-revoke-cancel", first.ID, "canceled", quote.AmountMinor, "USD", "paid-revoke-cancel-payload", now+4)
	require.NoError(t, err)
	second, err := CreateD2SOrder(user.Id, secondQuote, 0, "paid-revoke-2", now+5)
	require.NoError(t, err)
	assert.NotEqual(t, first.ID, second.ID)
}

func TestD2SProductOrdersFulfillRevokeAndOfflineExtension(t *testing.T) {
	useD2STestDB(t)
	user := createD2STestUser(t, "d2s-product-orders")
	const now = int64(2_000_380_000)
	licenses, err := ListD2SLicenses(user.Id, now)
	require.NoError(t, err)
	device := strings.Repeat("f", 64)
	license, err := BindD2SLicense(user.Id, licenses[0].ID, device, 1, now)
	require.NoError(t, err)

	revokeQuote, err := QuoteD2SOrder(user.Id, D2SOrderProductPaidRevoke, license.ID, "stripe", now+1)
	require.NoError(t, err)
	revokeOrder, err := CreateD2SOrder(user.Id, revokeQuote, 0, "paid-revoke-fulfill", now+1)
	require.NoError(t, err)
	paidRevoke, err := ProcessD2SPaymentEvent("stripe", "paid-revoke-paid", revokeOrder.ID, "paid", revokeQuote.AmountMinor, "USD", "paid-revoke-paid-payload", now+2)
	require.NoError(t, err)
	assert.Equal(t, D2SOrderPaid, paidRevoke.Status)

	var revokedLicense D2SLicense
	require.NoError(t, DB.First(&revokedLicense, "id = ?", license.ID).Error)
	assert.Empty(t, revokedLicense.DeviceHash)
	assert.Equal(t, D2SLicenseModeUnbound, revokedLicense.Mode)
	var bindingCount int64
	require.NoError(t, DB.Model(&D2SDeviceBinding{}).Where("license_id = ?", license.ID).Count(&bindingCount).Error)
	assert.Zero(t, bindingCount)
	require.NoError(t, DB.Model(&D2SLicense{}).Where("id = ?", license.ID).Update("expires_at", now+2).Error)
	_, err = QuoteD2SOrder(user.Id, D2SOrderProductOfflineExtension, license.ID, "stripe", now+3)
	assert.ErrorIs(t, err, ErrD2SLicenseUnavailable)
	require.NoError(t, DB.Model(&D2SLicense{}).Where("id = ?", license.ID).Update("expires_at", 0).Error)

	previousOfflineUntil := now - 10
	require.NoError(t, DB.Model(&D2SLicense{}).Where("id = ?", license.ID).Updates(map[string]any{
		"offline_valid_until": previousOfflineUntil,
	}).Error)
	previousCNPrice, hadCNPrice := os.LookupEnv("D2S_OFFLINE_EXTENSION_USD_MINOR")
	require.NoError(t, os.Setenv("D2S_OFFLINE_EXTENSION_USD_MINOR", "499"))
	t.Cleanup(func() {
		if hadCNPrice {
			_ = os.Setenv("D2S_OFFLINE_EXTENSION_USD_MINOR", previousCNPrice)
		} else {
			_ = os.Unsetenv("D2S_OFFLINE_EXTENSION_USD_MINOR")
		}
	})

	extensionQuote, err := QuoteD2SOrder(user.Id, D2SOrderProductOfflineExtension, license.ID, "stripe", now+3)
	require.NoError(t, err)
	assert.Equal(t, int64(499), extensionQuote.AmountMinor)
	extensionOrder, err := CreateD2SOrder(user.Id, extensionQuote, 0, "offline-extension-fulfill", now+3)
	require.NoError(t, err)
	paidExtension, err := ProcessD2SPaymentEvent("stripe", "offline-extension-paid", extensionOrder.ID, "paid", extensionQuote.AmountMinor, "USD", "offline-extension-paid-payload", now+4)
	require.NoError(t, err)
	assert.Equal(t, D2SOrderPaid, paidExtension.Status)

	require.NoError(t, DB.First(&revokedLicense, "id = ?", license.ID).Error)
	assert.GreaterOrEqual(t, revokedLicense.OfflineValidUntil, now+30*86400)
}

func TestD2SWithdrawalReviewRequiresReservedBalance(t *testing.T) {
	useD2STestDB(t)
	user := createD2STestUser(t, "d2s-withdrawal-review-reserve")
	const now = int64(2_000_415_000)
	profile, err := EnsureD2SProfileAndTrial(user.Id, now)
	require.NoError(t, err)
	require.NoError(t, DB.Model(profile).Updates(map[string]any{"region": D2SRegionCN, "region_locked_at": now}).Error)
	require.NoError(t, DB.Create(&D2SBalanceAccount{
		ID: "withdrawal-review-account", UserID: user.Id, Currency: "CNY", AvailableMinor: 5000, UpdatedAt: now,
	}).Error)
	request := &D2SWithdrawalRequest{
		ID: "withdrawal-without-reserve", UserID: user.Id, Currency: "CNY", AmountMinor: 5000,
		AlipayAccount: "user@example.com", RealName: "Test User", Status: "pending", CreatedAt: now, UpdatedAt: now,
	}
	require.NoError(t, DB.Create(request).Error)

	_, err = ReviewD2SWithdrawal(10, request.ID, "paid", "", now+1)
	assert.ErrorIs(t, err, ErrD2SOrderState)
	var persisted D2SWithdrawalRequest
	require.NoError(t, DB.First(&persisted, "id = ?", request.ID).Error)
	assert.Equal(t, "pending", persisted.Status)
}

func TestD2SFullBalanceOrderSettlesAtomically(t *testing.T) {
	useD2STestDB(t)
	user := createD2STestUser(t, "d2s-balance")
	const now = int64(2_000_400_000)
	_, err := EnsureD2SProfileAndTrial(user.Id, now)
	require.NoError(t, err)
	require.NoError(t, DB.Model(&D2SUserProfile{}).Where("user_id = ?", user.Id).Updates(map[string]any{"region": D2SRegionCN, "region_locked_at": now}).Error)
	account := D2SBalanceAccount{ID: "balance-account", UserID: user.Id, Currency: "CNY", AvailableMinor: 9900, UpdatedAt: now}
	require.NoError(t, DB.Create(&account).Error)
	quote, err := QuoteD2SOrder(user.Id, D2SOrderProductLicense, "", D2SProviderBalance, now)
	require.NoError(t, err)

	order, err := CreateD2SOrder(user.Id, quote, 9900, "balance-order", now)
	require.NoError(t, err)
	assert.Equal(t, D2SOrderPaid, order.Status)
	require.NoError(t, DB.First(&account, "id = ?", account.ID).Error)
	assert.Zero(t, account.AvailableMinor)
	assert.Zero(t, account.ReservedMinor)
	var transactions []D2SBalanceTransaction
	require.NoError(t, DB.Where("user_id = ?", user.Id).Find(&transactions).Error)
	assert.Len(t, transactions, 2)
}

func TestD2SExternalProviderCannotUseFullBalancePayment(t *testing.T) {
	useD2STestDB(t)
	user := createD2STestUser(t, "d2s-balance-region")
	const now = int64(2_000_410_000)
	_, err := EnsureD2SProfileAndTrial(user.Id, now)
	require.NoError(t, err)
	account := D2SBalanceAccount{ID: "balance-region-account", UserID: user.Id, Currency: "USD", AvailableMinor: 2990, UpdatedAt: now}
	require.NoError(t, DB.Create(&account).Error)
	quote, err := QuoteD2SOrder(user.Id, D2SOrderProductLicense, "", "stripe", now)
	require.NoError(t, err)
	_, err = CreateD2SOrder(user.Id, quote, 2990, "balance-region-order", now+1)
	assert.ErrorIs(t, err, ErrD2SOrderInvalid)
	_, err = QuoteD2SOrder(user.Id, D2SOrderProductLicense, "", D2SProviderBalance, now+1)
	assert.ErrorIs(t, err, ErrD2SRegionMismatch)
}

func TestD2SBalanceProviderCompletesFullPaymentInLockedRegion(t *testing.T) {
	useD2STestDB(t)
	user := createD2STestUser(t, "d2s-balance-provider")
	const now = int64(2_000_920_000)
	profile, err := EnsureD2SProfileAndTrial(user.Id, now)
	require.NoError(t, err)
	require.NoError(t, DB.Model(profile).Updates(map[string]any{"region": D2SRegionINTL, "region_locked_at": now}).Error)
	require.NoError(t, DB.Create(&D2SBalanceAccount{ID: "balance-provider-account", UserID: user.Id, Currency: "USD", AvailableMinor: 2990, UpdatedAt: now}).Error)

	quote, err := QuoteD2SOrder(user.Id, D2SOrderProductLicense, "", D2SProviderBalance, now+1)
	require.NoError(t, err)
	assert.Equal(t, D2SRegionINTL, quote.Region)
	order, err := CreateD2SOrder(user.Id, quote, quote.AmountMinor, "balance-provider-order", now+2)
	require.NoError(t, err)
	assert.Equal(t, D2SOrderPaid, order.Status)
	assert.NotEmpty(t, order.LicenseID)

	var account D2SBalanceAccount
	require.NoError(t, DB.First(&account, "id = ?", "balance-provider-account").Error)
	assert.Zero(t, account.AvailableMinor)
	assert.Zero(t, account.ReservedMinor)
}

func TestD2SUserWithoutVerifiedEmailCannotReceiveTrial(t *testing.T) {
	useD2STestDB(t)
	user := User{Username: "d2s-no-email", Password: "unused-hash", Role: common.RoleCommonUser, Status: common.UserStatusEnabled, Group: "default", AuthVersion: 1}
	require.NoError(t, DB.Create(&user).Error)
	_, err := ListD2SLicenses(user.Id, 2_000_500_000)
	assert.True(t, errors.Is(err, ErrD2SEmailVerificationRequired))
}

func TestD2SUserWithUnverifiedEmailCannotReceiveTrialWhenVerificationEnabled(t *testing.T) {
	useD2STestDB(t)
	previous := common.EmailVerificationEnabled
	common.EmailVerificationEnabled = true
	t.Cleanup(func() { common.EmailVerificationEnabled = previous })
	user := User{Username: "d2s-unverified-email", Password: "unused-hash", Email: "unverified@example.com", Role: common.RoleCommonUser, Status: common.UserStatusEnabled, Group: "default", AuthVersion: 1}
	require.NoError(t, DB.Create(&user).Error)
	_, err := ListD2SLicenses(user.Id, 2_000_510_000)
	assert.ErrorIs(t, err, ErrD2SEmailVerificationRequired)
}

func TestD2SVerifiedEmailCanReceiveTrialWhenVerificationEnabled(t *testing.T) {
	useD2STestDB(t)
	previous := common.EmailVerificationEnabled
	common.EmailVerificationEnabled = true
	t.Cleanup(func() { common.EmailVerificationEnabled = previous })
	user := User{Username: "d2s-verified-email", Password: "unused-hash", Email: "verified@example.com", EmailVerifiedAt: 2_000_510_000, Role: common.RoleCommonUser, Status: common.UserStatusEnabled, Group: "default", AuthVersion: 1}
	require.NoError(t, DB.Create(&user).Error)
	require.NoError(t, DB.Create(&D2SUserProfile{UserID: user.Id, EmailVerified: false, CreatedAt: 2_000_510_000, UpdatedAt: 2_000_510_000}).Error)
	licenses, err := ListD2SLicenses(user.Id, 2_000_510_001)
	require.NoError(t, err)
	require.Len(t, licenses, 1)
	assert.Equal(t, D2SLicenseKindTrial, licenses[0].Kind)
	var profile D2SUserProfile
	require.NoError(t, DB.First(&profile, "user_id = ?", user.Id).Error)
	assert.True(t, profile.EmailVerified)
}

func TestBindEmailPersistsVerificationTimestamp(t *testing.T) {
	useD2STestDB(t)
	user := createD2STestUser(t, "d2s-email-bind")
	user.Email = ""
	require.NoError(t, DB.Model(&User{}).Where("id = ?", user.Id).Update("email", "").Error)

	require.NoError(t, BindEmailToUser(&user, "bound@example.com"))
	var persisted User
	require.NoError(t, DB.First(&persisted, "id = ?", user.Id).Error)
	assert.Equal(t, "bound@example.com", persisted.Email)
	assert.Positive(t, persisted.EmailVerifiedAt)
}

func TestManualUnbindAllowsOnlyOnePendingRequest(t *testing.T) {
	useD2STestDB(t)
	user := createD2STestUser(t, "d2s-manual-unbind")
	const now = int64(2_000_520_000)
	licenses, err := ListD2SLicenses(user.Id, now)
	require.NoError(t, err)
	device := strings.Repeat("a", 64)
	license, err := BindD2SLicense(user.Id, licenses[0].ID, device, 1, now)
	require.NoError(t, err)
	_, err = ChangeD2SLicenseMode(user.Id, license.ID, device, D2SLicenseModePermanent, "PERMANENT", 7, now+1)
	require.NoError(t, err)
	_, err = CreateD2SManualUnbind(user.Id, license.ID, "hardware replacement", "ticket-1", now+2)
	require.NoError(t, err)
	_, err = CreateD2SManualUnbind(user.Id, license.ID, "duplicate request", "ticket-2", now+3)
	assert.ErrorIs(t, err, ErrD2SManualUnbindPending)
}

func TestD2SPaymentReconciliationReportsLocalAnomalies(t *testing.T) {
	useD2STestDB(t)
	const now = int64(2_000_600_000)
	require.NoError(t, DB.Create(&D2SOrder{
		ID: "reconcile-pending", UserID: 1, Product: D2SOrderProductLicense, Provider: "stripe",
		Region: D2SRegionINTL, Currency: "USD", AmountMinor: 2990, GatewayMinor: 2990,
		Status: D2SOrderPending, CreatedAt: now - 50, ExpiresAt: now - 1,
	}).Error)
	require.NoError(t, DB.Create(&D2SOrder{
		ID: "reconcile-paid-without-event", UserID: 1, Product: D2SOrderProductLicense, Provider: "stripe",
		Region: D2SRegionINTL, Currency: "USD", AmountMinor: 2990, GatewayMinor: 2990,
		Status: D2SOrderPaid, IdempotencyKey: "reconcile-paid-key", CreatedAt: now - 30, ExpiresAt: now + 1700,
	}).Error)
	require.NoError(t, DB.Create(&D2SOrder{
		ID: "reconcile-balance-paid", UserID: 1, Product: D2SOrderProductLicense, Provider: D2SProviderBalance,
		Region: D2SRegionINTL, Currency: "USD", AmountMinor: 2990, GatewayMinor: 0,
		Status: D2SOrderPaid, IdempotencyKey: "reconcile-balance-key", CreatedAt: now - 20, ExpiresAt: now + 1700,
	}).Error)
	require.NoError(t, DB.Create(&D2SPaymentEvent{
		ID: "reconcile-orphan-event", Provider: "stripe", ProviderEventID: "reconcile-event",
		OrderID: "missing-order", EventType: "paid", AmountMinor: 2990, Currency: "USD",
		PayloadHash: "reconcile-hash", ProcessedAt: now - 40,
	}).Error)

	report, err := ReconcileD2SPayments(now-100, now+100)
	require.NoError(t, err)
	assert.Equal(t, 3, report.Orders)
	assert.Equal(t, 1, report.PaymentEvents)
	assert.Equal(t, 1, report.PendingExpired)
	require.Len(t, report.Mismatches, 3)
	assert.Equal(t, "pending_expired", report.Mismatches[0].Kind)
	assert.Equal(t, "orphan_payment_event", report.Mismatches[1].Kind)
	assert.Equal(t, "paid_order_without_payment_event", report.Mismatches[2].Kind)
	_, err = ReconcileD2SPayments(now, now)
	assert.Error(t, err)
}

func TestD2SPaymentReconciliationIncludesOlderOrdersReferencedByEvents(t *testing.T) {
	useD2STestDB(t)
	const now = int64(2_000_700_000)
	require.NoError(t, DB.Create(&D2SOrder{
		ID: "reconcile-old-paid", UserID: 1, Product: D2SOrderProductLicense, Provider: "stripe",
		Region: D2SRegionINTL, Currency: "USD", AmountMinor: 2990, GatewayMinor: 2990,
		Status: D2SOrderPaid, CreatedAt: now - 86400, ExpiresAt: now + 1700,
	}).Error)
	require.NoError(t, DB.Create(&D2SPaymentEvent{
		ID: "reconcile-reversal-event", Provider: "stripe", ProviderEventID: "reconcile-reversal",
		OrderID: "reconcile-old-paid", EventType: "reversed", AmountMinor: 2990, Currency: "USD",
		PayloadHash: "reconcile-reversal-hash", ProcessedAt: now + 10,
	}).Error)

	report, err := ReconcileD2SPayments(now, now+100)
	require.NoError(t, err)
	assert.Equal(t, 1, report.Orders)
	assert.Equal(t, 1, report.PaymentEvents)
	assert.Equal(t, 1, report.PaidOrders)
	assert.Contains(t, report.Mismatches, D2SReconciliationMismatch{
		OrderID: "reconcile-old-paid", ProviderEventID: "reconcile-reversal", Kind: "reversal_not_settled",
	})
}

func TestD2SAdditionalReversalEventsAreRecordedAfterChargeback(t *testing.T) {
	useD2STestDB(t)
	const now = int64(2_000_800_000)
	require.NoError(t, DB.Create(&D2SOrder{
		ID: "reversal-chain-order", UserID: 1, Product: D2SOrderProductLicense, Provider: "stripe",
		Region: D2SRegionINTL, Currency: "USD", AmountMinor: 2990, GatewayMinor: 2990,
		Status: D2SOrderPaid, CreatedAt: now - 100, ExpiresAt: now + 1700,
	}).Error)
	require.NoError(t, DB.Create(&D2SPaymentEvent{
		ID: "reversal-chain-paid", Provider: "stripe", ProviderEventID: "reversal-chain-paid",
		OrderID: "reversal-chain-order", EventType: "paid", AmountMinor: 2990, Currency: "USD",
		PayloadHash: "paid-hash", ProcessedAt: now - 90,
	}).Error)

	_, err := ProcessD2SPaymentEvent("stripe", "reversal-chain-refund", "reversal-chain-order", "reversed", 2990, "USD", "refund-hash", now)
	require.NoError(t, err)
	_, err = ProcessD2SPaymentEvent("stripe", "reversal-chain-dispute", "reversal-chain-order", "chargeback", 2990, "USD", "dispute-hash", now+1)
	require.NoError(t, err)

	var order D2SOrder
	require.NoError(t, DB.First(&order, "id = ?", "reversal-chain-order").Error)
	assert.Equal(t, D2SOrderChargeback, order.Status)
	var eventCount int64
	require.NoError(t, DB.Model(&D2SPaymentEvent{}).Where("order_id = ?", order.ID).Count(&eventCount).Error)
	assert.EqualValues(t, 3, eventCount)
}
