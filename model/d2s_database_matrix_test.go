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
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestD2SIdentityVerificationIsNotAutoMigrated(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(d2sCoreMigrationModels()...))
	assert.False(t, db.Migrator().HasTable(&D2SIdentityVerification{}), "identity verification is not an active product feature")
}

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
				&D2SSigningKey{}, &D2SUserActionLog{}, &D2SRegistrationRecord{},
				&D2SAdminOperationAudit{}, &D2SAuditRetentionRun{}, &D2SAdminExportApproval{}, &D2SUserSecurityControl{}, &D2SSupportTicket{},
			}
			require.NoError(t, db.AutoMigrate(models...))
			require.NoError(t, db.AutoMigrate(models...), "migration must be idempotent")

			suffix := strconv.FormatInt(time.Now().UnixNano(), 10)
			shortSuffix := suffix[len(suffix)-10:]
			user := User{Username: "m-" + test.name + "-" + shortSuffix, Password: "unused-hash", EmailVerifiedAt: time.Now().UTC().Unix(), Email: "matrix-" + test.name + "-" + suffix + "@example.com", AffCode: "m-" + test.name + "-" + shortSuffix, Role: common.RoleCommonUser, Status: common.UserStatusEnabled, Group: "default", AuthVersion: 1}
			require.NoError(t, db.Create(&user).Error)

			lockTime := time.Now().UTC().Unix()
			lockControl := &D2SUserSecurityControl{
				UserID: user.Id, LoginBlocked: true, PurchaseBlocked: true,
				Reason: "matrix emergency lock", UpdatedAt: lockTime, UpdatedBy: 1,
			}
			lockAudit := &D2SAdminOperationAudit{
				OperatorID: 1, OperatorEmail: "matrix-admin@example.com", OperationType: "EMERGENCY_LOCK",
				OperationTime: lockTime, TargetResource: strconv.Itoa(user.Id), SourceIP: "192.0.2.10",
				Result: "SUCCESS", DetailsJSON: `{"login_blocked":true,"purchase_blocked":true}`,
			}
			require.NoError(t, SetD2SUserSecurityControlWithAudit(lockControl, lockAudit))
			loginBlocked, err := IsD2SUserLoginBlocked(user.Id)
			require.NoError(t, err)
			assert.True(t, loginBlocked)
			purchaseBlocked, err := IsD2SUserPurchaseBlocked(user.Id)
			require.NoError(t, err)
			assert.True(t, purchaseBlocked)

			unlockTime := lockTime + 1
			unlockControl := &D2SUserSecurityControl{
				UserID: user.Id, LoginBlocked: false, PurchaseBlocked: false,
				Reason: "matrix emergency unlock", UpdatedAt: unlockTime, UpdatedBy: 1,
			}
			unlockAudit := &D2SAdminOperationAudit{
				OperatorID: 1, OperatorEmail: "matrix-admin@example.com", OperationType: "EMERGENCY_UNLOCK",
				OperationTime: unlockTime, TargetResource: strconv.Itoa(user.Id), SourceIP: "192.0.2.10",
				Result: "SUCCESS", DetailsJSON: `{"login_blocked":false,"purchase_blocked":false}`,
			}
			require.NoError(t, SetD2SUserSecurityControlWithAudit(unlockControl, unlockAudit))
			loginBlocked, err = IsD2SUserLoginBlocked(user.Id)
			require.NoError(t, err)
			assert.False(t, loginBlocked)
			purchaseBlocked, err = IsD2SUserPurchaseBlocked(user.Id)
			require.NoError(t, err)
			assert.False(t, purchaseBlocked)

			var emergencyAudits []D2SAdminOperationAudit
			require.NoError(t, db.Where("target_resource = ? AND operation_type IN ?", strconv.Itoa(user.Id), []string{"EMERGENCY_LOCK", "EMERGENCY_UNLOCK"}).Order("operation_time ASC").Find(&emergencyAudits).Error)
			require.Len(t, emergencyAudits, 2)
			assert.Equal(t, "EMERGENCY_LOCK", emergencyAudits[0].OperationType)
			assert.Equal(t, "EMERGENCY_UNLOCK", emergencyAudits[1].OperationType)
			assert.Equal(t, "SUCCESS", emergencyAudits[0].Result)
			assert.Equal(t, "SUCCESS", emergencyAudits[1].Result)
			assert.Equal(t, "192.0.2.10", emergencyAudits[0].SourceIP)

			licenses, err := ListD2SLicenses(user.Id, 2_000_700_000)
			require.NoError(t, err)
			require.Len(t, licenses, 1)
			bound, err := BindD2SLicense(user.Id, licenses[0].ID, fmt.Sprintf("%064x", uint64(time.Now().UnixNano())), 1, 2_000_700_001)
			require.NoError(t, err)
			assert.Equal(t, D2SLicenseModeOnline, bound.Mode)

			ticket := &D2SSupportTicket{
				UserID: user.Id, Category: "ACCOUNT", ContactEmail: user.Email,
				Subject: "Database matrix support request", Message: "Verify support ticket persistence across configured databases.",
			}
			require.NoError(t, CreateD2SSupportTicket(ticket))
			userTickets, userTicketCount, err := ListD2SUserSupportTickets(user.Id, 1, 10)
			require.NoError(t, err)
			require.EqualValues(t, 1, userTicketCount)
			require.Len(t, userTickets, 1)
			assert.Equal(t, ticket.ID, userTickets[0].ID)
			assert.Equal(t, "Database matrix support request", userTickets[0].Subject)

			updatedTicket, err := UpdateD2SSupportTicketWithAudit(
				ticket.ID, D2SSupportInProgress, "We received your request.", 1,
				"matrix-admin@example.com", "192.0.2.10", `{"status":"IN_PROGRESS"}`,
			)
			require.NoError(t, err)
			assert.Equal(t, D2SSupportInProgress, updatedTicket.Status)
			userTickets, _, err = ListD2SUserSupportTickets(user.Id, 1, 10)
			require.NoError(t, err)
			require.Len(t, userTickets, 1)
			assert.Equal(t, "We received your request.", userTickets[0].AdminReply)
			var supportAudit D2SAdminOperationAudit
			require.NoError(t, db.Where("operation_type = ? AND target_resource = ?", "UPDATE_SUPPORT_TICKET", "d2s_support_tickets:"+ticket.ID).First(&supportAudit).Error)
			assert.Equal(t, "SUCCESS", supportAudit.Result)
			assert.Equal(t, 1, supportAudit.OperatorID)
			assert.Equal(t, "192.0.2.10", supportAudit.SourceIP)

			auditTime := time.Now().UTC().Unix()
			var profile D2SUserProfile
			require.NoError(t, db.Where("user_id = ?", user.Id).First(&profile).Error)
			region := D2SRegionCN
			if profile.Region == region {
				region = D2SRegionINTL
			}
			regionAudit := &D2SAdminOperationAudit{
				OperatorID: 1, OperatorEmail: "matrix-admin@example.com", OperationType: "SET_USER_REGION",
				TargetResource: strconv.Itoa(user.Id), SourceIP: "192.0.2.10",
				Result: "SUCCESS", DetailsJSON: `{"region":"` + region + `"}`,
			}
			updatedProfile, err := SetD2SUserRegionWithAudit(1, user.Id, region, auditTime, regionAudit)
			require.NoError(t, err)
			assert.Equal(t, region, updatedProfile.Region)
			var regionAuditRecord D2SAdminOperationAudit
			require.NoError(t, db.Where("operation_type = ? AND target_resource = ?", "SET_USER_REGION", strconv.Itoa(user.Id)).First(&regionAuditRecord).Error)
			assert.Equal(t, "SUCCESS", regionAuditRecord.Result)
			assert.Equal(t, 1, regionAuditRecord.OperatorID)
			assert.Equal(t, "192.0.2.10", regionAuditRecord.SourceIP)

			withdrawalID := "matrix-withdrawal-review-" + suffix
			withdrawalAccount := D2SBalanceAccount{
				ID: "matrix-withdrawal-account-" + suffix, UserID: user.Id, Currency: "CNY",
				ReservedMinor: 5000, UpdatedAt: auditTime,
			}
			require.NoError(t, db.Create(&withdrawalAccount).Error)
			require.NoError(t, db.Create(&D2SWithdrawalRequest{
				ID: withdrawalID, UserID: user.Id, Currency: "CNY", AmountMinor: 5000,
				AlipayAccount: "matrix@example.com", RealName: "Matrix Test", Status: "pending",
				CreatedAt: auditTime, UpdatedAt: auditTime,
			}).Error)
			withdrawalAudit := &D2SAdminOperationAudit{
				OperatorID: 1, OperatorEmail: "matrix-admin@example.com", OperationType: "REVIEW_WITHDRAWAL",
				TargetResource: "d2s_withdrawal_requests:" + withdrawalID, SourceIP: "192.0.2.10",
				Result: "SUCCESS", DetailsJSON: `{"status":"rejected"}`,
			}
			updatedWithdrawal, err := ReviewD2SWithdrawalWithAudit(1, withdrawalID, "rejected", "matrix review", auditTime, withdrawalAudit)
			require.NoError(t, err)
			assert.Equal(t, "rejected", updatedWithdrawal.Status)
			var updatedWithdrawalAccount D2SBalanceAccount
			require.NoError(t, db.First(&updatedWithdrawalAccount, "id = ?", withdrawalAccount.ID).Error)
			assert.EqualValues(t, 0, updatedWithdrawalAccount.ReservedMinor)
			assert.EqualValues(t, 5000, updatedWithdrawalAccount.AvailableMinor)
			var withdrawalAuditRecord D2SAdminOperationAudit
			require.NoError(t, db.Where("operation_type = ? AND target_resource = ?", "REVIEW_WITHDRAWAL", withdrawalAudit.TargetResource).First(&withdrawalAuditRecord).Error)
			assert.Equal(t, "SUCCESS", withdrawalAuditRecord.Result)
			assert.Equal(t, 1, withdrawalAuditRecord.OperatorID)
			assert.Equal(t, "192.0.2.10", withdrawalAuditRecord.SourceIP)

			unbindID := "matrix-unbind-review-" + suffix
			require.NoError(t, db.Create(&D2SManualUnbindRequest{
				ID: unbindID, LicenseID: licenses[0].ID, UserID: user.Id, Reason: "matrix review",
				Status: "pending", CreatedAt: auditTime, UpdatedAt: auditTime,
			}).Error)
			unbindAudit := &D2SAdminOperationAudit{
				OperatorID: 1, OperatorEmail: "matrix-admin@example.com", OperationType: "REVIEW_MANUAL_UNBIND",
				TargetResource: "d2s_manual_unbind_requests:" + unbindID, SourceIP: "192.0.2.10",
				Result: "SUCCESS", DetailsJSON: `{"status":"rejected"}`,
			}
			updatedUnbind, err := ReviewD2SManualUnbindWithAudit(1, unbindID, "rejected", "matrix review", auditTime, unbindAudit)
			require.NoError(t, err)
			assert.Equal(t, "rejected", updatedUnbind.Status)
			var unbindAuditRecord D2SAdminOperationAudit
			require.NoError(t, db.Where("operation_type = ? AND target_resource = ?", "REVIEW_MANUAL_UNBIND", unbindAudit.TargetResource).First(&unbindAuditRecord).Error)
			assert.Equal(t, "SUCCESS", unbindAuditRecord.Result)
			assert.Equal(t, 1, unbindAuditRecord.OperatorID)
			assert.Equal(t, "192.0.2.10", unbindAuditRecord.SourceIP)

			reconcileNow := time.Now().UTC().Unix()
			order := D2SOrder{
				ID: "matrix-reconciliation-" + suffix, UserID: user.Id, Product: D2SOrderProductLicense,
				Provider: "stripe", Region: D2SRegionINTL, Currency: "USD", AmountMinor: 2990,
				GatewayMinor: 2990, Status: D2SOrderPending, IdempotencyKey: "matrix-reconciliation-" + suffix,
				CreatedAt: reconcileNow, ExpiresAt: reconcileNow + 20,
			}
			require.NoError(t, db.Create(&order).Error)
			reconciliation, err := ReconcileD2SPayments(reconcileNow-1, reconcileNow+30)
			require.NoError(t, err)
			assert.Contains(t, reconciliation.Mismatches, D2SReconciliationMismatch{OrderID: order.ID, Kind: "pending_expired"})
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
				&D2SSigningKey{}, &D2SUserActionLog{}, &D2SRegistrationRecord{},
				&D2SAdminOperationAudit{}, &D2SAuditRetentionRun{}, &D2SUserSecurityControl{},
			}
			require.NoError(t, db.AutoMigrate(models...))
			suffix := time.Now().UnixNano()
			user := User{Username: "concurrent-" + test.name + "-" + strconv.FormatInt(suffix, 10), Password: "unused-hash", EmailVerifiedAt: time.Now().UTC().Unix(), Email: "concurrent-" + test.name + "-" + strconv.FormatInt(suffix, 10) + "@example.com", AffCode: "concurrent-" + strconv.FormatInt(suffix, 10), Role: common.RoleCommonUser, Status: common.UserStatusEnabled, Group: "default", AuthVersion: 1}
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

			regionUser := User{Username: "region-" + test.name + "-" + strconv.FormatInt(suffix, 10), Password: "unused-hash", EmailVerifiedAt: time.Now().UTC().Unix(), Email: "region-" + test.name + "-" + strconv.FormatInt(suffix, 10) + "@example.com", AffCode: "region-" + strconv.FormatInt(suffix, 10), Role: common.RoleCommonUser, Status: common.UserStatusEnabled, Group: "default", AuthVersion: 1}
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

			balanceUser := User{Username: "balance-" + test.name + "-" + strconv.FormatInt(suffix, 10), Password: "unused-hash", EmailVerifiedAt: time.Now().UTC().Unix(), Email: "balance-" + test.name + "-" + strconv.FormatInt(suffix, 10) + "@example.com", AffCode: "balance-" + strconv.FormatInt(suffix, 10), Role: common.RoleCommonUser, Status: common.UserStatusEnabled, Group: "default", AuthVersion: 1}
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

			unbindUser := User{Username: "unbind-" + test.name + "-" + strconv.FormatInt(suffix, 10), Password: "unused-hash", EmailVerifiedAt: time.Now().UTC().Unix(), Email: "unbind-" + test.name + "-" + strconv.FormatInt(suffix, 10) + "@example.com", AffCode: "unbind-" + strconv.FormatInt(suffix, 10), Role: common.RoleCommonUser, Status: common.UserStatusEnabled, Group: "default", AuthVersion: 1}
			require.NoError(t, db.Create(&unbindUser).Error)
			assertOnlyOneConcurrentManualUnbindApproval(t, unbindUser.Id)
		})
	}
}
