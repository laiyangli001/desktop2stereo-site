package controller

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/Calcium-Ion/go-epay/epay"
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stripe/stripe-go/v81"
	"github.com/stripe/stripe-go/v81/webhook"
	"github.com/waffo-com/waffo-go/core"
	waffoutils "github.com/waffo-com/waffo-go/utils"
	"gorm.io/gorm"
)

func stripeD2STestEvent(eventType stripe.EventType, object map[string]any) stripe.Event {
	return stripeD2STestEventWithID("evt_d2s_test", eventType, object)
}

func stripeD2STestEventWithID(id string, eventType stripe.EventType, object map[string]any) stripe.Event {
	raw, _ := json.Marshal(object)
	var objectMap map[string]interface{}
	_ = json.Unmarshal(raw, &objectMap)
	return stripe.Event{
		ID:   id,
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

	missingSecretBody := `{"event_id":"evt","order_id":"order","event_type":"paid","amount_minor":2990,"currency":"USD"}`
	require.NoError(t, os.Unsetenv("D2S_PAYMENT_BRIDGE_SECRET"))
	assert.Equal(t, http.StatusBadRequest, call(missingSecretBody, validD2SBridgeSignature(missingSecretBody, "bridge-test-secret")).Code)
	require.NoError(t, os.Setenv("D2S_PAYMENT_BRIDGE_SECRET", "bridge-test-secret"))
	require.NoError(t, os.Setenv("D2S_PAYMENT_BRIDGE_SECRET_STRIPE", "provider-specific-secret"))
	t.Cleanup(func() { _ = os.Unsetenv("D2S_PAYMENT_BRIDGE_SECRET_STRIPE") })
	assert.Equal(t, http.StatusBadRequest, call(missingSecretBody, validD2SBridgeSignature(missingSecretBody, "bridge-test-secret")).Code)
	providerSecretBody := `{"event_id":"provider-event","order_id":"missing-provider-order","event_type":"paid","amount_minor":2990,"currency":"USD"}`
	providerSecretResponse := call(providerSecretBody, validD2SBridgeSignature(providerSecretBody, "provider-specific-secret"))
	assert.Equal(t, http.StatusNotFound, providerSecretResponse.Code)
	require.NoError(t, os.Unsetenv("D2S_PAYMENT_BRIDGE_SECRET_STRIPE"))
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

func TestD2SPaymentWebhookMultiProviderBridge(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:d2s_multi_provider_bridge_test?mode=memory&cache=shared"), &gorm.Config{})
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
		&model.D2SOrder{}, &model.D2SPaymentEvent{}, &model.D2SInviteReward{},
		&model.D2SBalanceAccount{}, &model.D2SBalanceTransaction{},
	))

	const now = int64(2_001_000_000)
	type bridgeCase struct {
		provider   string
		secretName string
		currency   string
		amount     int64
	}
	cases := []bridgeCase{
		{provider: "epay", secretName: "D2S_PAYMENT_BRIDGE_SECRET_EPAY", currency: "CNY", amount: 9900},
		{provider: "paymentfm", secretName: "D2S_PAYMENT_BRIDGE_SECRET_PAYMENTFM", currency: "CNY", amount: 9900},
		{provider: "alipay", secretName: "D2S_PAYMENT_BRIDGE_SECRET_ALIPAY", currency: "CNY", amount: 9900},
		{provider: "wechat", secretName: "D2S_PAYMENT_BRIDGE_SECRET_WECHAT", currency: "CNY", amount: 9900},
		{provider: "waffo", secretName: "D2S_PAYMENT_BRIDGE_SECRET_WAFFO", currency: "CNY", amount: 9900},
		{provider: "stripe", secretName: "D2S_PAYMENT_BRIDGE_SECRET_STRIPE", currency: "USD", amount: 2990},
		{provider: "creem", secretName: "D2S_PAYMENT_BRIDGE_SECRET_CREEM", currency: "USD", amount: 2990},
		{provider: "waffo_pancake", secretName: "D2S_PAYMENT_BRIDGE_SECRET_WAFFO_PANCAKE", currency: "USD", amount: 2990},
	}

	// Force every case to use its provider-specific secret instead of the shared fallback.
	t.Setenv("D2S_PAYMENT_BRIDGE_SECRET", "")
	for _, testCase := range cases {
		t.Setenv(testCase.secretName, "")
	}

	call := func(provider, body, secret string) *httptest.ResponseRecorder {
		recorder := httptest.NewRecorder()
		context, _ := gin.CreateTestContext(recorder)
		context.Request = httptest.NewRequest(http.MethodPost, "/api/v1/webhooks/"+provider, bytes.NewBufferString(body))
		context.Params = gin.Params{{Key: "provider", Value: provider}}
		context.Request.Header.Set("X-D2S-Signature", validD2SBridgeSignature(body, secret))
		D2SPaymentWebhook(context)
		return recorder
	}

	for index, testCase := range cases {
		t.Run(testCase.provider, func(t *testing.T) {
			secret := "provider-bridge-secret-" + testCase.provider
			t.Setenv(testCase.secretName, secret)
			orderID := "multi-provider-" + testCase.provider
			eventID := "multi-provider-event-" + testCase.provider
			userID := 1401 + index
			require.NoError(t, db.Create(&model.User{
				Id: userID, Username: "multi-provider-" + testCase.provider,
				Email: testCase.provider + "@example.com", Password: "unused-hash",
				AffCode: "multi-provider-aff-" + testCase.provider,
				Role:    common.RoleCommonUser, Status: common.UserStatusEnabled,
				Group: "default", AuthVersion: 1,
			}).Error)
			require.NoError(t, db.Create(&model.D2SUserProfile{
				UserID: userID, EmailVerified: true, CreatedAt: now, UpdatedAt: now,
			}).Error)
			require.NoError(t, db.Create(&model.D2SOrder{
				ID: orderID, UserID: userID, Product: model.D2SOrderProductLicense,
				Provider: testCase.provider, Region: func() string {
					region, _ := model.D2SProviderRegion(testCase.provider)
					return region
				}(), Currency: testCase.currency, AmountMinor: testCase.amount,
				GatewayMinor: testCase.amount, Status: model.D2SOrderPending,
				CreatedAt: now, ExpiresAt: now + 1800,
			}).Error)

			bodyBytes, err := json.Marshal(d2sPaymentBridgeEvent{
				EventID: eventID, OrderID: orderID, EventType: "paid",
				AmountMinor: testCase.amount, Currency: testCase.currency,
			})
			require.NoError(t, err)
			body := string(bodyBytes)
			assert.Equal(t, http.StatusOK, call(testCase.provider, body, secret).Code)
			assert.Equal(t, http.StatusOK, call(testCase.provider, body, secret).Code)
			assert.Equal(t, http.StatusBadRequest, call(testCase.provider, body, "wrong-secret").Code)

			var eventCount int64
			require.NoError(t, db.Model(&model.D2SPaymentEvent{}).
				Where("provider_event_id = ?", eventID).Count(&eventCount).Error)
			assert.EqualValues(t, 1, eventCount)
			var order model.D2SOrder
			require.NoError(t, db.First(&order, "id = ?", orderID).Error)
			assert.Equal(t, model.D2SOrderPaid, order.Status)
		})
	}
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
		if providerRegion, ok := model.D2SProviderRegion(provider); ok {
			region = providerRegion
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
	partialRefund := &epay.VerifyRes{
		TradeNo:        "epay-refund-event",
		ServiceTradeNo: epayOrder.ID,
		Money:          "1.00",
		TradeStatus:    "TRADE_REFUND",
	}
	handled, err = processEpayD2SPayment(partialRefund, []byte("epay-refund-payload"))
	assert.True(t, handled)
	assert.ErrorIs(t, err, model.ErrD2SPaymentMismatch)
	var partiallyRefundedEpayOrder model.D2SOrder
	require.NoError(t, db.First(&partiallyRefundedEpayOrder, "id = ?", epayOrder.ID).Error)
	assert.Equal(t, model.D2SOrderPaid, partiallyRefundedEpayOrder.Status)
	fullRefund := &epay.VerifyRes{
		TradeNo:        "epay-full-refund-event",
		ServiceTradeNo: epayOrder.ID,
		Money:          "29.90",
		TradeStatus:    "TRADE_REFUND",
	}
	handled, err = processEpayD2SPayment(fullRefund, []byte("epay-full-refund-payload"))
	assert.True(t, handled)
	require.NoError(t, err)
	var fullyRefundedEpayOrder model.D2SOrder
	require.NoError(t, db.First(&fullyRefundedEpayOrder, "id = ?", epayOrder.ID).Error)
	assert.Equal(t, model.D2SOrderChargeback, fullyRefundedEpayOrder.Status)

	epayClosedPaidOrder := makeOrder("epay-closed-paid-order", 909, "epay", "CNY")
	handled, err = processEpayD2SPayment(&epay.VerifyRes{
		TradeNo:        "epay-closed-paid-event",
		ServiceTradeNo: epayClosedPaidOrder.ID,
		Money:          "29.90",
		TradeStatus:    epay.StatusTradeSuccess,
	}, []byte("epay-closed-paid-payload"))
	require.NoError(t, err)
	assert.True(t, handled)
	handled, err = processEpayD2SPayment(&epay.VerifyRes{
		TradeNo:        "epay-closed-after-paid-event",
		ServiceTradeNo: epayClosedPaidOrder.ID,
		Money:          "29.90",
		TradeStatus:    "TRADE_CLOSED",
	}, []byte("epay-closed-after-paid-payload"))
	assert.True(t, handled)
	assert.ErrorIs(t, err, model.ErrD2SOrderState)
	var closedPaidEpayOrder model.D2SOrder
	require.NoError(t, db.First(&closedPaidEpayOrder, "id = ?", epayClosedPaidOrder.ID).Error)
	assert.Equal(t, model.D2SOrderPaid, closedPaidEpayOrder.Status)

	for index, provider := range []string{"paymentfm", "alipay", "wechat"} {
		order := makeOrder("epay-method-"+provider, 910+index, provider, "CNY")
		handled, err = processEpayD2SPayment(&epay.VerifyRes{
			TradeNo:        "epay-method-event-" + provider,
			ServiceTradeNo: order.ID,
			Money:          "29.90",
			TradeStatus:    epay.StatusTradeSuccess,
		}, []byte("epay-method-payload-"+provider))
		require.NoError(t, err)
		assert.True(t, handled)
		var settled model.D2SOrder
		require.NoError(t, db.First(&settled, "id = ?", order.ID).Error)
		assert.Equal(t, model.D2SOrderPaid, settled.Status)
	}

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
			UserCurrency:         "cny",
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
	pancakePaidEvent := &service.WaffoPancakeWebhookEvent{EventID: "pancake-event", EventType: "order.completed", Data: service.WaffoPancakeWebhookData{OrderMerchantExternalID: pancakeOrder.ID, BuyerEmail: "PancakeBuyer@Example.com", Currency: "USD", Amount: "29.90"}}
	handled, err = processWaffoPancakeD2SPayment(pancakePaidEvent, []byte("pancake-payload"))
	require.NoError(t, err)
	assert.True(t, handled)
	handled, err = processWaffoPancakeD2SPayment(pancakePaidEvent, []byte("pancake-payload"))
	require.NoError(t, err)
	assert.True(t, handled, "repeated Pancake order.completed must be idempotent")
	conflictingPancakeEvent := &service.WaffoPancakeWebhookEvent{EventID: "pancake-event", EventType: "order.completed", Data: service.WaffoPancakeWebhookData{OrderMerchantExternalID: pancakeOrder.ID, Currency: "USD", Amount: "29.91"}}
	handled, err = processWaffoPancakeD2SPayment(conflictingPancakeEvent, []byte("pancake-conflicting-payload"))
	assert.True(t, handled)
	assert.ErrorIs(t, err, model.ErrD2SPaymentMismatch)
	handled, err = processWaffoPancakeD2SRefund(&service.WaffoPancakeWebhookEvent{EventID: "pancake-refund-event", EventType: "refund.succeeded", Data: service.WaffoPancakeWebhookData{OrderMerchantExternalID: pancakeOrder.ID, Currency: "USD", Amount: "29.90"}}, []byte("pancake-refund-payload"))
	require.NoError(t, err)
	assert.True(t, handled)
	var refundedPancakeOrder model.D2SOrder
	require.NoError(t, db.First(&refundedPancakeOrder, "id = ?", pancakeOrder.ID).Error)
	assert.Equal(t, model.D2SOrderChargeback, refundedPancakeOrder.Status)
	require.NotNil(t, refundedPancakeOrder.PaymentAccountIdentifierHash)
	assert.Equal(t, waffoPancakeD2SPaymentAccountIdentifierHash(pancakePaidEvent), *refundedPancakeOrder.PaymentAccountIdentifierHash)
	assert.NotContains(t, *refundedPancakeOrder.PaymentAccountIdentifierHash, "pancakebuyer@example.com")

	lowercaseCurrencyOrder := makeOrder("pancake-lowercase-refund-order", 907, "waffo_pancake", "USD")
	handled, err = processWaffoPancakeD2SPayment(&service.WaffoPancakeWebhookEvent{EventID: "pancake-lowercase-paid-event", EventType: "order.completed", Data: service.WaffoPancakeWebhookData{OrderMerchantExternalID: lowercaseCurrencyOrder.ID, Currency: "usd", Amount: "29.90"}}, []byte("pancake-lowercase-paid-payload"))
	require.NoError(t, err)
	assert.True(t, handled)
	handled, err = processWaffoPancakeD2SRefund(&service.WaffoPancakeWebhookEvent{EventID: "pancake-lowercase-refund-event", EventType: "refund.succeeded", Data: service.WaffoPancakeWebhookData{OrderMerchantExternalID: lowercaseCurrencyOrder.ID, Currency: "usd", Amount: "29.90"}}, []byte("pancake-lowercase-refund-payload"))
	require.NoError(t, err)
	assert.True(t, handled)
	var lowercaseRefundedOrder model.D2SOrder
	require.NoError(t, db.First(&lowercaseRefundedOrder, "id = ?", lowercaseCurrencyOrder.ID).Error)
	assert.Equal(t, model.D2SOrderChargeback, lowercaseRefundedOrder.Status)

	creemOrder := makeOrder("creem-reversal-order", 908, "creem", "USD")
	handled, err = processVerifiedD2SPayment("creem", "creem-paid-event", creemOrder.ID, "paid", 2990, "USD", []byte("creem-paid-payload"))
	require.NoError(t, err)
	assert.True(t, handled)
	handled, err = processCreemD2SReversal([]byte(`{"id":"creem-refund-event","eventType":"refund.created","object":{"refund_amount":2990,"refund_currency":"usd","checkout":{"request_id":"creem-reversal-order"}}}`))
	require.NoError(t, err)
	assert.True(t, handled)
	var reversedCreemOrder model.D2SOrder
	require.NoError(t, db.First(&reversedCreemOrder, "id = ?", creemOrder.ID).Error)
	assert.Equal(t, model.D2SOrderChargeback, reversedCreemOrder.Status)
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

func TestEpayWebhookLogSummaryExcludesSensitiveFields(t *testing.T) {
	params := map[string]string{
		"out_trade_no": "d2s-order-1",
		"trade_no":     "gateway-order-1",
		"type":         "alipay",
		"money":        "29.90",
		"trade_status": "TRADE_SUCCESS",
		"buyer":        "buyer@example.test",
		"param":        "internal-user-data",
		"sign":         "provider-signature",
	}

	assert.Equal(t, map[string]string{
		"out_trade_no": "d2s-order-1",
		"trade_no":     "gateway-order-1",
		"type":         "alipay",
		"money":        "29.90",
		"trade_status": "TRADE_SUCCESS",
	}, epayWebhookLogSummary(params))
}

func TestEpayNotifyD2SRejectsInvalidSignatureAndSettlesValidEvent(t *testing.T) {
	confirmPaymentComplianceForTest(t)
	previousPayAddress := operation_setting.PayAddress
	previousEpayID := operation_setting.EpayId
	previousEpayKey := operation_setting.EpayKey
	previousPayMethods := operation_setting.PayMethods
	t.Cleanup(func() {
		operation_setting.PayAddress = previousPayAddress
		operation_setting.EpayId = previousEpayID
		operation_setting.EpayKey = previousEpayKey
		operation_setting.PayMethods = previousPayMethods
	})
	operation_setting.PayAddress = "https://pay.example.test"
	operation_setting.EpayId = "epay_test_merchant"
	operation_setting.EpayKey = "epay_test_signing_key"
	operation_setting.PayMethods = []map[string]string{{"type": "alipay"}}

	db, err := gorm.Open(sqlite.Open("file:d2s_epay_webhook_signature_test?mode=memory&cache=shared"), &gorm.Config{})
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
		&model.D2SOrder{}, &model.D2SPaymentEvent{}, &model.D2SInviteReward{},
		&model.D2SBalanceAccount{}, &model.D2SBalanceTransaction{},
	))

	now := time.Now().Unix()
	require.NoError(t, db.Create(&model.User{
		Id: 1903, Username: "epay-webhook-user", Email: "epay-webhook@example.com",
		Password: "unused-hash", Role: common.RoleCommonUser, Status: common.UserStatusEnabled,
		Group: "default", AuthVersion: 1,
	}).Error)
	require.NoError(t, db.Create(&model.D2SUserProfile{
		UserID: 1903, EmailVerified: true, CreatedAt: now, UpdatedAt: now,
	}).Error)
	require.NoError(t, db.Create(&model.D2SOrder{
		ID: "epay-webhook-order", UserID: 1903, Product: model.D2SOrderProductLicense,
		Provider: "epay", Region: model.D2SRegionCN, Currency: "CNY",
		AmountMinor: 2990, GatewayMinor: 2990, Status: model.D2SOrderPending,
		CreatedAt: now, ExpiresAt: now + 3600,
	}).Error)

	params := epay.GenerateParams(map[string]string{
		"pid": operation_setting.EpayId, "type": "alipay", "trade_no": "epay-trade-1903",
		"out_trade_no": "epay-webhook-order", "money": "29.90", "trade_status": "TRADE_SUCCESS",
	}, operation_setting.EpayKey)
	call := func(valuesMap map[string]string) *httptest.ResponseRecorder {
		values := url.Values{}
		for key, value := range valuesMap {
			values.Set(key, value)
		}
		request := httptest.NewRequest(http.MethodPost, "/api/v1/epay/notify", bytes.NewBufferString(values.Encode()))
		request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		recorder := httptest.NewRecorder()
		context, _ := gin.CreateTestContext(recorder)
		context.Request = request
		EpayNotify(context)
		return recorder
	}

	invalidParams := make(map[string]string, len(params))
	for key, value := range params {
		invalidParams[key] = value
	}
	invalidParams["sign"] = "invalid-signature"
	invalidResponse := call(invalidParams)
	require.Equal(t, http.StatusOK, invalidResponse.Code)
	assert.Equal(t, "fail", invalidResponse.Body.String())
	var eventCount int64
	require.NoError(t, db.Model(&model.D2SPaymentEvent{}).Count(&eventCount).Error)
	assert.Zero(t, eventCount)
	var order model.D2SOrder
	require.NoError(t, db.First(&order, "id = ?", "epay-webhook-order").Error)
	assert.Equal(t, model.D2SOrderPending, order.Status)

	require.Equal(t, http.StatusOK, call(params).Code)
	require.Equal(t, http.StatusOK, call(params).Code)
	require.NoError(t, db.Model(&model.D2SPaymentEvent{}).Count(&eventCount).Error)
	assert.EqualValues(t, 1, eventCount)
	require.NoError(t, db.First(&order, "id = ?", "epay-webhook-order").Error)
	assert.Equal(t, model.D2SOrderPaid, order.Status)
}

func TestEpayD2SAdapterRejectsNonEpayOrder(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:d2s_epay_cross_provider_test?mode=memory&cache=shared"), &gorm.Config{})
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
	require.NoError(t, db.Create(&model.D2SOrder{
		ID: "epay-cross-provider-order", UserID: 1, Product: model.D2SOrderProductLicense,
		Provider: "waffo", Region: model.D2SRegionCN, Currency: "CNY", AmountMinor: 2990,
		GatewayMinor: 2990, Status: model.D2SOrderPending, CreatedAt: 1, ExpiresAt: 1801,
	}).Error)

	handled, err := processEpayD2SPayment(&epay.VerifyRes{
		TradeNo: "epay-cross-provider-event", ServiceTradeNo: "epay-cross-provider-order",
		Money: "29.90", TradeStatus: epay.StatusTradeSuccess,
	}, []byte("cross-provider-payload"))

	assert.True(t, handled)
	assert.ErrorIs(t, err, model.ErrD2SPaymentMismatch)
	var eventCount int64
	require.NoError(t, db.Model(&model.D2SPaymentEvent{}).Count(&eventCount).Error)
	assert.Zero(t, eventCount)
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

func TestD2SEpayCheckoutRetryKeepsStableSignedOrderIdentity(t *testing.T) {
	originalPayAddress := operation_setting.PayAddress
	originalEpayID := operation_setting.EpayId
	originalEpayKey := operation_setting.EpayKey
	t.Cleanup(func() {
		operation_setting.PayAddress = originalPayAddress
		operation_setting.EpayId = originalEpayID
		operation_setting.EpayKey = originalEpayKey
	})
	operation_setting.PayAddress = "https://gateway.example"
	operation_setting.EpayId = "test-merchant"
	operation_setting.EpayKey = "test-only-signing-key"

	order := &model.D2SOrder{
		ID: "d2s-retry-order-001", Provider: "epay", Region: model.D2SRegionCN,
		Currency: "CNY", GatewayMinor: 1990,
	}
	first, err := createD2SEpayCheckout(order)
	require.NoError(t, err)
	second, err := createD2SEpayCheckout(order)
	require.NoError(t, err)

	assert.Equal(t, first.URL, second.URL)
	assert.Equal(t, first.Params, second.Params)
	assert.Equal(t, order.ID, first.Params["out_trade_no"])
	assert.Equal(t, "19.90", first.Params["money"])
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

func TestStripeD2SPaymentAccountIdentifierHash(t *testing.T) {
	previousSecret := common.SessionSecret
	common.SessionSecret = "stripe-account-hash-test-secret"
	t.Cleanup(func() { common.SessionSecret = previousSecret })

	tests := []struct {
		name   string
		object map[string]any
		input  string
	}{
		{name: "customer ID", object: map[string]any{"customer": "cus_test_123"}, input: "stripe|customer|cus_test_123"},
		{name: "customer details email", object: map[string]any{"customer_details": map[string]string{"email": "Buyer@Example.com"}}, input: "stripe|email|buyer@example.com"},
		{name: "checkout email fallback", object: map[string]any{"customer_email": "buyer@example.com"}, input: "stripe|email|buyer@example.com"},
		{name: "missing identifier", object: map[string]any{}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			event := stripeD2STestEvent(stripe.EventTypeCheckoutSessionCompleted, test.object)
			got := stripeD2SPaymentAccountIdentifierHash(event)
			if test.input == "" {
				assert.Empty(t, got)
				return
			}
			expected := common.GenerateHMACWithKey(
				[]byte("d2s-payment-account-v1:"+common.SessionSecret),
				test.input,
			)
			assert.Equal(t, expected, got)
			assert.NotEqual(t, common.GenerateHMACWithKey([]byte("other-secret"), test.input), got)
			assert.NotContains(t, got, "buyer@example.com")
		})
	}
}

func TestStripeD2SOrderIDReadsMetadataForReversalEvents(t *testing.T) {
	event := stripeD2STestEvent("charge.refunded", map[string]any{
		"metadata": map[string]string{"d2s_order_id": "d2s-order-123"},
	})
	assert.Equal(t, "d2s-order-123", stripeD2SOrderID(event))
}

func TestStripeD2SAdapterSettlesFullRefundThroughSharedTransaction(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:d2s_stripe_reversal_integration?mode=memory&cache=shared"), &gorm.Config{})
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
		&model.User{}, &model.D2SUserProfile{}, &model.D2SLicense{},
		&model.D2SLicenseEvent{}, &model.D2SOrder{}, &model.D2SPaymentEvent{},
		&model.D2SBalanceAccount{}, &model.D2SBalanceTransaction{}, &model.D2SInviteReward{},
	))
	now := int64(2_000_950_000)
	require.NoError(t, db.Create(&model.User{
		Id: 1310, Username: "stripe-reversal", Email: "stripe-reversal@example.com",
		Password: "unused-hash", Role: common.RoleCommonUser, Status: common.UserStatusEnabled,
		Group: "default", AuthVersion: 1,
	}).Error)
	require.NoError(t, db.Create(&model.D2SUserProfile{
		UserID: 1310, EmailVerified: true, CreatedAt: now, UpdatedAt: now,
	}).Error)
	require.NoError(t, db.Create(&model.D2SOrder{
		ID: "stripe-reversal-order", UserID: 1310, Product: model.D2SOrderProductLicense,
		Provider: "stripe", Region: model.D2SRegionINTL, Currency: "USD", AmountMinor: 2990,
		GatewayMinor: 2990, Status: model.D2SOrderPending, CreatedAt: now, ExpiresAt: now + 1800,
	}).Error)

	paid := stripeD2STestEventWithID("evt_stripe_paid", stripe.EventTypeCheckoutSessionCompleted, map[string]any{
		"client_reference_id": "stripe-reversal-order",
		"payment_status":      "paid",
		"customer":            "cus_d2s_test",
		"amount_total":        "2990",
		"currency":            "usd",
	})
	handled, err := processStripeD2SPayment(paid, []byte(`{"id":"evt_stripe_paid"}`))
	require.NoError(t, err)
	assert.True(t, handled)

	refund := stripeD2STestEventWithID("evt_stripe_refund", "charge.refunded", map[string]any{
		"metadata":        map[string]string{"d2s_order_id": "stripe-reversal-order"},
		"amount_refunded": "2990",
		"currency":        "usd",
	})
	handled, err = processStripeD2SPayment(refund, []byte(`{"id":"evt_stripe_refund"}`))
	require.NoError(t, err)
	assert.True(t, handled)

	var order model.D2SOrder
	require.NoError(t, db.First(&order, "id = ?", "stripe-reversal-order").Error)
	assert.Equal(t, model.D2SOrderChargeback, order.Status)
	expectedAccountHash := stripeD2SPaymentAccountIdentifierHash(paid)
	require.NotNil(t, order.PaymentAccountIdentifierHash)
	assert.Equal(t, expectedAccountHash, *order.PaymentAccountIdentifierHash)
	assert.NotContains(t, *order.PaymentAccountIdentifierHash, "cus_d2s_test")
	var eventCount int64
	require.NoError(t, db.Model(&model.D2SPaymentEvent{}).Where("order_id = ?", order.ID).Count(&eventCount).Error)
	assert.EqualValues(t, 2, eventCount)
}

func TestCreemD2SReversalDetailsUsesCheckoutRequestID(t *testing.T) {
	payload := []byte(`{"id":"refund_123","eventType":"refund.created","object":{"refund_amount":2990,"refund_currency":"usd","checkout":{"request_id":"d2s-order-123"},"transaction":{"order":"creem-order-123"}}}`)

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

func TestWaffoPancakeD2SRefundRejectsPartialRefund(t *testing.T) {
	event := &service.WaffoPancakeWebhookEvent{
		EventType: "refund.succeeded",
		Data: service.WaffoPancakeWebhookData{
			RefundStatus: core.RefundStatusPartiallyRefunded,
		},
	}

	handled, err := processWaffoPancakeD2SRefund(event, []byte(`{"eventType":"refund.succeeded"}`))

	assert.True(t, handled)
	assert.ErrorIs(t, err, model.ErrD2SPaymentMismatch)
}

func TestD2SStripeCheckoutUsesOrderScopedIdempotencyKey(t *testing.T) {
	previousAPIBackend := stripe.GetBackend(stripe.APIBackend)
	previousStripeKey := stripe.Key
	previousStripeSecret := setting.StripeApiSecret
	t.Cleanup(func() {
		stripe.SetBackend(stripe.APIBackend, previousAPIBackend)
		stripe.Key = previousStripeKey
		setting.StripeApiSecret = previousStripeSecret
	})

	var requestCount int
	var idempotencyKeys []string
	var checkoutEmails []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		_ = r.ParseForm()
		checkoutEmails = append(checkoutEmails, r.Form.Get("customer_email"))
		idempotencyKeys = append(idempotencyKeys, r.Header.Get("Idempotency-Key"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"cs_test_d2s","object":"checkout.session","url":"https://checkout.stripe.test/session"}`))
	}))
	t.Cleanup(server.Close)

	stripe.SetBackend(stripe.APIBackend, stripe.GetBackendWithConfig(stripe.APIBackend, &stripe.BackendConfig{
		URL: stripe.String(server.URL), HTTPClient: server.Client(), MaxNetworkRetries: stripe.Int64(0),
	}))
	setting.StripeApiSecret = "sk_test_d2s_checkout"
	order := &model.D2SOrder{
		ID: "order-stable-checkout-key", Provider: "stripe", Region: model.D2SRegionINTL,
		Currency: "USD", GatewayMinor: 2990, EmailSnapshot: "purchased@example.com",
	}
	user := &model.User{Email: "changed-after-purchase@example.com"}

	firstURL, err := createD2SStripeCheckout(order, user)
	require.NoError(t, err)
	secondURL, err := createD2SStripeCheckout(order, user)
	require.NoError(t, err)

	assert.Equal(t, firstURL, secondURL)
	assert.Equal(t, 2, requestCount)
	require.Len(t, idempotencyKeys, 2)
	assert.Equal(t, "d2s-checkout-"+order.ID, idempotencyKeys[0])
	assert.Equal(t, idempotencyKeys[0], idempotencyKeys[1])
	require.Equal(t, []string{"purchased@example.com", "purchased@example.com"}, checkoutEmails)
}

func TestD2SOrderCheckoutEmailUsesOrderSnapshot(t *testing.T) {
	order := &model.D2SOrder{EmailSnapshot: "purchased@example.com"}
	user := &model.User{Email: "changed@example.com"}

	assert.Equal(t, "purchased@example.com", d2SOrderCheckoutEmail(order, user))
	assert.Equal(t, "legacy@example.com", d2SOrderCheckoutEmail(&model.D2SOrder{}, &model.User{Email: "legacy@example.com"}))
	assert.Empty(t, d2SOrderCheckoutEmail(nil, nil))
}

func TestD2SWaffoPaymentRequestIDIsStableAndBounded(t *testing.T) {
	orderID := "3a5f48cd-7190-4f5b-a2bb-1da87f630d2e"
	assert.Equal(t, "3a5f48cd71904f5ba2bb1da87f630d2e", d2SWaffoPaymentRequestID(orderID))
	assert.Equal(t, d2SWaffoPaymentRequestID(orderID), d2SWaffoPaymentRequestID(" "+orderID+" "))

	nonUUID := d2SWaffoPaymentRequestID("legacy-order-reference-with-an-arbitrary-length")
	assert.Len(t, nonUUID, 32)
	assert.Equal(t, nonUUID, d2SWaffoPaymentRequestID("legacy-order-reference-with-an-arbitrary-length"))
	assert.NotEqual(t, nonUUID, d2SWaffoPaymentRequestID("another-order-reference"))
	assert.Empty(t, d2SWaffoPaymentRequestID("  "))
}

type d2SCreemTestRoundTripper func(*http.Request) (*http.Response, error)

func (roundTripper d2SCreemTestRoundTripper) RoundTrip(request *http.Request) (*http.Response, error) {
	return roundTripper(request)
}

func TestCreemCheckoutLogsOmitEmailAndCheckoutURL(t *testing.T) {
	previousTransport := http.DefaultTransport
	previousWriter := gin.DefaultWriter
	previousAPIKey := setting.CreemApiKey
	previousTestMode := setting.CreemTestMode
	var logOutput bytes.Buffer

	http.DefaultTransport = d2SCreemTestRoundTripper(func(request *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       io.NopCloser(bytes.NewBufferString(`{"id":"creem-response-123","checkout_url":"https://checkout.creem.test/session?token=private-checkout-token"}`)),
			Request:    request,
		}, nil
	})
	gin.DefaultWriter = &logOutput
	setting.CreemApiKey = "creem_test_key"
	setting.CreemTestMode = true
	t.Cleanup(func() {
		http.DefaultTransport = previousTransport
		gin.DefaultWriter = previousWriter
		setting.CreemApiKey = previousAPIKey
		setting.CreemTestMode = previousTestMode
	})

	checkoutURL, err := genCreemLink(context.Background(), "d2s-order-123", &CreemProduct{
		ProductId: "d2s-product-123",
		Name:      "Desktop2Stereo license",
	}, "private-buyer@example.com", "d2s-user-1901")
	require.NoError(t, err)
	assert.Equal(t, "https://checkout.creem.test/session?token=private-checkout-token", checkoutURL)
	assert.Contains(t, logOutput.String(), "email_present=true")
	assert.Contains(t, logOutput.String(), "checkout_url_present=true")
	assert.NotContains(t, logOutput.String(), "private-buyer@example.com")
	assert.NotContains(t, logOutput.String(), "private-checkout-token")
	assert.NotContains(t, logOutput.String(), "https://checkout.creem.test/session")
}

func TestD2SRequestLogPathOmitsQueryParameters(t *testing.T) {
	context, _ := gin.CreateTestContext(httptest.NewRecorder())
	context.Request = httptest.NewRequest(http.MethodPost, "/api/creem/webhook?sign=private-signature&param=private-value", nil)

	assert.Equal(t, "/api/creem/webhook", d2SRequestLogPath(context))
}

func TestStripeWebhookD2SRejectsInvalidSignatureAndSettlesValidEvent(t *testing.T) {
	confirmPaymentComplianceForTest(t)
	previousAPISecret := setting.StripeApiSecret
	previousWebhookSecret := setting.StripeWebhookSecret
	previousPriceID := setting.StripePriceId
	t.Cleanup(func() {
		setting.StripeApiSecret = previousAPISecret
		setting.StripeWebhookSecret = previousWebhookSecret
		setting.StripePriceId = previousPriceID
	})
	setting.StripeApiSecret = "sk_test_d2s_webhook"
	setting.StripeWebhookSecret = "whsec_d2s_webhook"
	setting.StripePriceId = "price_d2s_webhook"

	db, err := gorm.Open(sqlite.Open("file:d2s_stripe_webhook_signature_test?mode=memory&cache=shared"), &gorm.Config{})
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
		&model.D2SOrder{}, &model.D2SPaymentEvent{}, &model.D2SInviteReward{},
		&model.D2SBalanceAccount{}, &model.D2SBalanceTransaction{},
	))

	now := time.Now().Unix()
	require.NoError(t, db.Create(&model.User{
		Id: 1901, Username: "stripe-webhook-user", Email: "stripe-webhook@example.com",
		Password: "unused-hash", Role: common.RoleCommonUser, Status: common.UserStatusEnabled,
		Group: "default", AuthVersion: 1,
	}).Error)
	require.NoError(t, db.Create(&model.D2SUserProfile{
		UserID: 1901, EmailVerified: true, CreatedAt: now, UpdatedAt: now,
	}).Error)
	require.NoError(t, db.Create(&model.D2SOrder{
		ID: "stripe-webhook-order", UserID: 1901, Product: model.D2SOrderProductLicense,
		Provider: "stripe", Region: model.D2SRegionINTL, Currency: "USD",
		AmountMinor: 2990, GatewayMinor: 2990, Status: model.D2SOrderPending,
		CreatedAt: now, ExpiresAt: now + 3600,
	}).Error)

	payload := []byte(`{"id":"evt_d2s_signed","object":"event","type":"checkout.session.completed","data":{"object":{"id":"cs_d2s_signed","object":"checkout.session","client_reference_id":"stripe-webhook-order","payment_status":"paid","amount_total":2990,"currency":"usd"}}}`)
	signed := webhook.GenerateTestSignedPayload(&webhook.UnsignedPayload{
		Payload: payload, Secret: setting.StripeWebhookSecret, Timestamp: time.Now(),
	})
	require.NotNil(t, signed)
	call := func(signature string) *httptest.ResponseRecorder {
		request := httptest.NewRequest(http.MethodPost, "/api/v1/stripe/webhook", bytes.NewReader(payload))
		request.Header.Set("Stripe-Signature", signature)
		recorder := httptest.NewRecorder()
		context, _ := gin.CreateTestContext(recorder)
		context.Request = request
		StripeWebhook(context)
		return recorder
	}

	require.Equal(t, http.StatusBadRequest, call(signed.Header+"0").Code)
	var eventCount int64
	require.NoError(t, db.Model(&model.D2SPaymentEvent{}).Count(&eventCount).Error)
	assert.Zero(t, eventCount)
	var order model.D2SOrder
	require.NoError(t, db.First(&order, "id = ?", "stripe-webhook-order").Error)
	assert.Equal(t, model.D2SOrderPending, order.Status)

	require.Equal(t, http.StatusOK, call(signed.Header).Code)
	require.Equal(t, http.StatusOK, call(signed.Header).Code)
	require.NoError(t, db.Model(&model.D2SPaymentEvent{}).Count(&eventCount).Error)
	assert.EqualValues(t, 1, eventCount)
	require.NoError(t, db.First(&order, "id = ?", "stripe-webhook-order").Error)
	assert.Equal(t, model.D2SOrderPaid, order.Status)
}

func TestCreemWebhookD2SRejectsInvalidSignatureAndSettlesValidEvent(t *testing.T) {
	confirmPaymentComplianceForTest(t)
	previousAPIKey := setting.CreemApiKey
	previousProducts := setting.CreemProducts
	previousWebhookSecret := setting.CreemWebhookSecret
	t.Cleanup(func() {
		setting.CreemApiKey = previousAPIKey
		setting.CreemProducts = previousProducts
		setting.CreemWebhookSecret = previousWebhookSecret
	})
	setting.CreemApiKey = "creem_test_d2s_webhook"
	setting.CreemProducts = `[{"productId":"prod_d2s_webhook"}]`
	setting.CreemWebhookSecret = "creem_test_d2s_webhook_secret"

	db, err := gorm.Open(sqlite.Open("file:d2s_creem_webhook_signature_test?mode=memory&cache=shared"), &gorm.Config{})
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
		&model.D2SOrder{}, &model.D2SPaymentEvent{}, &model.D2SInviteReward{},
		&model.D2SBalanceAccount{}, &model.D2SBalanceTransaction{},
	))

	now := time.Now().Unix()
	require.NoError(t, db.Create(&model.User{
		Id: 1902, Username: "creem-webhook-user", Email: "creem-webhook@example.com",
		Password: "unused-hash", Role: common.RoleCommonUser, Status: common.UserStatusEnabled,
		Group: "default", AuthVersion: 1,
	}).Error)
	require.NoError(t, db.Create(&model.D2SUserProfile{
		UserID: 1902, EmailVerified: true, CreatedAt: now, UpdatedAt: now,
	}).Error)
	require.NoError(t, db.Create(&model.D2SOrder{
		ID: "creem-webhook-order", UserID: 1902, Product: model.D2SOrderProductLicense,
		Provider: "creem", Region: model.D2SRegionINTL, Currency: "USD",
		AmountMinor: 2990, GatewayMinor: 2990, Status: model.D2SOrderPending,
		CreatedAt: now, ExpiresAt: now + 3600,
	}).Error)

	payload := []byte(`{"id":"evt_creem_signed","eventType":"checkout.completed","object":{"request_id":"creem-webhook-order","order":{"amount_paid":2990,"currency":"USD","status":"paid"},"customer":{"id":"creem_customer_test","email":"Buyer@Example.com"}}}`)
	call := func(signature string) *httptest.ResponseRecorder {
		request := httptest.NewRequest(http.MethodPost, "/api/v1/creem/webhook", bytes.NewReader(payload))
		request.Header.Set(CreemSignatureHeader, signature)
		recorder := httptest.NewRecorder()
		context, _ := gin.CreateTestContext(recorder)
		context.Request = request
		CreemWebhook(context)
		return recorder
	}

	require.Equal(t, http.StatusUnauthorized, call("invalid-signature").Code)
	var eventCount int64
	require.NoError(t, db.Model(&model.D2SPaymentEvent{}).Count(&eventCount).Error)
	assert.Zero(t, eventCount)
	var order model.D2SOrder
	require.NoError(t, db.First(&order, "id = ?", "creem-webhook-order").Error)
	assert.Equal(t, model.D2SOrderPending, order.Status)

	signature := validD2SBridgeSignature(string(payload), setting.CreemWebhookSecret)
	require.Equal(t, http.StatusOK, call(signature).Code)
	require.Equal(t, http.StatusOK, call(signature).Code)
	require.NoError(t, db.Model(&model.D2SPaymentEvent{}).Count(&eventCount).Error)
	assert.EqualValues(t, 1, eventCount)
	require.NoError(t, db.First(&order, "id = ?", "creem-webhook-order").Error)
	assert.Equal(t, model.D2SOrderPaid, order.Status)
	var creemEvent CreemWebhookEvent
	require.NoError(t, json.Unmarshal(payload, &creemEvent))
	require.NotNil(t, order.PaymentAccountIdentifierHash)
	assert.Equal(t, creemD2SPaymentAccountIdentifierHash(&creemEvent), *order.PaymentAccountIdentifierHash)
	assert.NotContains(t, *order.PaymentAccountIdentifierHash, "creem_customer_test")
	assert.NotContains(t, *order.PaymentAccountIdentifierHash, "buyer@example.com")
}

func TestWaffoPancakeWebhookRejectsInvalidSignature(t *testing.T) {
	confirmPaymentComplianceForTest(t)
	previousMerchantID := setting.WaffoPancakeMerchantID
	previousPrivateKey := setting.WaffoPancakePrivateKey
	previousProductID := setting.WaffoPancakeProductID
	t.Cleanup(func() {
		setting.WaffoPancakeMerchantID = previousMerchantID
		setting.WaffoPancakePrivateKey = previousPrivateKey
		setting.WaffoPancakeProductID = previousProductID
	})
	setting.WaffoPancakeMerchantID = "merchant-test"
	setting.WaffoPancakePrivateKey = "private-key-test"
	setting.WaffoPancakeProductID = "product-test"

	payload := []byte(`{"eventType":"order.completed","mode":"test"}`)
	signature := "t=" + strconv.FormatInt(time.Now().UnixMilli(), 10) + ",v1=" + "0000000000000000000000000000000000000000000000000000000000000000"
	request := httptest.NewRequest(http.MethodPost, "/api/v1/waffo-pancake/webhook/test", bytes.NewReader(payload))
	request.Header.Set("X-Waffo-Signature", signature)
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = request
	context.Params = gin.Params{{Key: "env", Value: "test"}}

	WaffoPancakeWebhook(context)

	require.Equal(t, http.StatusUnauthorized, recorder.Code)
	assert.Equal(t, "invalid signature", recorder.Body.String())
}

func TestWaffoWebhookD2SRejectsInvalidSignatureAndSettlesValidEvent(t *testing.T) {
	confirmPaymentComplianceForTest(t)
	keyPair, err := waffoutils.GenerateKeyPair()
	require.NoError(t, err)
	previousWaffoEnabled, previousWaffoSandbox := setting.WaffoEnabled, setting.WaffoSandbox
	previousApiKey, previousPrivateKey, previousPublicCert := setting.WaffoSandboxApiKey, setting.WaffoSandboxPrivateKey, setting.WaffoSandboxPublicCert
	t.Cleanup(func() {
		setting.WaffoEnabled, setting.WaffoSandbox = previousWaffoEnabled, previousWaffoSandbox
		setting.WaffoSandboxApiKey, setting.WaffoSandboxPrivateKey, setting.WaffoSandboxPublicCert = previousApiKey, previousPrivateKey, previousPublicCert
	})
	setting.WaffoEnabled, setting.WaffoSandbox = true, true
	setting.WaffoSandboxApiKey, setting.WaffoSandboxPrivateKey, setting.WaffoSandboxPublicCert = "waffo-test-api-key", keyPair.PrivateKey, keyPair.PublicKey

	db, err := gorm.Open(sqlite.Open("file:d2s_waffo_webhook_signature_test?mode=memory&cache=shared"), &gorm.Config{})
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
		&model.D2SOrder{}, &model.D2SPaymentEvent{}, &model.D2SInviteReward{},
		&model.D2SBalanceAccount{}, &model.D2SBalanceTransaction{},
	))
	now := time.Now().Unix()
	require.NoError(t, db.Create(&model.User{
		Id: 1904, Username: "waffo-webhook-user", Email: "waffo-webhook@example.com",
		Password: "unused-hash", Role: common.RoleCommonUser, Status: common.UserStatusEnabled,
		Group: "default", AuthVersion: 1,
	}).Error)
	require.NoError(t, db.Create(&model.D2SUserProfile{
		UserID: 1904, EmailVerified: true, CreatedAt: now, UpdatedAt: now,
	}).Error)
	require.NoError(t, db.Create(&model.D2SOrder{
		ID: "waffo-webhook-order", UserID: 1904, Product: model.D2SOrderProductLicense,
		Provider: "waffo", Region: model.D2SRegionCN, Currency: "CNY",
		AmountMinor: 9900, GatewayMinor: 9900, Status: model.D2SOrderPending,
		CreatedAt: now, ExpiresAt: now + 3600,
	}).Error)

	payload := []byte(`{"eventType":"PAYMENT_NOTIFICATION","result":{"paymentRequestId":"waffo-payment","merchantOrderId":"waffo-webhook-order","orderStatus":"PAY_SUCCESS","orderCurrency":"CNY","orderAmount":"99.00"}}`)
	signature, err := waffoutils.Sign(string(payload), keyPair.PrivateKey)
	require.NoError(t, err)
	call := func(sig string) *httptest.ResponseRecorder {
		request := httptest.NewRequest(http.MethodPost, "/api/v1/waffo/webhook", bytes.NewReader(payload))
		request.Header.Set("X-SIGNATURE", sig)
		recorder := httptest.NewRecorder()
		context, _ := gin.CreateTestContext(recorder)
		context.Request = request
		WaffoWebhook(context)
		return recorder
	}

	require.Equal(t, http.StatusBadRequest, call("invalid-signature").Code)
	var eventCount int64
	require.NoError(t, db.Model(&model.D2SPaymentEvent{}).Count(&eventCount).Error)
	assert.Zero(t, eventCount)
	var order model.D2SOrder
	require.NoError(t, db.First(&order, "id = ?", "waffo-webhook-order").Error)
	assert.Equal(t, model.D2SOrderPending, order.Status)

	require.Equal(t, http.StatusOK, call(signature).Code)
	require.Equal(t, http.StatusOK, call(signature).Code)
	require.NoError(t, db.Model(&model.D2SPaymentEvent{}).Count(&eventCount).Error)
	assert.EqualValues(t, 1, eventCount)
	require.NoError(t, db.First(&order, "id = ?", "waffo-webhook-order").Error)
	assert.Equal(t, model.D2SOrderPaid, order.Status)
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
