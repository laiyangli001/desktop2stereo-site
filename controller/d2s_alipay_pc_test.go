package controller

import (
	"bytes"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"net/url"
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

func setAlipayPCTestConfig(t *testing.T) (*rsa.PrivateKey, *rsa.PrivateKey) {
	t.Helper()
	confirmPaymentComplianceForTest(t)
	merchantKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	alipayKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	privatePEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: mustPKCS8PrivateKey(t, merchantKey)})
	publicPEM := pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: mustPKIXPublicKey(t, &alipayKey.PublicKey)})
	t.Setenv("D2S_ALIPAY_PC_ENABLED", "true")
	t.Setenv("D2S_ALIPAY_APP_ID", "2026000000000000")
	t.Setenv("D2S_ALIPAY_SELLER_ID", "2088000000000000")
	t.Setenv("D2S_ALIPAY_PRIVATE_KEY_PEM", string(privatePEM))
	t.Setenv("D2S_ALIPAY_PUBLIC_KEY_PEM", string(publicPEM))
	t.Setenv("D2S_ALIPAY_NOTIFY_URL", "https://d2s.site/api/v1/alipay/d2s/notify")
	t.Setenv("D2S_ALIPAY_RETURN_URL", "https://d2s.site/d2s")
	return merchantKey, alipayKey
}

func mustPKCS8PrivateKey(t *testing.T, key *rsa.PrivateKey) []byte {
	t.Helper()
	encoded, err := x509.MarshalPKCS8PrivateKey(key)
	require.NoError(t, err)
	return encoded
}

func mustPKIXPublicKey(t *testing.T, key *rsa.PublicKey) []byte {
	t.Helper()
	encoded, err := x509.MarshalPKIXPublicKey(key)
	require.NoError(t, err)
	return encoded
}

func TestD2SAlipayPCCheckoutBuildsSignedPagePayURL(t *testing.T) {
	previousDB := model.DB
	db, err := gorm.Open(sqlite.Open("file:"+strings.ReplaceAll(t.Name(), "/", "_")+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.D2SAlipayPCSettings{}))
	model.DB = db
	t.Cleanup(func() {
		model.DB = previousDB
		sqlDB, dbErr := db.DB()
		if dbErr == nil {
			_ = sqlDB.Close()
		}
	})
	merchantKey, _ := setAlipayPCTestConfig(t)
	orderID := "a14710dd-3c1f-43e5-84d4-d345d9d88e85"
	checkout, err := createD2SAlipayPCCheckout(&model.D2SOrder{
		ID: orderID, Provider: "alipay_pc", Region: model.D2SRegionCN,
		Currency: "CNY", GatewayMinor: 1990, ExpiresAt: time.Now().Unix() + 1800,
	})
	require.NoError(t, err)
	require.Equal(t, alipayPCGatewayURL, checkout.URL)
	require.NotEmpty(t, checkout.Params["sign"])
	values := checkout.Params
	require.Equal(t, "alipay.trade.page.pay", values["method"])
	require.Equal(t, "2026000000000000", values["app_id"])
	signature, err := base64.StdEncoding.DecodeString(values["sign"])
	require.NoError(t, err)
	digest := sha256.Sum256([]byte(alipayCanonicalString(values, true)))
	require.NoError(t, rsa.VerifyPKCS1v15(&merchantKey.PublicKey, crypto.SHA256, digest[:], signature))
	require.Contains(t, alipayCanonicalString(values, true), "sign_type=RSA2")
	var bizContent map[string]string
	require.NoError(t, common.Unmarshal([]byte(values["biz_content"]), &bizContent))
	require.Equal(t, "D2Sa14710dd3c1f43e584d4d345d9d88e85", bizContent["out_trade_no"])
	require.Equal(t, "19.90", bizContent["total_amount"])
	require.Equal(t, "FAST_INSTANT_TRADE_PAY", bizContent["product_code"])
	require.Equal(t, "4", bizContent["qr_pay_mode"])
	require.Equal(t, "240", bizContent["qrcode_width"])
	require.NotEmpty(t, bizContent["time_expire"])
	reversedOrderID, err := d2SOrderIDFromAlipayOutTradeNo(bizContent["out_trade_no"])
	require.NoError(t, err)
	require.Equal(t, orderID, reversedOrderID)
	values["app_id"] = "tampered"
	digest = sha256.Sum256([]byte(alipayCanonicalString(values, true)))
	require.Error(t, rsa.VerifyPKCS1v15(&merchantKey.PublicKey, crypto.SHA256, digest[:], signature))
}

func TestAlipayApplicationGatewaySafeDetection(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/api/v1/alipay/app-gateway", AlipayPCApplicationGateway)
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/alipay/app-gateway", nil)
	router.ServeHTTP(response, request)
	assert.Equal(t, http.StatusOK, response.Code)
	assert.Equal(t, "success", response.Body.String())
}

func TestAlipayQRPayModeValidation(t *testing.T) {
	for _, mode := range []string{"0", "1", "3", "4"} {
		assert.True(t, validAlipayQRPayMode(mode))
	}
	for _, mode := range []string{"", "2", "5", "04", "4;drop"} {
		assert.False(t, validAlipayQRPayMode(mode))
	}
}

func signAlipayPCTestNotification(t *testing.T, values map[string]string, privateKey *rsa.PrivateKey) {
	t.Helper()
	digest := sha256.Sum256([]byte(alipayCanonicalString(values, false)))
	signature, err := rsa.SignPKCS1v15(rand.Reader, privateKey, crypto.SHA256, digest[:])
	require.NoError(t, err)
	values["sign"] = base64.StdEncoding.EncodeToString(signature)
}

func TestD2SAlipayPCAmountAndFormValidation(t *testing.T) {
	for _, test := range []struct {
		amount string
		minor  int64
		valid  bool
	}{
		{amount: "19.90", minor: 1990, valid: true},
		{amount: "0.01", minor: 1, valid: true},
		{amount: "0", valid: false},
		{amount: "1.001", valid: false},
		{amount: "-1.00", valid: false},
	} {
		minor, err := alipayAmountMinor(test.amount)
		if test.valid {
			require.NoError(t, err)
			require.Equal(t, test.minor, minor)
		} else {
			require.Error(t, err)
		}
	}
	_, err := parseAlipayForm(url.Values{"app_id": {"one", "two"}})
	require.Error(t, err)
	_, err = d2SOrderIDFromAlipayOutTradeNo("D2Sbad-order-id")
	require.Error(t, err)
}

func TestD2SAlipayPCCheckoutRejectsNearlyExpiredOrder(t *testing.T) {
	setAlipayPCTestConfig(t)
	_, err := createD2SAlipayPCCheckout(&model.D2SOrder{
		ID: "a14710dd-3c1f-43e5-84d4-d345d9d88e85", Provider: "alipay_pc",
		Region: model.D2SRegionCN, Currency: "CNY", GatewayMinor: 1990,
		ExpiresAt: time.Now().Unix() + 30,
	})
	require.ErrorIs(t, err, model.ErrD2SCheckoutUnavailable)
}

func TestD2SAlipayPCRejectsUnsafeCallbackURLs(t *testing.T) {
	for _, raw := range []string{"http://d2s.site/notify", "https:///notify", "https://user:pass@d2s.site/notify", "https://d2s.site/notify#fragment"} {
		require.False(t, validAlipayHTTPSURL(raw), raw)
	}
	require.True(t, validAlipayHTTPSURL("https://d2s.site/api/v1/alipay/d2s/notify"))
	require.True(t, validAlipayGatewayURL(alipayPCGatewayURL))
	require.True(t, validAlipayGatewayURL(alipayPCSandboxGatewayURL))
	require.True(t, validAlipayGatewayURL("https://openapi.alipaydev.com/gateway.do"))
	require.False(t, validAlipayGatewayURL("https://mapi.alipaydev.com/gateway.do"))
	require.False(t, validAlipayGatewayURL("https://pay.example.test/gateway.do"))
}

func TestAlipayPCD2SNotifyVerifiesAndSettlesOrder(t *testing.T) {
	merchantKey, alipayKey := setAlipayPCTestConfig(t)
	orderID := "d07d267a-83a7-430e-a0ca-ea5643203ab1"
	outTradeNo, err := alipayOutTradeNo(orderID)
	require.NoError(t, err)
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open("file:d2s_alipay_pc_notify_test?mode=memory&cache=shared"), &gorm.Config{})
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
		Id: 2901, Username: "alipay-pc-user", Email: "alipay-pc@example.com", Password: "unused-hash",
		Role: common.RoleCommonUser, Status: common.UserStatusEnabled, Group: "default", AuthVersion: 1,
	}).Error)
	require.NoError(t, db.Create(&model.D2SUserProfile{UserID: 2901, EmailVerified: true, CreatedAt: now, UpdatedAt: now}).Error)
	require.NoError(t, db.Create(&model.D2SOrder{
		ID: orderID, UserID: 2901, Product: model.D2SOrderProductLicense,
		Provider: "alipay_pc", Region: model.D2SRegionCN, Currency: "CNY", AmountMinor: 1990,
		GatewayMinor: 1990, Status: model.D2SOrderPending, CreatedAt: now, ExpiresAt: now + 3600,
	}).Error)

	values := map[string]string{
		"app_id": "2026000000000000", "auth_app_id": "2026000000000000", "seller_id": "2088000000000000",
		"notify_type": "trade_status_sync", "sign_type": "RSA2", "notify_id": "alipay-notify-event-1",
		"out_trade_no": outTradeNo, "trade_no": "alipay-trade-1",
		"total_amount": "19.90", "trade_status": "TRADE_SUCCESS",
	}
	signAlipayPCTestNotification(t, values, alipayKey)
	call := func(payload map[string]string) *httptest.ResponseRecorder {
		form := url.Values{}
		for key, value := range payload {
			form.Set(key, value)
		}
		request := httptest.NewRequest(http.MethodPost, "/api/v1/alipay/d2s/notify", bytes.NewBufferString(form.Encode()))
		request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		recorder := httptest.NewRecorder()
		context, _ := gin.CreateTestContext(recorder)
		context.Request = request
		AlipayPCD2SNotify(context)
		return recorder
	}
	invalid := make(map[string]string, len(values))
	for key, value := range values {
		invalid[key] = value
	}
	invalid["total_amount"] = "0.01"
	signAlipayPCTestNotification(t, invalid, alipayKey)
	assert.Equal(t, http.StatusBadRequest, call(invalid).Code)
	invalid["total_amount"] = "19.90"
	invalid["app_id"] = "another-app"
	signAlipayPCTestNotification(t, invalid, alipayKey)
	assert.Equal(t, http.StatusBadRequest, call(invalid).Code)
	invalid["app_id"] = values["app_id"]
	invalid["seller_id"] = "2088999999999999"
	signAlipayPCTestNotification(t, invalid, alipayKey)
	assert.Equal(t, http.StatusBadRequest, call(invalid).Code)
	invalid["seller_id"] = values["seller_id"]
	invalid["auth_app_id"] = "another-app"
	signAlipayPCTestNotification(t, invalid, alipayKey)
	assert.Equal(t, http.StatusBadRequest, call(invalid).Code)
	invalid["auth_app_id"] = values["auth_app_id"]
	invalid["out_trade_no"] = "D2Sbad-order-id"
	signAlipayPCTestNotification(t, invalid, alipayKey)
	assert.Equal(t, http.StatusBadRequest, call(invalid).Code)
	badSignature := make(map[string]string, len(values))
	for key, value := range values {
		badSignature[key] = value
	}
	badSignature["total_amount"] = "0.01"
	assert.Equal(t, http.StatusBadRequest, call(badSignature).Code)
	signAlipayPCTestNotification(t, badSignature, merchantKey)
	assert.Equal(t, http.StatusBadRequest, call(badSignature).Code)
	t.Setenv("D2S_ALIPAY_PC_ENABLED", "false")
	require.False(t, isAlipayPCEnabled())
	assert.Equal(t, http.StatusOK, call(values).Code)
	assert.Equal(t, http.StatusOK, call(values).Code)
	var order model.D2SOrder
	require.NoError(t, db.First(&order, "id = ?", orderID).Error)
	assert.Equal(t, model.D2SOrderPaid, order.Status)
	var eventCount int64
	require.NoError(t, db.Model(&model.D2SPaymentEvent{}).Where("order_id = ?", order.ID).Count(&eventCount).Error)
	assert.EqualValues(t, 1, eventCount)
}

func TestAlipayPCD2SNotifyRejectsOversizedBody(t *testing.T) {
	setAlipayPCTestConfig(t)
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest(http.MethodPost, "/api/v1/alipay/d2s/notify", bytes.NewBufferString("x="+strings.Repeat("a", 1<<20)))
	AlipayPCD2SNotify(context)
	require.Equal(t, http.StatusBadRequest, recorder.Code)
}
