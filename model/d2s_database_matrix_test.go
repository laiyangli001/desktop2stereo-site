package model

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestD2SSchemaConfiguredDatabases(t *testing.T) {
	tests := []struct {
		name    string
		envName string
		dbType  common.DatabaseType
		open    func(string) (*gorm.DB, error)
	}{
		{name: "mysql", envName: "D2S_TEST_MYSQL_DSN", dbType: common.DatabaseTypeMySQL, open: func(dsn string) (*gorm.DB, error) {
			return gorm.Open(mysql.Open(dsn), &gorm.Config{})
		}},
		{name: "postgres", envName: "D2S_TEST_POSTGRES_DSN", dbType: common.DatabaseTypePostgreSQL, open: func(dsn string) (*gorm.DB, error) {
			return gorm.Open(postgres.Open(dsn), &gorm.Config{})
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			dsn := strings.TrimSpace(os.Getenv(test.envName))
			if dsn == "" {
				t.Skip(test.envName + " is not configured")
			}
			db, err := test.open(dsn)
			require.NoError(t, err)
			sqlDB, err := db.DB()
			require.NoError(t, err)
			t.Cleanup(func() { _ = sqlDB.Close() })
			previousDB, previousLogDB := DB, LOG_DB
			previousMainType, previousLogType := common.MainDatabaseType(), common.LogDatabaseType()
			DB, LOG_DB = db, db
			common.SetDatabaseTypes(test.dbType, test.dbType)
			t.Cleanup(func() {
				DB, LOG_DB = previousDB, previousLogDB
				common.SetDatabaseTypes(previousMainType, previousLogType)
			})

			models := []any{
				&User{}, &D2SUserProfile{}, &D2SLicense{}, &D2SDeviceBinding{}, &D2SLicenseEvent{},
				&D2SDeviceCode{}, &D2SOfflineEntitlement{}, &D2SOnlineLease{}, &D2SFreeRevokeCooldown{},
				&D2SPaidRevokeQuota{}, &D2SManualUnbindRequest{}, &D2SOrder{}, &D2SPaymentEvent{},
				&D2SBalanceAccount{}, &D2SBalanceTransaction{}, &D2SInviteReward{}, &D2SWithdrawalRequest{},
				&D2SSigningKey{},
			}
			require.NoError(t, db.AutoMigrate(models...))
			require.NoError(t, db.AutoMigrate(models...), "migration must be idempotent")

			suffix := strconv.FormatInt(time.Now().UnixNano(), 10)
			shortSuffix := suffix[len(suffix)-10:]
			user := User{Username: "m-" + test.name + "-" + shortSuffix, Password: "unused-hash", Email: "matrix-" + test.name + "-" + suffix + "@example.com", AffCode: "m-" + test.name + "-" + shortSuffix, Role: common.RoleCommonUser, Status: common.UserStatusEnabled, Group: "default", AuthVersion: 1}
			require.NoError(t, db.Create(&user).Error)
			licenses, err := ListD2SLicenses(user.Id, 2_000_700_000)
			require.NoError(t, err)
			require.Len(t, licenses, 1)
			bound, err := BindD2SLicense(user.Id, licenses[0].ID, fmt.Sprintf("%064x", uint64(time.Now().UnixNano())), 1, 2_000_700_001)
			require.NoError(t, err)
			assert.Equal(t, D2SLicenseModeOnline, bound.Mode)
		})
	}
}

func TestD2SConcurrentCriticalPaths(t *testing.T) {
	tests := []struct {
		name    string
		envName string
		dbType  common.DatabaseType
		open    func(string) (*gorm.DB, error)
	}{
		{name: "mysql", envName: "D2S_TEST_MYSQL_DSN", dbType: common.DatabaseTypeMySQL, open: func(dsn string) (*gorm.DB, error) {
			return gorm.Open(mysql.Open(dsn), &gorm.Config{})
		}},
		{name: "postgres", envName: "D2S_TEST_POSTGRES_DSN", dbType: common.DatabaseTypePostgreSQL, open: func(dsn string) (*gorm.DB, error) {
			return gorm.Open(postgres.Open(dsn), &gorm.Config{})
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			dsn := strings.TrimSpace(os.Getenv(test.envName))
			if dsn == "" {
				t.Skip(test.envName + " is not configured")
			}
			db, err := test.open(dsn)
			require.NoError(t, err)
			sqlDB, err := db.DB()
			require.NoError(t, err)
			t.Cleanup(func() { _ = sqlDB.Close() })
			previousDB, previousLogDB := DB, LOG_DB
			previousMainType, previousLogType := common.MainDatabaseType(), common.LogDatabaseType()
			DB, LOG_DB = db, db
			common.SetDatabaseTypes(test.dbType, test.dbType)
			t.Cleanup(func() {
				DB, LOG_DB = previousDB, previousLogDB
				common.SetDatabaseTypes(previousMainType, previousLogType)
			})

			models := []any{
				&User{}, &D2SUserProfile{}, &D2SLicense{}, &D2SDeviceBinding{}, &D2SLicenseEvent{},
				&D2SDeviceCode{}, &D2SOfflineEntitlement{}, &D2SOnlineLease{}, &D2SFreeRevokeCooldown{},
				&D2SPaidRevokeQuota{}, &D2SManualUnbindRequest{}, &D2SOrder{}, &D2SPaymentEvent{},
				&D2SBalanceAccount{}, &D2SBalanceTransaction{}, &D2SInviteReward{}, &D2SWithdrawalRequest{},
				&D2SSigningKey{},
			}
			require.NoError(t, db.AutoMigrate(models...))
			suffix := time.Now().UnixNano()
			user := User{Username: "concurrent-" + test.name + "-" + strconv.FormatInt(suffix, 10), Password: "unused-hash", Email: "concurrent-" + test.name + "-" + strconv.FormatInt(suffix, 10) + "@example.com", AffCode: "concurrent-" + strconv.FormatInt(suffix, 10), Role: common.RoleCommonUser, Status: common.UserStatusEnabled, Group: "default", AuthVersion: 1}
			require.NoError(t, db.Create(&user).Error)
			const now = int64(2_000_800_000)

			const workers = 8
			var wait sync.WaitGroup
			trialErrors := make(chan error, workers)
			for i := 0; i < workers; i++ {
				wait.Add(1)
				go func() {
					defer wait.Done()
					_, callErr := ListD2SLicenses(user.Id, now)
					trialErrors <- callErr
				}()
			}
			wait.Wait()
			close(trialErrors)
			for callErr := range trialErrors {
				require.NoError(t, callErr)
			}
			var trialCount int64
			require.NoError(t, db.Model(&D2SLicense{}).Where("user_id = ? AND kind = ?", user.Id, D2SLicenseKindTrial).Count(&trialCount).Error)
			assert.EqualValues(t, 1, trialCount)

			licenses, err := ListD2SLicenses(user.Id, now+1)
			require.NoError(t, err)
			device := fmt.Sprintf("%064x", uint64(suffix))
			bindErrors := make(chan error, workers)
			for i := 0; i < workers; i++ {
				wait.Add(1)
				go func() {
					defer wait.Done()
					_, callErr := BindD2SLicense(user.Id, licenses[0].ID, device, 1, now+2)
					bindErrors <- callErr
				}()
			}
			wait.Wait()
			close(bindErrors)
			for callErr := range bindErrors {
				require.NoError(t, callErr)
			}

			leaseErrors := make(chan error, workers)
			for i := 0; i < workers; i++ {
				wait.Add(1)
				go func() {
					defer wait.Done()
					_, _, callErr := StartOrRenewD2SOnlineLease(user.Id, licenses[0].ID, device, "", now+3)
					leaseErrors <- callErr
				}()
			}
			wait.Wait()
			close(leaseErrors)
			leaseSuccesses := 0
			for callErr := range leaseErrors {
				if callErr == nil {
					leaseSuccesses++
				} else {
					assert.ErrorIs(t, callErr, ErrD2SLeaseConflict)
				}
			}
			assert.Equal(t, 1, leaseSuccesses)

			quote, err := QuoteD2SOrder(user.Id, D2SOrderProductLicense, "", "stripe", now+4)
			require.NoError(t, err)
			idempotencyKey := "concurrent-order-" + strconv.FormatInt(suffix, 10)
			eventID := "concurrent-event-" + strconv.FormatInt(suffix, 10)
			payloadHash := strings.Repeat("a", 64)
			orderResults := make(chan *D2SOrder, workers)
			orderErrors := make(chan error, workers)
			for i := 0; i < workers; i++ {
				wait.Add(1)
				go func() {
					defer wait.Done()
					order, callErr := CreateD2SOrder(user.Id, quote, 0, idempotencyKey, now+5)
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
			for callErr := range orderErrors {
				require.NoError(t, callErr)
			}
			require.NotEmpty(t, orderID)

			eventErrors := make(chan error, workers)
			for i := 0; i < workers; i++ {
				wait.Add(1)
				go func() {
					defer wait.Done()
					_, callErr := ProcessD2SPaymentEvent("stripe", eventID, orderID, "paid", quote.AmountMinor, quote.Currency, payloadHash, now+6)
					eventErrors <- callErr
				}()
			}
			wait.Wait()
			close(eventErrors)
			for callErr := range eventErrors {
				require.NoError(t, callErr)
			}
			var paymentEventCount int64
			require.NoError(t, db.Model(&D2SPaymentEvent{}).Where("provider = ? AND provider_event_id = ?", "stripe", eventID).Count(&paymentEventCount).Error)
			assert.EqualValues(t, 1, paymentEventCount)

			regionUser := User{Username: "region-" + test.name + "-" + strconv.FormatInt(suffix, 10), Password: "unused-hash", Email: "region-" + test.name + "-" + strconv.FormatInt(suffix, 10) + "@example.com", AffCode: "region-" + strconv.FormatInt(suffix, 10), Role: common.RoleCommonUser, Status: common.UserStatusEnabled, Group: "default", AuthVersion: 1}
			require.NoError(t, db.Create(&regionUser).Error)
			cnQuote, err := QuoteD2SOrder(regionUser.Id, D2SOrderProductLicense, "", "epay", now+7)
			require.NoError(t, err)
			intlQuote, err := QuoteD2SOrder(regionUser.Id, D2SOrderProductLicense, "", "stripe", now+7)
			require.NoError(t, err)
			cnOrder, err := CreateD2SOrder(regionUser.Id, cnQuote, 0, "region-cn-"+strconv.FormatInt(suffix, 10), now+7)
			require.NoError(t, err)
			intlOrder, err := CreateD2SOrder(regionUser.Id, intlQuote, 0, "region-intl-"+strconv.FormatInt(suffix, 10), now+7)
			require.NoError(t, err)
			regionErrors := make(chan error, 2)
			wait.Add(2)
			go func() {
				defer wait.Done()
				_, callErr := ProcessD2SPaymentEvent("epay", "region-cn-event-"+strconv.FormatInt(suffix, 10), cnOrder.ID, "paid", cnQuote.AmountMinor, cnQuote.Currency, "region-cn-payload", now+8)
				regionErrors <- callErr
			}()
			go func() {
				defer wait.Done()
				_, callErr := ProcessD2SPaymentEvent("stripe", "region-intl-event-"+strconv.FormatInt(suffix, 10), intlOrder.ID, "paid", intlQuote.AmountMinor, intlQuote.Currency, "region-intl-payload", now+8)
				regionErrors <- callErr
			}()
			wait.Wait()
			close(regionErrors)
			regionSuccesses := 0
			regionMismatches := 0
			for callErr := range regionErrors {
				if callErr == nil {
					regionSuccesses++
				} else if errors.Is(callErr, ErrD2SRegionMismatch) {
					regionMismatches++
				} else {
					require.NoError(t, callErr)
				}
			}
			assert.Equal(t, 1, regionSuccesses)
			assert.Equal(t, 1, regionMismatches)

			balanceUser := User{Username: "balance-" + test.name + "-" + strconv.FormatInt(suffix, 10), Password: "unused-hash", Email: "balance-" + test.name + "-" + strconv.FormatInt(suffix, 10) + "@example.com", AffCode: "balance-" + strconv.FormatInt(suffix, 10), Role: common.RoleCommonUser, Status: common.UserStatusEnabled, Group: "default", AuthVersion: 1}
			require.NoError(t, db.Create(&balanceUser).Error)
			balanceProfile, err := EnsureD2SProfileAndTrial(balanceUser.Id, now+9)
			require.NoError(t, err)
			require.NoError(t, db.Model(balanceProfile).Updates(map[string]any{"region": D2SRegionCN, "region_locked_at": now + 9}).Error)
			require.NoError(t, db.Create(&D2SBalanceAccount{ID: "concurrent-balance-" + strconv.FormatInt(suffix, 10), UserID: balanceUser.Id, Currency: "CNY", AvailableMinor: 9900, UpdatedAt: now + 9}).Error)
			withdrawalErrors := make(chan error, 2)
			wait.Add(2)
			for i := 0; i < 2; i++ {
				go func() {
					defer wait.Done()
					_, callErr := CreateD2SWithdrawal(balanceUser.Id, 5000, "alipay-account", "Test User", now+10)
					withdrawalErrors <- callErr
				}()
			}
			wait.Wait()
			close(withdrawalErrors)
			withdrawalSuccesses := 0
			for callErr := range withdrawalErrors {
				if callErr == nil {
					withdrawalSuccesses++
				} else {
					assert.ErrorIs(t, callErr, ErrD2SInsufficientBalance)
				}
			}
			assert.Equal(t, 1, withdrawalSuccesses)
		})
	}
}
