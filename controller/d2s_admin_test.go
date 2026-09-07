package controller

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestD2SAdminLicensesIncludesUserRegion(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:d2s_admin_license_region_test?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	previousDB, previousLogDB := model.DB, model.LOG_DB
	previousMainType, previousLogType := common.MainDatabaseType(), common.LogDatabaseType()
	model.DB, model.LOG_DB = db, db
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	t.Cleanup(func() {
		model.DB, model.LOG_DB = previousDB, previousLogDB
		common.SetDatabaseTypes(previousMainType, previousLogType)
	})
	require.NoError(t, db.AutoMigrate(&model.D2SUserProfile{}, &model.D2SLicense{}))
	require.NoError(t, db.Create(&model.D2SUserProfile{UserID: 701, Region: model.D2SRegionINTL}).Error)
	require.NoError(t, db.Create(&model.D2SUserProfile{UserID: 702, Region: ""}).Error)
	require.NoError(t, db.Create(&model.D2SLicense{ID: "region-license-intl", LicenseCode: "D2S-INTL", UserID: 701, Product: model.D2SProductDesktop2Stereo, Kind: model.D2SLicenseKindPaid, Status: model.D2SLicenseStatusActive, Mode: model.D2SLicenseModeUnbound}).Error)
	require.NoError(t, db.Create(&model.D2SLicense{ID: "region-license-unlocked", LicenseCode: "D2S-UNLOCKED", UserID: 702, Product: model.D2SProductDesktop2Stereo, Kind: model.D2SLicenseKindTrial, Status: model.D2SLicenseStatusActive, Mode: model.D2SLicenseModeUnbound}).Error)

	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest(http.MethodGet, "/api/v1/admin/licenses", nil)
	D2SAdminLicenses(context)

	assert.Equal(t, http.StatusOK, recorder.Code)
	var response struct {
		Data struct {
			Licenses []struct {
				UserID int    `json:"user_id"`
				Region string `json:"region"`
			} `json:"licenses"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &response))
	regions := map[int]string{}
	for _, license := range response.Data.Licenses {
		regions[license.UserID] = license.Region
	}
	assert.Equal(t, model.D2SRegionINTL, regions[701])
	assert.Equal(t, "", regions[702])
}

func TestD2SAdminSigningKeysMarksCurrentKey(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:d2s_admin_signing_keys_test?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	previousDB, previousLogDB := model.DB, model.LOG_DB
	previousMainType, previousLogType := common.MainDatabaseType(), common.LogDatabaseType()
	model.DB, model.LOG_DB = db, db
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	previousKeyID := os.Getenv("D2S_LICENSE_KEY_ID")
	t.Setenv("D2S_LICENSE_KEY_ID", "current-admin-key")
	t.Cleanup(func() {
		_ = os.Setenv("D2S_LICENSE_KEY_ID", previousKeyID)
		model.DB, model.LOG_DB = previousDB, previousLogDB
		common.SetDatabaseTypes(previousMainType, previousLogType)
	})
	require.NoError(t, db.AutoMigrate(&model.D2SSigningKey{}))
	require.NoError(t, db.Create(&model.D2SSigningKey{KeyID: "current-admin-key", Algorithm: "ES256", PublicJWK: "{}", Status: "active", CreatedAt: 1}).Error)
	require.NoError(t, db.Create(&model.D2SSigningKey{KeyID: "old-admin-key", Algorithm: "ES256", PublicJWK: "{}", Status: "active", CreatedAt: 2}).Error)

	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest(http.MethodGet, "/api/v1/admin/signing-keys", nil)
	D2SAdminSigningKeys(context)

	assert.Equal(t, http.StatusOK, recorder.Code)
	var response struct {
		Data struct {
			Keys []struct {
				KeyID     string `json:"key_id"`
				IsCurrent bool   `json:"is_current"`
			} `json:"keys"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &response))
	current := map[string]bool{}
	for _, key := range response.Data.Keys {
		current[key.KeyID] = key.IsCurrent
	}
	assert.True(t, current["current-admin-key"])
	assert.False(t, current["old-admin-key"])
	assert.True(t, service.IsD2SCurrentSigningKey("current-admin-key"))
}

func TestD2SAdminBalancesDefaultsToNegativeAccounts(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:d2s_admin_balances_default_test?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	previousDB, previousLogDB := model.DB, model.LOG_DB
	previousMainType, previousLogType := common.MainDatabaseType(), common.LogDatabaseType()
	model.DB, model.LOG_DB = db, db
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	t.Cleanup(func() {
		model.DB, model.LOG_DB = previousDB, previousLogDB
		common.SetDatabaseTypes(previousMainType, previousLogType)
	})
	require.NoError(t, db.AutoMigrate(&model.D2SBalanceAccount{}))
	require.NoError(t, db.Create(&model.D2SBalanceAccount{ID: "negative-balance", UserID: 801, Currency: "USD", AvailableMinor: -140}).Error)
	require.NoError(t, db.Create(&model.D2SBalanceAccount{ID: "positive-balance", UserID: 802, Currency: "USD", AvailableMinor: 140}).Error)

	call := func(path string) []model.D2SBalanceAccount {
		recorder := httptest.NewRecorder()
		context, _ := gin.CreateTestContext(recorder)
		context.Request = httptest.NewRequest(http.MethodGet, path, nil)
		D2SAdminBalances(context)
		assert.Equal(t, http.StatusOK, recorder.Code)
		var response struct {
			Data struct {
				Accounts []model.D2SBalanceAccount `json:"accounts"`
			} `json:"data"`
		}
		require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &response))
		return response.Data.Accounts
	}

	defaultAccounts := call("/api/v1/admin/balances")
	require.Len(t, defaultAccounts, 1)
	assert.Equal(t, "negative-balance", defaultAccounts[0].ID)

	allAccounts := call("/api/v1/admin/balances?negative=false")
	assert.Len(t, allAccounts, 2)
}

func TestD2SAdminBalancesRejectsInvalidUserFilter(t *testing.T) {
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest(http.MethodGet, "/api/v1/admin/balances?user_id=not-a-number", nil)
	D2SAdminBalances(context)

	assert.Equal(t, http.StatusBadRequest, recorder.Code)
}

func TestD2SAdminOrdersSupportsStatusAndUserFilters(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:d2s_admin_orders_filters_test?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	previousDB, previousLogDB := model.DB, model.LOG_DB
	previousMainType, previousLogType := common.MainDatabaseType(), common.LogDatabaseType()
	model.DB, model.LOG_DB = db, db
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	t.Cleanup(func() {
		model.DB, model.LOG_DB = previousDB, previousLogDB
		common.SetDatabaseTypes(previousMainType, previousLogType)
	})
	require.NoError(t, db.AutoMigrate(&model.D2SOrder{}))
	require.NoError(t, db.Create(&model.D2SOrder{ID: "admin-order-paid", UserID: 901, Status: model.D2SOrderPaid, IdempotencyKey: "admin-paid-key", CreatedAt: 3}).Error)
	require.NoError(t, db.Create(&model.D2SOrder{ID: "admin-order-pending", UserID: 901, Status: model.D2SOrderPending, IdempotencyKey: "admin-pending-key", CreatedAt: 2}).Error)
	require.NoError(t, db.Create(&model.D2SOrder{ID: "other-user-paid", UserID: 902, Status: model.D2SOrderPaid, IdempotencyKey: "other-paid-key", CreatedAt: 1}).Error)

	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest(http.MethodGet, "/api/v1/admin/orders?status=paid&user_id=901", nil)
	D2SAdminOrders(context)

	assert.Equal(t, http.StatusOK, recorder.Code)
	var response struct {
		Data struct {
			Orders []model.D2SOrder `json:"orders"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &response))
	require.Len(t, response.Data.Orders, 1)
	assert.Equal(t, "admin-order-paid", response.Data.Orders[0].ID)
}

func TestD2SAdminOrdersRejectsInvalidUserFilter(t *testing.T) {
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest(http.MethodGet, "/api/v1/admin/orders?user_id=not-a-number", nil)
	D2SAdminOrders(context)

	assert.Equal(t, http.StatusBadRequest, recorder.Code)
}

func TestD2SAdminOrdersRejectsInvalidStatusFilter(t *testing.T) {
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest(http.MethodGet, "/api/v1/admin/orders?status=unknown", nil)
	D2SAdminOrders(context)

	assert.Equal(t, http.StatusBadRequest, recorder.Code)
}

func TestD2SAdminPaymentEventsSupportsOperationalFilters(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:d2s_admin_payment_events_test?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	previousDB, previousLogDB := model.DB, model.LOG_DB
	previousMainType, previousLogType := common.MainDatabaseType(), common.LogDatabaseType()
	model.DB, model.LOG_DB = db, db
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	t.Cleanup(func() {
		model.DB, model.LOG_DB = previousDB, previousLogDB
		common.SetDatabaseTypes(previousMainType, previousLogType)
	})
	require.NoError(t, db.AutoMigrate(&model.D2SPaymentEvent{}))
	require.NoError(t, db.Create(&model.D2SPaymentEvent{
		ID: "admin-event-1", Provider: "stripe", ProviderEventID: "stripe-chargeback-1",
		OrderID: "admin-order-1", EventType: "chargeback", AmountMinor: 2990, Currency: "USD",
		PayloadHash: "hash-1", ProcessedAt: 20,
	}).Error)
	require.NoError(t, db.Create(&model.D2SPaymentEvent{
		ID: "admin-event-2", Provider: "creem", ProviderEventID: "creem-paid-1",
		OrderID: "admin-order-2", EventType: "paid", AmountMinor: 2990, Currency: "USD",
		PayloadHash: "hash-2", ProcessedAt: 10,
	}).Error)

	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest(http.MethodGet, "/api/v1/admin/payment-events?provider=STRIPE&event_type=CHARGEBACK", nil)
	D2SAdminPaymentEvents(context)

	assert.Equal(t, http.StatusOK, recorder.Code)
	var response struct {
		Data struct {
			Events []model.D2SPaymentEvent `json:"events"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &response))
	require.Len(t, response.Data.Events, 1)
	assert.Equal(t, "stripe-chargeback-1", response.Data.Events[0].ProviderEventID)
}

func TestD2SAdminReconciliationRejectsInvalidWindow(t *testing.T) {
	for _, query := range []string{
		"?start_at=200&end_at=200",
		"?start_at=0&end_at=200",
		"?start_at=300&end_at=200",
	} {
		recorder := httptest.NewRecorder()
		context, _ := gin.CreateTestContext(recorder)
		context.Request = httptest.NewRequest(http.MethodGet, "/api/v1/admin/reconciliation"+query, nil)
		D2SAdminReconciliation(context)

		assert.Equal(t, http.StatusBadRequest, recorder.Code)
		var response struct {
			Success bool `json:"success"`
			Error   struct {
				Code string `json:"code"`
			} `json:"error"`
		}
		require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &response))
		assert.False(t, response.Success)
		assert.Equal(t, "invalid_input", response.Error.Code)
	}
}

func TestD2SInviteRecordsLimitResults(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:d2s_invite_records_limit_test?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	previousDB, previousLogDB := model.DB, model.LOG_DB
	previousMainType, previousLogType := common.MainDatabaseType(), common.LogDatabaseType()
	model.DB, model.LOG_DB = db, db
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	t.Cleanup(func() {
		model.DB, model.LOG_DB = previousDB, previousLogDB
		common.SetDatabaseTypes(previousMainType, previousLogType)
	})
	require.NoError(t, db.AutoMigrate(&model.D2SInviteReward{}))
	for i := 1; i <= 501; i++ {
		require.NoError(t, db.Create(&model.D2SInviteReward{
			ID: "invite-reward-" + strconv.Itoa(i), InviterUserID: 901,
			InviteeUserID: 1000 + i, OrderID: "invite-order-" + strconv.Itoa(i),
			Currency: "USD", AmountMinor: 140, Status: "credited", CreatedAt: int64(i),
		}).Error)
	}

	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest(http.MethodGet, "/api/v1/invite/records", nil)
	context.Set("id", 901)
	D2SInviteRecords(context)

	assert.Equal(t, http.StatusOK, recorder.Code)
	var response struct {
		Data struct {
			Records []model.D2SInviteReward `json:"records"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &response))
	require.Len(t, response.Data.Records, 500)
	assert.Equal(t, "invite-reward-501", response.Data.Records[0].ID)
}
