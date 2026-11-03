package controller

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupD2SAdminBillingAuditDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:d2s_admin_billing_audit_test?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	previousDB, previousLogDB := model.DB, model.LOG_DB
	previousMainType, previousLogType := common.MainDatabaseType(), common.LogDatabaseType()
	model.DB, model.LOG_DB = db, db
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	t.Cleanup(func() {
		model.DB, model.LOG_DB = previousDB, previousLogDB
		common.SetDatabaseTypes(previousMainType, previousLogType)
		sqlDB, dbErr := db.DB()
		if dbErr == nil {
			_ = sqlDB.Close()
		}
	})
	require.NoError(t, db.AutoMigrate(
		&model.D2SAdminOperationAudit{}, &model.D2SWithdrawalRequest{}, &model.D2SBalanceAccount{},
		&model.D2SBalanceTransaction{}, &model.D2SManualUnbindRequest{}, &model.D2SUserProfile{},
		&model.D2SOrder{}, &model.D2SPaymentEvent{}, &model.D2SLicense{}, &model.D2SSigningKey{},
	))
	return db
}

func d2sAdminBillingAuditContext(method, path, body string) (*gin.Context, *httptest.ResponseRecorder) {
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Set("d2s_admin_id", 77)
	context.Set("d2s_admin_email", "operator@example.com")
	context.Request = httptest.NewRequest(method, path, strings.NewReader(body))
	return context, recorder
}

func TestD2SAdminBillingMutationsAndSensitiveReadsAreAudited(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupD2SAdminBillingAuditDB(t)
	const now int64 = 2_000_500_000
	require.NoError(t, db.Create(&model.D2SBalanceAccount{
		ID: "admin-audit-withdrawal-balance", UserID: 9001, Currency: "CNY", ReservedMinor: 5000, UpdatedAt: now,
	}).Error)
	require.NoError(t, db.Create(&model.D2SWithdrawalRequest{
		ID: "admin-audit-withdrawal", UserID: 9001, Currency: "CNY", AmountMinor: 5000,
		AlipayAccount: "user@example.com", RealName: "Test User", Status: "pending", CreatedAt: now, UpdatedAt: now,
	}).Error)
	require.NoError(t, db.Create(&model.D2SManualUnbindRequest{
		ID: "admin-audit-unbind", LicenseID: "license-1", UserID: 9002, Reason: "support case",
		Status: "pending", CreatedAt: now, UpdatedAt: now,
	}).Error)
	require.NoError(t, db.Create(&model.D2SUserProfile{UserID: 9003, Region: "", UpdatedAt: now}).Error)
	require.NoError(t, db.Create(&model.D2SSigningKey{
		KeyID: "retired-admin-audit-key", Algorithm: "ES256", PublicJWK: "{}", Status: "active", CreatedAt: now,
	}).Error)
	require.NoError(t, db.Create(&model.D2SOrder{
		ID: "admin-audit-order", UserID: 9005, Product: model.D2SOrderProductLicense, ProductID: model.D2SOrderProductLicense,
		Provider: "stripe", PaymentMethod: "stripe", Region: model.D2SRegionINTL, Currency: "USD", AmountMinor: 2990,
		Status: model.D2SOrderPaid, IdempotencyKey: "admin-audit-order-key", CreatedAt: now,
	}).Error)
	require.NoError(t, db.Create(&model.D2SPaymentEvent{
		ID: "admin-audit-event", Provider: "stripe", ProviderEventID: "admin-audit-event-id", OrderID: "admin-audit-order",
		EventType: "paid", AmountMinor: 2990, Currency: "USD", PayloadHash: strings.Repeat("a", 64), ProcessedAt: now,
	}).Error)
	require.NoError(t, db.Create(&model.D2SLicense{
		ID: "admin-audit-license", LicenseCode: "D2S-AUDIT", UserID: 9005, Product: model.D2SProductDesktop2Stereo,
		Kind: model.D2SLicenseKindPaid, Status: model.D2SLicenseStatusActive, Mode: model.D2SLicenseModeUnbound, CreatedAt: now,
	}).Error)
	t.Setenv("D2S_LICENSE_KEY_ID", "current-admin-audit-key")

	context, recorder := d2sAdminBillingAuditContext(http.MethodGet, "/api/v1/admin/withdrawals", "")
	D2SAdminWithdrawals(context)
	assert.Equal(t, http.StatusOK, recorder.Code)
	assert.NotContains(t, recorder.Body.String(), "user@example.com")
	assert.NotContains(t, recorder.Body.String(), "Test User")

	context, recorder = d2sAdminBillingAuditContext(http.MethodGet, "/api/v1/admin/withdrawals/admin-audit-withdrawal/payout", "")
	context.Params = gin.Params{{Key: "id", Value: "admin-audit-withdrawal"}}
	D2SAdminWithdrawalPayout(context)
	assert.Equal(t, http.StatusOK, recorder.Code)
	assert.Contains(t, recorder.Body.String(), "user@example.com")
	assert.Contains(t, recorder.Body.String(), "Test User")

	context, recorder = d2sAdminBillingAuditContext(http.MethodGet, "/api/v1/withdrawal/status", "")
	context.Set("id", 9001)
	D2SWithdrawalStatus(context)
	assert.Equal(t, http.StatusOK, recorder.Code)
	assert.NotContains(t, recorder.Body.String(), "user@example.com")
	assert.NotContains(t, recorder.Body.String(), "Test User")
	for _, test := range []struct {
		path   string
		handle gin.HandlerFunc
	}{
		{path: "/api/v1/admin/orders", handle: D2SAdminOrders},
		{path: "/api/v1/admin/payment-events", handle: D2SAdminPaymentEvents},
		{path: "/api/v1/admin/balances", handle: D2SAdminBalances},
		{path: "/api/v1/admin/licenses", handle: D2SAdminLicenses},
		{path: "/api/v1/admin/unbind-requests", handle: D2SAdminUnbindRequests},
		{path: "/api/v1/admin/signing-keys", handle: D2SAdminSigningKeys},
	} {
		context, recorder = d2sAdminBillingAuditContext(http.MethodGet, test.path, "")
		test.handle(context)
		assert.Equal(t, http.StatusOK, recorder.Code, test.path)
	}

	context, recorder = d2sAdminBillingAuditContext(http.MethodPut, "/api/v1/admin/withdrawals/admin-audit-withdrawal", `{"status":"paid"}`)
	context.Params = gin.Params{{Key: "id", Value: "admin-audit-withdrawal"}}
	D2SAdminWithdrawalReview(context)
	assert.Equal(t, http.StatusOK, recorder.Code)

	context, recorder = d2sAdminBillingAuditContext(http.MethodPut, "/api/v1/admin/unbind-requests/admin-audit-unbind", `{"status":"rejected","note":"reviewed"}`)
	context.Params = gin.Params{{Key: "id", Value: "admin-audit-unbind"}}
	D2SAdminUnbindReview(context)
	assert.Equal(t, http.StatusOK, recorder.Code)

	context, recorder = d2sAdminBillingAuditContext(http.MethodPut, "/api/v1/admin/users/9003/region", `{"region":"CN"}`)
	context.Params = gin.Params{{Key: "id", Value: "9003"}}
	D2SAdminSetRegion(context)
	assert.Equal(t, http.StatusOK, recorder.Code)

	context, recorder = d2sAdminBillingAuditContext(http.MethodPut, "/api/v1/admin/signing-keys/retired-admin-audit-key", `{"status":"retired"}`)
	context.Params = gin.Params{{Key: "id", Value: "retired-admin-audit-key"}}
	D2SAdminSigningKeyReview(context)
	assert.Equal(t, http.StatusOK, recorder.Code)

	var audits []model.D2SAdminOperationAudit
	require.NoError(t, db.Order("operation_type ASC").Find(&audits).Error)
	operations := make(map[string]int)
	for _, audit := range audits {
		operations[audit.OperationType]++
		assert.Equal(t, 77, audit.OperatorID)
		assert.Equal(t, "operator@example.com", audit.OperatorEmail)
		assert.Equal(t, "SUCCESS", audit.Result)
	}
	for _, operation := range []string{
		"VIEW_WITHDRAWALS", "VIEW_WITHDRAWAL_PAYOUT", "VIEW_ADMIN_ORDERS", "VIEW_PAYMENT_EVENTS", "VIEW_BALANCES",
		"VIEW_LICENSES", "VIEW_UNBIND_REQUESTS", "VIEW_SIGNING_KEYS",
		"REVIEW_WITHDRAWAL", "REVIEW_MANUAL_UNBIND", "SET_USER_REGION", "RETIRE_SIGNING_KEY",
	} {
		assert.Equal(t, 1, operations[operation], operation)
	}
}

func TestD2SAdminWithdrawalReviewRollsBackWhenAuditCannotBeStored(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupD2SAdminBillingAuditDB(t)
	const now int64 = 2_000_500_100
	require.NoError(t, db.Create(&model.D2SBalanceAccount{
		ID: "admin-audit-rollback-balance", UserID: 9004, Currency: "CNY", ReservedMinor: 5000, UpdatedAt: now,
	}).Error)
	require.NoError(t, db.Create(&model.D2SWithdrawalRequest{
		ID: "admin-audit-rollback-request", UserID: 9004, Currency: "CNY", AmountMinor: 5000,
		AlipayAccount: "user@example.com", RealName: "Test User", Status: "pending", CreatedAt: now, UpdatedAt: now,
	}).Error)
	require.NoError(t, db.Create(&model.D2SManualUnbindRequest{
		ID: "admin-audit-rollback-unbind", LicenseID: "license-rollback", UserID: 9005,
		Reason: "support case", Status: "pending", CreatedAt: now, UpdatedAt: now,
	}).Error)
	require.NoError(t, db.Create(&model.D2SUserProfile{UserID: 9006, Region: "", UpdatedAt: now}).Error)
	require.NoError(t, db.Create(&model.D2SSigningKey{
		KeyID: "admin-audit-rollback-key", Algorithm: "ES256", PublicJWK: "{}", Status: "active", CreatedAt: now,
	}).Error)
	t.Setenv("D2S_LICENSE_KEY_ID", "current-admin-audit-key")
	require.NoError(t, db.Migrator().DropTable(&model.D2SAdminOperationAudit{}))

	context, recorder := d2sAdminBillingAuditContext(http.MethodPut, "/api/v1/admin/withdrawals/admin-audit-rollback-request", `{"status":"paid"}`)
	context.Params = gin.Params{{Key: "id", Value: "admin-audit-rollback-request"}}
	D2SAdminWithdrawalReview(context)

	assert.Equal(t, http.StatusInternalServerError, recorder.Code)
	var persisted model.D2SWithdrawalRequest
	require.NoError(t, db.First(&persisted, "id = ?", "admin-audit-rollback-request").Error)
	assert.Equal(t, "pending", persisted.Status)
	var account model.D2SBalanceAccount
	require.NoError(t, db.First(&account, "id = ?", "admin-audit-rollback-balance").Error)
	assert.EqualValues(t, 5000, account.ReservedMinor)

	context, recorder = d2sAdminBillingAuditContext(http.MethodPut, "/api/v1/admin/unbind-requests/admin-audit-rollback-unbind", `{"status":"rejected"}`)
	context.Params = gin.Params{{Key: "id", Value: "admin-audit-rollback-unbind"}}
	D2SAdminUnbindReview(context)
	assert.Equal(t, http.StatusInternalServerError, recorder.Code)
	var unbind model.D2SManualUnbindRequest
	require.NoError(t, db.First(&unbind, "id = ?", "admin-audit-rollback-unbind").Error)
	assert.Equal(t, "pending", unbind.Status)

	context, recorder = d2sAdminBillingAuditContext(http.MethodPut, "/api/v1/admin/users/9006/region", `{"region":"CN"}`)
	context.Params = gin.Params{{Key: "id", Value: "9006"}}
	D2SAdminSetRegion(context)
	assert.Equal(t, http.StatusInternalServerError, recorder.Code)
	var profile model.D2SUserProfile
	require.NoError(t, db.First(&profile, "user_id = ?", 9006).Error)
	assert.Empty(t, profile.Region)

	context, recorder = d2sAdminBillingAuditContext(http.MethodPut, "/api/v1/admin/signing-keys/admin-audit-rollback-key", `{"status":"retired"}`)
	context.Params = gin.Params{{Key: "id", Value: "admin-audit-rollback-key"}}
	D2SAdminSigningKeyReview(context)
	assert.Equal(t, http.StatusInternalServerError, recorder.Code)
	var key model.D2SSigningKey
	require.NoError(t, db.First(&key, "key_id = ?", "admin-audit-rollback-key").Error)
	assert.Equal(t, "active", key.Status)
}

func TestD2SAdminSensitiveListReadsFailClosedWhenAuditCannotBeStored(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupD2SAdminBillingAuditDB(t)
	require.NoError(t, db.Create(&model.D2SWithdrawalRequest{
		ID: "admin-audit-fail-closed-withdrawal", UserID: 9101, Currency: "CNY", AmountMinor: 5000,
		AlipayAccount: "private@example.com", RealName: "Private Name", Status: "pending", CreatedAt: 2_000_500_200,
	}).Error)
	require.NoError(t, db.Create(&model.D2SManualUnbindRequest{
		ID: "admin-audit-fail-closed-unbind", LicenseID: "private-license", UserID: 9102,
		Reason: "private reason", Status: "pending", CreatedAt: 2_000_500_200,
	}).Error)
	require.NoError(t, db.Create(&model.D2SBalanceAccount{
		ID: "private-balance-account", UserID: 9103, Currency: "USD", AvailableMinor: -2990, UpdatedAt: 2_000_500_200,
	}).Error)
	require.NoError(t, db.Create(&model.D2SOrder{
		ID: "private-order", UserID: 9104, Product: model.D2SOrderProductLicense, ProductID: model.D2SOrderProductLicense,
		Provider: "stripe", PaymentMethod: "stripe", Region: model.D2SRegionINTL, Currency: "USD", AmountMinor: 2990,
		Status: model.D2SOrderPaid, IdempotencyKey: "private-order-key", EmailSnapshot: "private-order@example.com", CreatedAt: 2_000_500_200,
	}).Error)
	require.NoError(t, db.Create(&model.D2SPaymentEvent{
		ID: "private-payment-event", Provider: "stripe", ProviderEventID: "private-provider-event", OrderID: "private-order",
		EventType: "paid", AmountMinor: 2990, Currency: "USD", PayloadHash: strings.Repeat("b", 64), ProcessedAt: 2_000_500_200,
	}).Error)
	require.NoError(t, db.Create(&model.D2SLicense{
		ID: "private-license-record", LicenseCode: "D2S-PRIVATE-LICENSE", UserID: 9104,
		Product: model.D2SProductDesktop2Stereo, Kind: model.D2SLicenseKindPaid,
		Status: model.D2SLicenseStatusActive, Mode: model.D2SLicenseModeUnbound, CreatedAt: 2_000_500_200,
	}).Error)
	require.NoError(t, db.Create(&model.D2SSigningKey{
		KeyID: "private-signing-key", Algorithm: "ES256", PublicJWK: `{"kid":"private-signing-key"}`,
		Status: "active", CreatedAt: 2_000_500_200,
	}).Error)
	require.NoError(t, db.Migrator().DropTable(&model.D2SAdminOperationAudit{}))

	for _, test := range []struct {
		path   string
		handle gin.HandlerFunc
		marker string
	}{
		{path: "/api/v1/admin/withdrawals", handle: D2SAdminWithdrawals, marker: "private@example.com"},
		{path: "/api/v1/admin/unbind-requests", handle: D2SAdminUnbindRequests, marker: "private reason"},
		{path: "/api/v1/admin/orders", handle: D2SAdminOrders, marker: "private-order@example.com"},
		{path: "/api/v1/admin/payment-events", handle: D2SAdminPaymentEvents, marker: "private-provider-event"},
		{path: "/api/v1/admin/balances", handle: D2SAdminBalances, marker: "private-balance-account"},
		{path: "/api/v1/admin/licenses", handle: D2SAdminLicenses, marker: "D2S-PRIVATE-LICENSE"},
		{path: "/api/v1/admin/signing-keys", handle: D2SAdminSigningKeys, marker: "private-signing-key"},
	} {
		context, recorder := d2sAdminBillingAuditContext(http.MethodGet, test.path, "")
		test.handle(context)
		assert.Equal(t, http.StatusInternalServerError, recorder.Code, test.path)
		assert.Contains(t, recorder.Body.String(), `"code":"AUDIT_WRITE_FAILED"`, test.path)
		assert.NotContains(t, recorder.Body.String(), test.marker, test.path)
		assert.NotContains(t, recorder.Body.String(), "Private Name", test.path)
	}
	context, recorder := d2sAdminBillingAuditContext(http.MethodGet, "/api/v1/admin/withdrawals/admin-audit-fail-closed-withdrawal/payout", "")
	context.Params = gin.Params{{Key: "id", Value: "admin-audit-fail-closed-withdrawal"}}
	D2SAdminWithdrawalPayout(context)
	assert.Equal(t, http.StatusInternalServerError, recorder.Code)
	assert.Contains(t, recorder.Body.String(), `"code":"AUDIT_WRITE_FAILED"`)
	assert.NotContains(t, recorder.Body.String(), "private@example.com")
	assert.NotContains(t, recorder.Body.String(), "Private Name")
}

func TestD2SAdminWithdrawalPayoutDoesNotReturnClosedRequestDetails(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupD2SAdminBillingAuditDB(t)
	require.NoError(t, db.Create(&model.D2SWithdrawalRequest{
		ID: "closed-withdrawal", UserID: 9101, Currency: "CNY", AmountMinor: 5000,
		AlipayAccount: "closed@example.com", RealName: "Closed Recipient", Status: "paid", CreatedAt: 2_000_500_200,
	}).Error)

	context, recorder := d2sAdminBillingAuditContext(http.MethodGet, "/api/v1/admin/withdrawals/closed-withdrawal/payout", "")
	context.Params = gin.Params{{Key: "id", Value: "closed-withdrawal"}}
	D2SAdminWithdrawalPayout(context)
	assert.Equal(t, http.StatusNotFound, recorder.Code)
	assert.NotContains(t, recorder.Body.String(), "closed@example.com")
	assert.NotContains(t, recorder.Body.String(), "Closed Recipient")

	var audit model.D2SAdminOperationAudit
	require.NoError(t, db.Where("operation_type = ?", "VIEW_WITHDRAWAL_PAYOUT").Take(&audit).Error)
	assert.Equal(t, "FAILURE", audit.Result)
	assert.Contains(t, audit.DetailsJSON, "not_found")
}
