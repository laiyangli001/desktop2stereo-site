package controller

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"testing"

	"github.com/Calcium-Ion/go-epay/epay"
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stripe/stripe-go/v81"
	"github.com/waffo-com/waffo-go/core"
	"gorm.io/gorm"
)

func stripeD2STestEvent(eventType stripe.EventType, object map[string]any) stripe.Event {
	raw, _ := json.Marshal(object)
	var objectMap map[string]interface{}
	_ = json.Unmarshal(raw, &objectMap)
	return stripe.Event{
		ID:   "evt_d2s_test",
		Type: eventType,
		Data: &stripe.EventData{Raw: raw, Object: objectMap},
	}
}

func TestD2SPaymentWebhookRejectsBridgeFailures(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:d2s_bridge_test?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	previousDB, previousLogDB := model.DB, model.LOG_DB
	previousMainType, previousLogType := common.MainDatabaseType(), common.LogDatabaseType()
	model.DB, model.LOG_DB = db, db
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	t.Cleanup(func() {
		model.DB, model.LOG_DB = previousDB, previousLogDB
		common.SetDatabaseTypes(previousMainType, previousLogType)
	})
	require.NoError(t, db.AutoMigrate(&model.D2SOrder{}, &model.D2SPaymentEvent{}))
	previousSecret := os.Getenv("D2S_PAYMENT_BRIDGE_SECRET")
	t.Cleanup(func() { _ = os.Setenv("D2S_PAYMENT_BRIDGE_SECRET", previousSecret) })
	require.NoError(t, os.Setenv("D2S_PAYMENT_BRIDGE_SECRET", "bridge-test-secret"))

	call := func(body, signature string) *httptest.ResponseRecorder {
		request := httptest.NewRequest(http.MethodPost, "/api/v1/webhooks/stripe", bytes.NewBufferString(body))
		request.Header.Set("X-D2S-Signature", signature)
		recorder := httptest.NewRecorder()
		context, _ := gin.CreateTestContext(recorder)
		context.Request = request
		context.Params = gin.Params{{Key: "provider", Value: "stripe"}}
		D2SPaymentWebhook(context)
		return recorder
	}

	assert.Equal(t, http.StatusBadRequest, call(`{"event_id":"evt","order_id":"order","event_type":"paid","amount_minor":2990,"currency":"USD"}`, "not-hex").Code)
	assert.Equal(t, http.StatusBadRequest, call("not-json", validD2SBridgeSignature("not-json", "bridge-test-secret")).Code)
	validBody := `{"event_id":"evt","order_id":"missing-order","event_type":"paid","amount_minor":2990,"currency":"USD"}`
	assert.Equal(t, http.StatusNotFound, call(validBody, validD2SBridgeSignature(validBody, "bridge-test-secret")).Code)
}

func TestD2SPaymentWebhookSandboxMatrix(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:d2s_bridge_matrix_test?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	previousDB, previousLogDB := model.DB, model.LOG_DB
	previousMainType, previousLogType := common.MainDatabaseType(), common.LogDatabaseType()
	model.DB, model.LOG_DB = db, db
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	t.Cleanup(func() {
		model.DB, model.LOG_DB = previousDB, previousLogDB
		common.SetDatabaseTypes(previousMainType, previousLogType)
	})
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.D2SUserProfile{}, &model.D2SLicense{}, &model.D2SLicenseEvent{}, &model.D2SOrder{}, &model.D2SPaymentEvent{}, &model.D2SInviteReward{}, &model.D2SBalanceAccount{}, &model.D2SBalanceTransaction{}))
	const now = int64(2_000_950_000)
	require.NoError(t, db.Create(&model.User{Id: 1301, Username: "bridge-matrix", Email: "bridge-matrix@example.com", Password: "unused-hash", Role: common.RoleCommonUser, Status: common.UserStatusEnabled, Group: "default", AuthVersion: 1}).Error)
	require.NoError(t, db.Create(&model.D2SUserProfile{UserID: 1301, EmailVerified: true, CreatedAt: now, UpdatedAt: now}).Error)
	require.NoError(t, db.Create(&model.D2SOrder{ID: "bridge-matrix-order", UserID: 1301, Product: model.D2SOrderProductLicense, Provider: "stripe", Region: model.D2SRegionINTL, Currency: "USD", AmountMinor: 2990, GatewayMinor: 2990, Status: model.D2SOrderPending, CreatedAt: now, ExpiresAt: now + 1800}).Error)

	previousSecret := os.Getenv("D2S_PAYMENT_BRIDGE_SECRET")
	t.Setenv("D2S_PAYMENT_BRIDGE_SECRET", "bridge-matrix-secret")
	t.Cleanup(func() { _ = os.Setenv("D2S_PAYMENT_BRIDGE_SECRET", previousSecret) })
	call := func(body string) *httptest.ResponseRecorder {
		recorder := httptest.NewRecorder()
		context, _ := gin.CreateTestContext(recorder)
		context.Request = httptest.NewRequest(http.MethodPost, "/api/v1/webhooks/stripe", bytes.NewBufferString(body))
		context.Params = gin.Params{{Key: "provider", Value: "stripe"}}
		context.Request.Header.Set("X-D2S-Signature", validD2SBridgeSignature(body, "bridge-matrix-secret"))
		D2SPaymentWebhook(context)
		return recorder
	}

	paidBody := `{"event_id":"matrix-paid","order_id":"bridge-matrix-order","event_type":"paid","amount_minor":2990,"currency":"USD"}`
	assert.Equal(t, http.StatusOK, call(paidBody).Code)
	assert.Equal(t, http.StatusOK, call(paidBody).Code)
	mismatchBody := `{"event_id":"matrix-paid","order_id":"bridge-matrix-order","event_type":"paid","amount_minor":2991,"currency":"USD"}`
	assert.Equal(t, http.StatusBadRequest, call(mismatchBody).Code)

	var eventCount int64
	require.NoError(t, db.Model(&model.D2SPaymentEvent{}).Where("provider_event_id = ?", "matrix-paid").Count(&eventCount).Error)
	assert.EqualValues(t, 1, eventCount)
	var order model.D2SOrder
	require.NoError(t, db.First(&order, "id = ?", "bridge-matrix-order").Error)
	assert.Equal(t, model.D2SOrderPaid, order.Status)
}

func validD2SBridgeSignature(body, secret string) string {
	hash := hmac.New(sha256.New, []byte(secret))
	_, _ = hash.Write([]byte(body))
	return hex.EncodeToString(hash.Sum(nil))
}

func TestD2SProviderAdaptersEnterSharedOrderTransaction(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:d2s_provider_adapter_test?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	previousDB, previousLogDB := model.DB, model.LOG_DB
	previousMainType, previousLogType := common.MainDatabaseType(), common.LogDatabaseType()
	model.DB, model.LOG_DB = db, db
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	t.Cleanup(func() {
		model.DB, model.LOG_DB = previousDB, previousLogDB
		common.SetDatabaseTypes(previousMainType, previousLogType)
	})
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.D2SUserProfile{}, &model.D2SLicense{}, &model.D2SLicenseEvent{}, &model.D2SOrder{}, &model.D2SPaymentEvent{}, &model.D2SBalanceAccount{}, &model.D2SBalanceTransaction{}, &model.D2SInviteReward{}))
	const now = int64(2_000_900_000)
	makeOrder := func(id string, userID int, provider, currency string) model.D2SOrder {
		require.NoError(t, db.Create(&model.User{Id: userID, Username: "adapter-user-" + strconv.Itoa(userID), Email: "adapter-" + strconv.Itoa(userID) + "@example.com", Password: "unused-hash", AffCode: "adapter-aff-" + strconv.Itoa(userID), Role: common.RoleCommonUser, Status: common.UserStatusEnabled, Group: "default", AuthVersion: 1}).Error)
		require.NoError(t, db.Create(&model.D2SUserProfile{UserID: userID, EmailVerified: true, CreatedAt: now, UpdatedAt: now}).Error)
		region := model.D2SRegionINTL
		if provider == "epay" || provider == "waffo" {
			region = model.D2SRegionCN
		}
		order := model.D2SOrder{ID: id, UserID: userID, Product: model.D2SOrderProductLicense, Provider: provider, Region: region, Currency: currency, AmountMinor: 2990, GatewayMinor: 2990, Status: model.D2SOrderPending, CreatedAt: now, ExpiresAt: now + 1800}
		require.NoError(t, db.Create(&order).Error)
		return order
	}

	epayOrder := makeOrder("epay-adapter-order", 901, "epay", "CNY")
	epayResult := &epay.VerifyRes{TradeNo: "epay-event", ServiceTradeNo: epayOrder.ID, Money: "29.90", TradeStatus: epay.StatusTradeSuccess}
	handled, err := processEpayD2SPayment(epayResult, []byte("epay-payload"))
	require.NoError(t, err)
	assert.True(t, handled)

	epayCanceledOrder := makeOrder("epay-canceled-order", 905, "epay", "CNY")
	handled, err = processEpayD2SPayment(&epay.VerifyRes{
		TradeNo:        "epay-canceled-event",
		ServiceTradeNo: epayCanceledOrder.ID,
		Money:          "29.90",
		TradeStatus:    "TRADE_CLOSED",
	}, []byte("epay-canceled-payload"))
	require.NoError(t, err)
	assert.True(t, handled)
	var canceledEpayOrder model.D2SOrder
	require.NoError(t, db.First(&canceledEpayOrder, "id = ?", epayCanceledOrder.ID).Error)
	assert.Equal(t, model.D2SOrderCanceled, canceledEpayOrder.Status)

	waffoOrder := makeOrder("waffo-adapter-order", 902, "waffo", "CNY")
	handled, err = processWaffoD2SPayment(&core.PaymentNotificationResult{PaymentRequestID: "waffo-event", MerchantOrderID: waffoOrder.ID, OrderStatus: core.OrderStatusPaySuccess, OrderCurrency: "CNY", OrderAmount: "29.90"}, []byte("waffo-payload"))
	require.NoError(t, err)
	assert.True(t, handled)
	refund := &core.RefundNotification{
		EventType: core.EventRefund,
		Result: &core.RefundNotificationResult{
			RefundRequestID:      "waffo-refund-event",
			OrigPaymentRequestID: "waffo-event",
			RefundAmount:         "29.90",
			RefundStatus:         core.RefundStatusFullyRefunded,
			UserCurrency:         "CNY",
		},
	}
	handled, err = processWaffoD2SRefund(refund, []byte("waffo-refund-payload"))
	require.NoError(t, err)
	assert.True(t, handled)
	var refundedOrder model.D2SOrder
	require.NoError(t, db.First(&refundedOrder, "id = ?", waffoOrder.ID).Error)
	assert.Equal(t, model.D2SOrderChargeback, refundedOrder.Status)

	waffoCanceledOrder := makeOrder("waffo-canceled-order", 906, "waffo", "CNY")
	handled, err = processWaffoD2SPayment(&core.PaymentNotificationResult{
		PaymentRequestID: "waffo-canceled-event",
		MerchantOrderID:  waffoCanceledOrder.ID,
		OrderStatus:      core.OrderStatusOrderClose,
		OrderCurrency:    "CNY",
		OrderAmount:      "29.90",
	}, []byte("waffo-canceled-payload"))
	require.NoError(t, err)
	assert.True(t, handled)
	var canceledWaffoOrder model.D2SOrder
	require.NoError(t, db.First(&canceledWaffoOrder, "id = ?", waffoCanceledOrder.ID).Error)
	assert.Equal(t, model.D2SOrderCanceled, canceledWaffoOrder.Status)

	pancakeOrder := makeOrder("pancake-adapter-order", 903, "waffo_pancake", "USD")
	handled, err = processWaffoPancakeD2SPayment(&service.WaffoPancakeWebhookEvent{EventID: "pancake-event", EventType: "order.completed", Data: service.WaffoPancakeWebhookData{OrderMerchantExternalID: pancakeOrder.ID, Currency: "USD", Amount: "29.90"}}, []byte("pancake-payload"))
	require.NoError(t, err)
	assert.True(t, handled)
	handled, err = processWaffoPancakeD2SRefund(&service.WaffoPancakeWebhookEvent{EventID: "pancake-refund-event", EventType: "refund.succeeded", Data: service.WaffoPancakeWebhookData{OrderMerchantExternalID: pancakeOrder.ID, Currency: "USD", Amount: "29.90"}}, []byte("pancake-refund-payload"))
	require.NoError(t, err)
	assert.True(t, handled)
	var refundedPancakeOrder model.D2SOrder
	require.NoError(t, db.First(&refundedPancakeOrder, "id = ?", pancakeOrder.ID).Error)
	assert.Equal(t, model.D2SOrderChargeback, refundedPancakeOrder.Status)
	handled, err = processWaffoPancakeD2SPayment(&service.WaffoPancakeWebhookEvent{
		EventID:   "pancake-pending-event",
		EventType: "checkout.created",
		Data: service.WaffoPancakeWebhookData{
			OrderMerchantExternalID: pancakeOrder.ID,
			Currency:                "USD",
			Amount:                  "29.90",
		},
	}, []byte("pancake-pending-payload"))
	require.NoError(t, err)
	assert.False(t, handled)

	badWaffoOrder := makeOrder("waffo-bad-amount-order", 904, "waffo", "CNY")
	handled, err = processWaffoD2SPayment(&core.PaymentNotificationResult{PaymentRequestID: "waffo-bad-event", MerchantOrderID: badWaffoOrder.ID, OrderStatus: core.OrderStatusPaySuccess, OrderCurrency: "CNY", OrderAmount: "29.901"}, []byte("bad-payload"))
	assert.True(t, handled)
	assert.ErrorIs(t, err, model.ErrD2SPaymentMismatch)
}

func TestD2SPaymentEventRejectsProviderRegionMismatch(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:d2s_provider_region_mismatch?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	previousDB, previousLogDB := model.DB, model.LOG_DB
	previousMainType, previousLogType := common.MainDatabaseType(), common.LogDatabaseType()
	model.DB, model.LOG_DB = db, db
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	t.Cleanup(func() {
		model.DB, model.LOG_DB = previousDB, previousLogDB
		common.SetDatabaseTypes(previousMainType, previousLogType)
	})
	require.NoError(t, db.AutoMigrate(&model.D2SOrder{}, &model.D2SPaymentEvent{}))
	require.NoError(t, db.Create(&model.D2SOrder{ID: "region-mismatch-order", UserID: 1, Product: model.D2SOrderProductLicense, Provider: "epay", Region: model.D2SRegionINTL, Currency: "CNY", AmountMinor: 2990, GatewayMinor: 2990, Status: model.D2SOrderPending, CreatedAt: 2_000_900_000, ExpiresAt: 2_000_901_800}).Error)

	_, err = model.ProcessD2SPaymentEvent("epay", "region-mismatch-event", "region-mismatch-order", "paid", 2990, "CNY", "region-mismatch-payload", 2_000_900_100)
	assert.ErrorIs(t, err, model.ErrD2SRegionMismatch)
}

func TestD2SOrderProvidersHidesBalanceUntilRegionIsLocked(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:d2s_provider_visibility_test?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	previousDB, previousLogDB := model.DB, model.LOG_DB
	previousMainType, previousLogType := common.MainDatabaseType(), common.LogDatabaseType()
	model.DB, model.LOG_DB = db, db
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	t.Cleanup(func() {
		model.DB, model.LOG_DB = previousDB, previousLogDB
		common.SetDatabaseTypes(previousMainType, previousLogType)
	})
	require.NoError(t, db.AutoMigrate(&model.D2SUserProfile{}))
	require.NoError(t, db.Create(&model.D2SUserProfile{UserID: 1201, Region: "", CreatedAt: 1, UpdatedAt: 1}).Error)
	require.NoError(t, db.Create(&model.D2SUserProfile{UserID: 1202, Region: model.D2SRegionCN, RegionLockedAt: 2, CreatedAt: 1, UpdatedAt: 2}).Error)

	providersFor := func(userID int) []string {
		recorder := httptest.NewRecorder()
		context, _ := gin.CreateTestContext(recorder)
		context.Set("id", userID)
		D2SOrderProviders(context)
		var response struct {
			Data struct {
				Providers []string `json:"providers"`
			} `json:"data"`
		}
		require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &response))
		return response.Data.Providers
	}

	assert.NotContains(t, providersFor(1201), model.D2SProviderBalance)
	assert.Contains(t, providersFor(1202), model.D2SProviderBalance)
}

func TestD2SOrderProvidersExposesConfiguredPaymentFM(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:d2s_paymentfm_provider_test?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	previousDB, previousLogDB := model.DB, model.LOG_DB
	model.DB, model.LOG_DB = db, db
	t.Cleanup(func() { model.DB, model.LOG_DB = previousDB, previousLogDB })
	require.NoError(t, db.AutoMigrate(&model.D2SUserProfile{}))
	require.NoError(t, db.Create(&model.D2SUserProfile{UserID: 1203, Region: model.D2SRegionCN, RegionLockedAt: 2, CreatedAt: 1, UpdatedAt: 2}).Error)

	confirmPaymentComplianceForTest(t)
	originalPayAddress := operation_setting.PayAddress
	originalEpayID := operation_setting.EpayId
	originalEpayKey := operation_setting.EpayKey
	originalPayMethods := operation_setting.PayMethods
	t.Cleanup(func() {
		operation_setting.PayAddress = originalPayAddress
		operation_setting.EpayId = originalEpayID
		operation_setting.EpayKey = originalEpayKey
		operation_setting.PayMethods = originalPayMethods
	})
	operation_setting.PayAddress = "https://pay.example.com"
	operation_setting.EpayId = "epay-id"
	operation_setting.EpayKey = "epay-key"
	operation_setting.PayMethods = []map[string]string{
		{"type": "paymentfm"}, {"type": "paymentfm"}, {"type": "alipay"}, {"type": "wxpay"},
	}

	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Set("id", 1203)
	D2SOrderProviders(context)
	var response struct {
		Data struct {
			Providers []string `json:"providers"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &response))
	assert.Contains(t, response.Data.Providers, "paymentfm")
	assert.Equal(t, 1, countString(response.Data.Providers, "paymentfm"))
	assert.Contains(t, response.Data.Providers, "alipay")
	assert.Contains(t, response.Data.Providers, "wechat")
}

func countString(values []string, target string) int {
	count := 0
	for _, value := range values {
		if value == target {
			count++
		}
	}
	return count
}

func TestStripeD2SPaymentEventMapsRefundAndDisputeStates(t *testing.T) {
	tests := []struct {
		name      string
		eventType stripe.EventType
		object    map[string]any
		wantType  string
		wantOK    bool
	}{
		{name: "full refund", eventType: "charge.refunded", object: map[string]any{}, wantType: "reversed", wantOK: true},
		{name: "lost dispute", eventType: "charge.dispute.created", object: map[string]any{}, wantType: "chargeback", wantOK: true},
		{name: "won dispute", eventType: "charge.dispute.closed", object: map[string]any{"status": "won"}, wantOK: false},
		{name: "async payment failed", eventType: stripe.EventTypeCheckoutSessionAsyncPaymentFailed, object: map[string]any{}, wantType: "failed", wantOK: true},
		{name: "expired checkout", eventType: stripe.EventTypeCheckoutSessionExpired, object: map[string]any{"status": "expired"}, wantType: "canceled", wantOK: true},
		{name: "unpaid checkout", eventType: stripe.EventTypeCheckoutSessionCompleted, object: map[string]any{"payment_status": "unpaid"}, wantOK: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			eventType, ok := stripeD2SPaymentEvent(stripeD2STestEvent(test.eventType, test.object))
			assert.Equal(t, test.wantOK, ok)
			assert.Equal(t, test.wantType, eventType)
		})
	}
}

func TestStripeD2SOrderIDReadsMetadataForReversalEvents(t *testing.T) {
	event := stripeD2STestEvent("charge.refunded", map[string]any{
		"metadata": map[string]string{"d2s_order_id": "d2s-order-123"},
	})
	assert.Equal(t, "d2s-order-123", stripeD2SOrderID(event))
}

func TestCreemD2SReversalDetailsUsesCheckoutRequestID(t *testing.T) {
	payload := []byte(`{"id":"refund_123","eventType":"refund.created","object":{"refund_amount":2990,"refund_currency":"USD","checkout":{"request_id":"d2s-order-123"},"transaction":{"order":"creem-order-123"}}}`)

	handled, eventID, orderID, eventType, amountMinor, currency, err := creemD2SReversalFields(payload)

	assert.True(t, handled)
	assert.NoError(t, err)
	assert.Equal(t, "refund_123", eventID)
	assert.Equal(t, "d2s-order-123", orderID)
	assert.Equal(t, "refund.created", eventType)
	assert.EqualValues(t, 2990, amountMinor)
	assert.Equal(t, "USD", currency)
}

func TestCreemD2SReversalRejectsMissingAmount(t *testing.T) {
	payload := []byte(`{"id":"dispute_123","eventType":"dispute.created","object":{"currency":"USD","checkout":{"request_id":"d2s-order-123"}}}`)

	handled, _, _, _, _, _, err := creemD2SReversalFields(payload)

	assert.True(t, handled)
	assert.ErrorIs(t, err, model.ErrD2SPaymentMismatch)
}

func TestCreemD2SPaidFieldsNormalizeProviderValues(t *testing.T) {
	event := &CreemWebhookEvent{}
	event.Id = "  creem-event  "
	event.Object.RequestId = "  d2s-order  "
	event.Object.Order.AmountPaid = 2990
	event.Object.Order.Currency = " usd "

	eventID, orderID, amountMinor, currency := creemD2SPaidFields(event)

	assert.Equal(t, "creem-event", eventID)
	assert.Equal(t, "d2s-order", orderID)
	assert.EqualValues(t, 2990, amountMinor)
	assert.Equal(t, "USD", currency)
}

func TestWaffoD2SRefundRejectsPartialRefund(t *testing.T) {
	notification := &core.RefundNotification{
		EventType: core.EventRefund,
		Result: &core.RefundNotificationResult{
			RefundStatus: core.RefundStatusPartiallyRefunded,
		},
	}

	handled, err := processWaffoD2SRefund(notification, []byte(`{"eventType":"REFUND_NOTIFICATION"}`))

	assert.True(t, handled)
	assert.ErrorIs(t, err, model.ErrD2SPaymentMismatch)
}

func TestD2SCheckoutRejectsStripeOutsideINTL(t *testing.T) {
	_, err := createD2SStripeCheckout(&model.D2SOrder{
		Provider:     "stripe",
		Region:       model.D2SRegionCN,
		Currency:     "CNY",
		GatewayMinor: 9900,
		Status:       model.D2SOrderPending,
	}, &model.User{})
	assert.ErrorIs(t, err, model.ErrD2SCheckoutUnavailable)
}
