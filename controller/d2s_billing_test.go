package controller

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestD2SOrderCreateRejectsChangedServerQuoteBeforeCreatingOrder(t *testing.T) {
	t.Setenv("D2S_PURCHASES_ENABLED", "true")
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open("file:d2s_order_quote_snapshot_test?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	previousDB, previousLogDB := model.DB, model.LOG_DB
	previousMainType, previousLogType := common.MainDatabaseType(), common.LogDatabaseType()
	model.DB, model.LOG_DB = db, db
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	t.Cleanup(func() {
		model.DB, model.LOG_DB = previousDB, previousLogDB
		common.SetDatabaseTypes(previousMainType, previousLogType)
	})
	require.NoError(t, db.AutoMigrate(
		&model.User{}, &model.D2SUserProfile{}, &model.D2SLicense{}, &model.D2SLicenseEvent{},
		&model.D2SOrder{}, &model.D2SUserSecurityControl{}, &model.D2SUserActionLog{},
	))
	require.NoError(t, db.Create(&model.User{Id: 1401, Username: "quote-test", Email: "quote-test@example.com", EmailVerifiedAt: time.Now().Unix(), Password: "unused-hash", Role: common.RoleCommonUser, Status: common.UserStatusEnabled, Group: "default", AuthVersion: 1}).Error)

	for _, test := range []struct {
		name, quoteFields string
	}{
		{name: "quote omitted"},
		{name: "amount changed", quoteFields: `"expected_amount_minor":2991,"expected_currency":"USD","expected_region":"INTL"`},
		{name: "currency changed", quoteFields: `"expected_amount_minor":2990,"expected_currency":"CNY","expected_region":"INTL"`},
		{name: "region changed", quoteFields: `"expected_amount_minor":2990,"expected_currency":"USD","expected_region":"CN"`},
	} {
		t.Run(test.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			context, _ := gin.CreateTestContext(recorder)
			context.Set("id", 1401)
			body := `{"product":"license","provider":"stripe","balance_minor":0,"idempotency_key":"` + test.name + `"`
			if test.quoteFields != "" {
				body += "," + test.quoteFields
			}
			body += "}"
			context.Request = httptest.NewRequest(http.MethodPost, "/api/v1/orders/create", strings.NewReader(body))

			D2SOrderCreate(context)

			assert.Equal(t, http.StatusConflict, recorder.Code)
			var response struct {
				Error struct {
					Code string `json:"code"`
				} `json:"error"`
			}
			require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
			assert.Equal(t, "quote_changed", response.Error.Code)
			var orderCount int64
			require.NoError(t, db.Model(&model.D2SOrder{}).Count(&orderCount).Error)
			assert.Zero(t, orderCount)
		})
	}
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Set("id", 1401)
	context.Request = httptest.NewRequest(
		http.MethodPost,
		"/api/v1/orders/create",
		strings.NewReader(`{"product":"license","provider":"stripe","balance_minor":0,"idempotency_key":"confirmed-quote","expected_amount_minor":2990,"expected_currency":"USD","expected_region":"INTL"}`),
	)

	D2SOrderCreate(context)

	assert.Equal(t, http.StatusCreated, recorder.Code)
	var orderCount, purchaseAuditCount int64
	require.NoError(t, db.Model(&model.D2SOrder{}).Count(&orderCount).Error)
	require.NoError(t, db.Model(&model.D2SUserActionLog{}).Where("action_type = ?", model.D2SActionPurchase).Count(&purchaseAuditCount).Error)
	assert.EqualValues(t, 1, orderCount)
	assert.EqualValues(t, 1, purchaseAuditCount)
}

func TestD2SOrderCreateReplaysHistoricalQuoteSnapshot(t *testing.T) {
	t.Setenv("D2S_PURCHASES_ENABLED", "true")
	gin.SetMode(gin.TestMode)
	previousDB := model.DB
	db, err := gorm.Open(sqlite.Open("file:d2s_order_historical_quote_test?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	model.DB = db
	t.Cleanup(func() {
		model.DB = previousDB
		sqlDB, dbErr := db.DB()
		if dbErr == nil {
			_ = sqlDB.Close()
		}
	})
	require.NoError(t, db.AutoMigrate(&model.D2SOrder{}, &model.D2SUserSecurityControl{}))
	historicalOrder := model.D2SOrder{
		ID: "historical-quote-order", UserID: 1402, Product: model.D2SOrderProductLicense,
		ProductID: model.D2SOrderProductLicense, Provider: "stripe", PaymentMethod: "stripe",
		Region: model.D2SRegionINTL, Currency: "USD", AmountMinor: 2500, GatewayMinor: 2500,
		Status: model.D2SOrderPending, ResultStatus: model.D2SActionResultSuccess,
		IdempotencyKey: "historical-price-retry", PurchaseTime: 1_800_000_000,
		CreatedAt: 1_800_000_000, ExpiresAt: 1_800_001_800,
	}
	require.NoError(t, db.Create(&historicalOrder).Error)

	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Set("id", historicalOrder.UserID)
	context.Request = httptest.NewRequest(http.MethodPost, "/api/v1/orders/create", strings.NewReader(
		`{"product":"license","provider":"stripe","balance_minor":0,"idempotency_key":"historical-price-retry","expected_amount_minor":2500,"expected_currency":"USD","expected_region":"INTL"}`,
	))

	D2SOrderCreate(context)

	assert.Equal(t, http.StatusOK, recorder.Code)
	var response struct {
		Data model.D2SOrder `json:"data"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	assert.Equal(t, historicalOrder.ID, response.Data.ID)
	assert.Equal(t, int64(2500), response.Data.AmountMinor)
}

func TestD2SSpecializedOrderEndpointsRejectWrongProduct(t *testing.T) {
	t.Setenv("D2S_PURCHASES_ENABLED", "true")
	gin.SetMode(gin.TestMode)
	previousDB := model.DB
	db, err := gorm.Open(sqlite.Open("file:d2s_specialized_order_product_test?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.D2SUserSecurityControl{}))
	model.DB = db
	t.Cleanup(func() {
		model.DB = previousDB
		sqlDB, dbErr := db.DB()
		if dbErr == nil {
			_ = sqlDB.Close()
		}
	})
	tests := []struct {
		name    string
		handler gin.HandlerFunc
		product string
	}{
		{name: "paid revoke", handler: D2SLicensePaidRevoke, product: "license"},
		{name: "offline extension", handler: D2SLicenseOfflineExtend, product: "paid_revoke"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			context, _ := gin.CreateTestContext(recorder)
			context.Set("id", 1)
			context.Request = httptest.NewRequest(
				http.MethodPost,
				"/api/v1/license/"+strings.ReplaceAll(test.name, " ", "/"),
				strings.NewReader(`{"product":"`+test.product+`","provider":"stripe","idempotency_key":"wrong-product"}`),
			)

			test.handler(context)

			assert.Equal(t, http.StatusBadRequest, recorder.Code)
		})
	}
}

func TestD2SPurchaseGateDefaultsClosed(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Setenv("D2S_PURCHASES_ENABLED", "")

	tests := []struct {
		name    string
		handler gin.HandlerFunc
		method  string
		path    string
	}{
		{name: "preview", handler: D2SOrderPreview, method: http.MethodPost, path: "/api/v1/orders/preview"},
		{name: "create", handler: D2SOrderCreate, method: http.MethodPost, path: "/api/v1/orders/create"},
		{name: "checkout", handler: D2SOrderCheckout, method: http.MethodPost, path: "/api/v1/orders/order-1/checkout"},
		{name: "paid revoke", handler: D2SLicensePaidRevoke, method: http.MethodPost, path: "/api/v1/license/revoke/paid"},
		{name: "offline extension", handler: D2SLicenseOfflineExtend, method: http.MethodPost, path: "/api/v1/license/offline/extend"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			context, _ := gin.CreateTestContext(recorder)
			context.Set("id", 1901)
			context.Request = httptest.NewRequest(test.method, test.path, strings.NewReader(`{}`))

			test.handler(context)

			assert.Equal(t, http.StatusServiceUnavailable, recorder.Code)
			var response struct {
				Success bool `json:"success"`
				Error   struct {
					Code string `json:"code"`
				} `json:"error"`
			}
			require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
			assert.False(t, response.Success)
			assert.Equal(t, "purchases_disabled", response.Error.Code)
		})
	}

	providerRecorder := httptest.NewRecorder()
	providerContext, _ := gin.CreateTestContext(providerRecorder)
	providerContext.Set("id", 1901)
	providerContext.Request = httptest.NewRequest(http.MethodGet, "/api/v1/orders/providers", nil)
	D2SOrderProviders(providerContext)
	assert.Equal(t, http.StatusOK, providerRecorder.Code)
	var providersResponse struct {
		Data struct {
			Enabled bool     `json:"enabled"`
			Items   []string `json:"providers"`
		} `json:"data"`
	}
	require.NoError(t, common.Unmarshal(providerRecorder.Body.Bytes(), &providersResponse))
	assert.False(t, providersResponse.Data.Enabled)
	assert.Empty(t, providersResponse.Data.Items)
}

func TestD2SEmergencyPurchaseBlockRejectsEveryPaidEntryPoint(t *testing.T) {
	t.Setenv("D2S_PURCHASES_ENABLED", "true")
	gin.SetMode(gin.TestMode)
	previousDB := model.DB
	db, err := gorm.Open(sqlite.Open("file:d2s_emergency_purchase_block_test?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.D2SUserSecurityControl{}, &model.D2SOrder{}))
	require.NoError(t, db.Create(&model.D2SUserSecurityControl{UserID: 1902, PurchaseBlocked: true}).Error)
	model.DB = db
	t.Cleanup(func() {
		model.DB = previousDB
		sqlDB, dbErr := db.DB()
		if dbErr == nil {
			_ = sqlDB.Close()
		}
	})

	tests := []struct {
		name    string
		handler gin.HandlerFunc
		path    string
	}{
		{name: "preview", handler: D2SOrderPreview, path: "/api/v1/orders/preview"},
		{name: "create", handler: D2SOrderCreate, path: "/api/v1/orders/create"},
		{name: "checkout", handler: D2SOrderCheckout, path: "/api/v1/orders/order-1/checkout"},
		{name: "paid revoke", handler: D2SLicensePaidRevoke, path: "/api/v1/license/revoke/paid"},
		{name: "offline extension", handler: D2SLicenseOfflineExtend, path: "/api/v1/license/offline/extend"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			context, _ := gin.CreateTestContext(recorder)
			context.Set("id", 1902)
			context.Request = httptest.NewRequest(http.MethodPost, test.path, strings.NewReader(`{}`))

			test.handler(context)

			assert.Equal(t, http.StatusLocked, recorder.Code)
			var response struct {
				Success bool   `json:"success"`
				Code    string `json:"code"`
			}
			require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
			assert.False(t, response.Success)
			assert.Equal(t, "ACCOUNT_PURCHASE_BLOCKED", response.Code)
		})
	}

	var orderCount int64
	require.NoError(t, db.Model(&model.D2SOrder{}).Count(&orderCount).Error)
	assert.Zero(t, orderCount)
}

func TestD2SErrorMapsBlockedAccountLogin(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)

	d2sError(context, model.ErrD2SAccountLoginBlocked)

	assert.Equal(t, http.StatusLocked, recorder.Code)
	var response struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	assert.Equal(t, "account_login_blocked", response.Error.Code)
}

func TestD2SErrorMapsInvalidOfflinePeriod(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)

	d2sError(context, model.ErrD2SOfflinePeriodInvalid)

	assert.Equal(t, http.StatusBadRequest, recorder.Code)
	var response struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &response))
	assert.Equal(t, "invalid_offline_period", response.Error.Code)
}
