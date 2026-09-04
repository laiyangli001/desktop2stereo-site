package model

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

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

func TestD2SFullBalanceOrderSettlesAtomically(t *testing.T) {
	useD2STestDB(t)
	user := createD2STestUser(t, "d2s-balance")
	const now = int64(2_000_400_000)
	_, err := EnsureD2SProfileAndTrial(user.Id, now)
	require.NoError(t, err)
	require.NoError(t, DB.Model(&D2SUserProfile{}).Where("user_id = ?", user.Id).Updates(map[string]any{"region": D2SRegionCN, "region_locked_at": now}).Error)
	account := D2SBalanceAccount{ID: "balance-account", UserID: user.Id, Currency: "CNY", AvailableMinor: 9900, UpdatedAt: now}
	require.NoError(t, DB.Create(&account).Error)
	quote, err := QuoteD2SOrder(user.Id, D2SOrderProductLicense, "", "epay", now)
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

func TestD2SUserWithoutVerifiedEmailCannotReceiveTrial(t *testing.T) {
	useD2STestDB(t)
	user := User{Username: "d2s-no-email", Password: "unused-hash", Role: common.RoleCommonUser, Status: common.UserStatusEnabled, Group: "default", AuthVersion: 1}
	require.NoError(t, DB.Create(&user).Error)
	_, err := ListD2SLicenses(user.Id, 2_000_500_000)
	assert.True(t, errors.Is(err, ErrD2SEmailVerificationRequired))
}
