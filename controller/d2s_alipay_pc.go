package controller

import (
	"bytes"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"io"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

const alipayPCGatewayURL = "https://openapi.alipay.com/gateway.do"

const alipayPCSandboxGatewayURL = "https://openapi-sandbox.dl.alipaydev.com/gateway.do"

const alipayPCOrderPrefix = "D2S"

type alipayPCConfig struct {
	appID      string
	sellerID   string
	privateKey *rsa.PrivateKey
	publicKey  *rsa.PublicKey
	gatewayURL string
	notifyURL  string
	returnURL  string
}

type alipayTradeQueryResult struct {
	Code        string `json:"code"`
	Msg         string `json:"msg"`
	AppID       string `json:"app_id"`
	SellerID    string `json:"seller_id"`
	OutTradeNo  string `json:"out_trade_no"`
	TradeNo     string `json:"trade_no"`
	TradeStatus string `json:"trade_status"`
	TotalAmount string `json:"total_amount"`
}

func callAlipayPCAPI(method string, bizContent map[string]string) (json.RawMessage, error) {
	config, err := getAlipayPCConfig()
	if err != nil {
		return nil, err
	}
	chinaLocation, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		chinaLocation = time.FixedZone("CST", 8*60*60)
	}
	biz, err := common.Marshal(bizContent)
	if err != nil {
		return nil, err
	}
	values := map[string]string{
		"app_id": config.appID, "method": method, "format": "JSON", "charset": "utf-8",
		"sign_type": "RSA2", "timestamp": time.Now().In(chinaLocation).Format("2006-01-02 15:04:05"),
		"version": "1.0", "biz_content": string(biz),
	}
	signature, err := signAlipayParameters(values, config.privateKey)
	if err != nil {
		return nil, err
	}
	form := url.Values{}
	for key, value := range values {
		form.Set(key, value)
	}
	form.Set("sign", signature)
	request, err := http.NewRequest(http.MethodPost, config.gatewayURL, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	client := &http.Client{Timeout: 15 * time.Second, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}
	response, err := client.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, errors.New("Alipay API returned a non-success HTTP status")
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, (1<<20)+1))
	if err != nil || len(body) > 1<<20 {
		return nil, errors.New("Alipay API response is invalid or too large")
	}
	var envelope map[string]json.RawMessage
	if err := common.Unmarshal(body, &envelope); err != nil {
		return nil, err
	}
	var responseSign string
	if err := common.Unmarshal(envelope["sign"], &responseSign); err != nil || responseSign == "" {
		return nil, errors.New("Alipay API response signature is missing")
	}
	responseKey := strings.ReplaceAll(method, ".", "_") + "_response"
	responseJSON := envelope[responseKey]
	if len(responseJSON) == 0 {
		return nil, errors.New("Alipay API response payload is missing")
	}
	var compact bytes.Buffer
	if err := json.Compact(&compact, responseJSON); err != nil {
		return nil, err
	}
	canonical := alipayCanonicalString(map[string]string{responseKey: compact.String()}, true)
	canonicalParams := map[string]string{responseKey: compact.String(), "sign_type": "RSA2", "sign": responseSign}
	if canonical != alipayCanonicalString(canonicalParams, false) {
		return nil, errors.New("Alipay API response signature canonicalization failed")
	}
	canonicalMap := map[string]string{responseKey: compact.String(), "sign": responseSign, "sign_type": "RSA2"}
	if !verifyAlipayParameters(canonicalMap, config.publicKey) {
		return nil, errors.New("Alipay API response signature verification failed")
	}
	return responseJSON, nil
}

func queryAlipayPCTrade(order *model.D2SOrder) (*alipayTradeQueryResult, error) {
	if order == nil || order.Provider != "alipay_pc" {
		return nil, model.ErrD2SOrderInvalid
	}
	outTradeNo, err := alipayOutTradeNo(order.ID)
	if err != nil {
		return nil, err
	}
	raw, err := callAlipayPCAPI("alipay.trade.query", map[string]string{"out_trade_no": outTradeNo})
	if err != nil {
		return nil, err
	}
	var result alipayTradeQueryResult
	if err := common.Unmarshal(raw, &result); err != nil || result.Code != "10000" || result.AppID != strings.TrimSpace(os.Getenv("D2S_ALIPAY_APP_ID")) || result.SellerID != strings.TrimSpace(os.Getenv("D2S_ALIPAY_SELLER_ID")) || result.OutTradeNo != outTradeNo {
		return nil, model.ErrD2SPaymentMismatch
	}
	amount, err := alipayAmountMinor(result.TotalAmount)
	if err != nil || amount != order.GatewayMinor || strings.TrimSpace(result.TradeNo) == "" {
		return nil, model.ErrD2SPaymentMismatch
	}
	return &result, nil
}

func closeAlipayPCTrade(order *model.D2SOrder) error {
	if order == nil || order.Provider != "alipay_pc" {
		return model.ErrD2SOrderInvalid
	}
	outTradeNo, err := alipayOutTradeNo(order.ID)
	if err != nil {
		return err
	}
	raw, err := callAlipayPCAPI("alipay.trade.close", map[string]string{"out_trade_no": outTradeNo})
	if err != nil {
		return err
	}
	var result struct {
		Code        string `json:"code"`
		AppID       string `json:"app_id"`
		OutTradeNo  string `json:"out_trade_no"`
		TradeStatus string `json:"trade_status"`
	}
	config, err := getAlipayPCConfig()
	if err != nil {
		return err
	}
	if err := common.Unmarshal(raw, &result); err != nil || result.Code != "10000" || result.AppID != config.appID || result.OutTradeNo != outTradeNo || result.TradeStatus != "TRADE_CLOSED" {
		return model.ErrD2SPaymentMismatch
	}
	return nil
}

func isAlipayPCEnabled() bool {
	if !isPaymentComplianceConfirmed() {
		return false
	}
	settings, err := model.GetD2SAlipayPCSettings()
	if err != nil {
		return false
	}
	enabled := strings.TrimSpace(os.Getenv("D2S_ALIPAY_PC_ENABLED")) == "true"
	if settings.ConfigManaged {
		enabled = settings.ChannelEnabled
	}
	if !enabled {
		return false
	}
	_, err = getAlipayPCConfig()
	return err == nil
}

func getAlipayPCConfig() (*alipayPCConfig, error) {
	settings, err := model.GetD2SAlipayPCSettings()
	if err != nil {
		return nil, err
	}
	appID := strings.TrimSpace(os.Getenv("D2S_ALIPAY_APP_ID"))
	sellerID := strings.TrimSpace(os.Getenv("D2S_ALIPAY_SELLER_ID"))
	privateKeyRaw := os.Getenv("D2S_ALIPAY_PRIVATE_KEY_PEM")
	publicKeyRaw := os.Getenv("D2S_ALIPAY_PUBLIC_KEY_PEM")
	notifyURL := strings.TrimSpace(os.Getenv("D2S_ALIPAY_NOTIFY_URL"))
	returnURL := strings.TrimSpace(os.Getenv("D2S_ALIPAY_RETURN_URL"))
	gatewayURL := strings.TrimSpace(os.Getenv("D2S_ALIPAY_GATEWAY_URL"))
	if settings.ConfigManaged {
		appID, sellerID = settings.AppID, settings.SellerID
		gatewayURL, notifyURL, returnURL = settings.GatewayURL, settings.NotifyURL, settings.ReturnURL
		privateKeyRaw, err = decryptD2SAlipaySetting(settings.PrivateKeyCiphertext)
		if err != nil {
			return nil, err
		}
		publicKeyRaw, err = decryptD2SAlipaySetting(settings.PublicKeyCiphertext)
		if err != nil {
			return nil, err
		}
	}
	privateKey, err := parseAlipayPrivateKey(privateKeyRaw)
	if err != nil {
		return nil, err
	}
	publicKey, err := parseAlipayPublicKey(publicKeyRaw)
	if err != nil {
		return nil, err
	}
	appID = strings.TrimSpace(appID)
	sellerID = strings.TrimSpace(sellerID)
	notifyURL = strings.TrimSpace(notifyURL)
	returnURL = strings.TrimSpace(returnURL)
	gatewayURL = strings.TrimSpace(gatewayURL)
	if gatewayURL == "" {
		gatewayURL = alipayPCGatewayURL
	}
	if appID == "" || sellerID == "" || !validAlipayHTTPSURL(notifyURL) || !validAlipayHTTPSURL(returnURL) || !validAlipayGatewayURL(gatewayURL) {
		return nil, errors.New("Alipay PC configuration is incomplete")
	}
	return &alipayPCConfig{appID: appID, sellerID: sellerID, privateKey: privateKey, publicKey: publicKey, gatewayURL: gatewayURL, notifyURL: notifyURL, returnURL: returnURL}, nil
}

func validAlipayHTTPSURL(raw string) bool {
	parsed, err := url.Parse(raw)
	return err == nil && strings.EqualFold(parsed.Scheme, "https") && parsed.Host != "" && parsed.User == nil && parsed.Fragment == ""
}

func validAlipayGatewayURL(raw string) bool {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme != "https" || parsed.Path != "/gateway.do" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return false
	}
	switch strings.ToLower(parsed.Hostname()) {
	case "openapi.alipay.com", "openapi.alipaydev.com", "openapi-sandbox.dl.alipaydev.com":
		return parsed.Port() == ""
	default:
		return false
	}
}

func alipayKeyBytes(raw string) []byte {
	value := strings.TrimSpace(strings.ReplaceAll(raw, `\n`, "\n"))
	if block, _ := pem.Decode([]byte(value)); block != nil {
		return block.Bytes
	}
	decoded, err := base64.StdEncoding.DecodeString(value)
	if err != nil {
		decoded, err = base64.RawStdEncoding.DecodeString(value)
	}
	if err != nil {
		return nil
	}
	if block, _ := pem.Decode(decoded); block != nil {
		return block.Bytes
	}
	return decoded
}

func parseAlipayPrivateKey(raw string) (*rsa.PrivateKey, error) {
	keyBytes := alipayKeyBytes(raw)
	if len(keyBytes) == 0 {
		return nil, errors.New("Alipay RSA2 private key is missing or invalid")
	}
	if key, err := x509.ParsePKCS1PrivateKey(keyBytes); err == nil && key.N.BitLen() >= 2048 {
		return key, nil
	}
	parsed, err := x509.ParsePKCS8PrivateKey(keyBytes)
	if err != nil {
		return nil, errors.New("Alipay RSA2 private key must be PKCS#1 or PKCS#8 PEM")
	}
	key, ok := parsed.(*rsa.PrivateKey)
	if !ok || key.N.BitLen() < 2048 {
		return nil, errors.New("Alipay RSA2 private key must be RSA with at least 2048 bits")
	}
	return key, nil
}

func parseAlipayPublicKey(raw string) (*rsa.PublicKey, error) {
	keyBytes := alipayKeyBytes(raw)
	if len(keyBytes) == 0 {
		return nil, errors.New("Alipay RSA2 public key is missing or invalid")
	}
	if key, err := x509.ParsePKCS1PublicKey(keyBytes); err == nil && key.N.BitLen() >= 2048 {
		return key, nil
	}
	parsed, err := x509.ParsePKIXPublicKey(keyBytes)
	if err != nil {
		return nil, errors.New("Alipay RSA2 public key must be PKCS#1 or PKIX PEM")
	}
	key, ok := parsed.(*rsa.PublicKey)
	if !ok || key.N.BitLen() < 2048 {
		return nil, errors.New("Alipay RSA2 public key must be RSA with at least 2048 bits")
	}
	return key, nil
}

func alipayCanonicalString(values map[string]string, includeSignType bool) string {
	keys := make([]string, 0, len(values))
	for key, value := range values {
		if key != "sign" && (includeSignType || key != "sign_type") && strings.TrimSpace(value) != "" {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, key+"="+values[key])
	}
	return strings.Join(parts, "&")
}

func signAlipayParameters(values map[string]string, privateKey *rsa.PrivateKey) (string, error) {
	if privateKey == nil {
		return "", errors.New("Alipay private key is unavailable")
	}
	digest := sha256.Sum256([]byte(alipayCanonicalString(values, true)))
	signature, err := rsa.SignPKCS1v15(rand.Reader, privateKey, crypto.SHA256, digest[:])
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(signature), nil
}

func verifyAlipayParameters(values map[string]string, publicKey *rsa.PublicKey) bool {
	if publicKey == nil || values["sign_type"] != "RSA2" || strings.TrimSpace(values["sign"]) == "" {
		return false
	}
	signature, err := base64.StdEncoding.DecodeString(values["sign"])
	if err != nil {
		return false
	}
	digest := sha256.Sum256([]byte(alipayCanonicalString(values, false)))
	return rsa.VerifyPKCS1v15(publicKey, crypto.SHA256, digest[:], signature) == nil
}

func alipayOutTradeNo(orderID string) (string, error) {
	parsed, err := uuid.Parse(orderID)
	if err != nil || parsed.String() != orderID {
		return "", model.ErrD2SCheckoutUnavailable
	}
	return alipayPCOrderPrefix + strings.ReplaceAll(orderID, "-", ""), nil
}

func d2SOrderIDFromAlipayOutTradeNo(outTradeNo string) (string, error) {
	if len(outTradeNo) != len(alipayPCOrderPrefix)+32 || !strings.HasPrefix(outTradeNo, alipayPCOrderPrefix) {
		return "", model.ErrD2SPaymentMismatch
	}
	parsed, err := uuid.Parse(outTradeNo[len(alipayPCOrderPrefix):])
	if err != nil || alipayPCOrderPrefix+strings.ReplaceAll(parsed.String(), "-", "") != outTradeNo {
		return "", model.ErrD2SPaymentMismatch
	}
	return parsed.String(), nil
}

func validAlipayQRPayMode(mode string) bool {
	switch mode {
	case "0", "1", "3", "4":
		return true
	default:
		return false
	}
}

func createD2SAlipayPCCheckout(order *model.D2SOrder) (*d2SEpayCheckout, error) {
	if order == nil || order.Provider != "alipay_pc" || order.Region != model.D2SRegionCN || order.Currency != "CNY" || order.GatewayMinor <= 0 || order.GatewayMinor > 10_000_000_000 || order.ExpiresAt-time.Now().Unix() < 60 || !isAlipayPCEnabled() {
		return nil, model.ErrD2SCheckoutUnavailable
	}
	settings, err := model.GetD2SAlipayPCSettings()
	if err != nil {
		return nil, model.ErrD2SCheckoutUnavailable
	}
	qrPayMode := settings.QRPayMode
	if !validAlipayQRPayMode(qrPayMode) {
		qrPayMode = "4"
	}
	config, err := getAlipayPCConfig()
	if err != nil {
		return nil, model.ErrD2SCheckoutUnavailable
	}
	outTradeNo, err := alipayOutTradeNo(order.ID)
	if err != nil {
		return nil, err
	}
	chinaLocation, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		chinaLocation = time.FixedZone("CST", 8*60*60)
	}
	bizValues := map[string]string{
		"out_trade_no": outTradeNo,
		"product_code": "FAST_INSTANT_TRADE_PAY",
		"qr_pay_mode":  qrPayMode,
		"subject":      "Desktop2Stereo license",
		"total_amount": decimal.NewFromInt(order.GatewayMinor).Div(decimal.NewFromInt(100)).StringFixed(2),
		"time_expire":  time.Unix(order.ExpiresAt, 0).In(chinaLocation).Format("2006-01-02 15:04:05"),
	}
	if qrPayMode == "4" {
		bizValues["qrcode_width"] = "240"
	}
	bizContent, err := common.Marshal(bizValues)
	if err != nil {
		return nil, model.ErrD2SCheckoutUnavailable
	}
	values := map[string]string{
		"app_id": config.appID, "method": "alipay.trade.page.pay", "format": "JSON",
		"charset": "utf-8", "sign_type": "RSA2", "timestamp": time.Now().In(chinaLocation).Format("2006-01-02 15:04:05"),
		"version": "1.0", "notify_url": config.notifyURL, "return_url": config.returnURL,
		"biz_content": string(bizContent),
	}
	signature, err := signAlipayParameters(values, config.privateKey)
	if err != nil {
		return nil, model.ErrD2SCheckoutUnavailable
	}
	values["sign"] = signature
	return &d2SEpayCheckout{URL: config.gatewayURL, Params: values, QRPayMode: qrPayMode}, nil
}

func parseAlipayForm(values url.Values) (map[string]string, error) {
	parsed := make(map[string]string, len(values))
	for key, entries := range values {
		if len(entries) != 1 {
			return nil, errors.New("duplicate Alipay notification field")
		}
		parsed[key] = entries[0]
	}
	return parsed, nil
}

func alipayAmountMinor(raw string) (int64, error) {
	amount, err := decimal.NewFromString(strings.TrimSpace(raw))
	if err != nil || !amount.IsPositive() {
		return 0, model.ErrD2SPaymentMismatch
	}
	minor := amount.Mul(decimal.NewFromInt(100))
	if !minor.IsInteger() || !minor.BigInt().IsInt64() {
		return 0, model.ErrD2SPaymentMismatch
	}
	return minor.IntPart(), nil
}

func AlipayPCD2SNotify(c *gin.Context) {
	body, err := io.ReadAll(io.LimitReader(c.Request.Body, (1<<20)+1))
	if err != nil || len(body) > 1<<20 {
		c.String(http.StatusBadRequest, "failure")
		return
	}
	// Existing payments must still settle after new checkouts are disabled.
	config, err := getAlipayPCConfig()
	if err != nil {
		c.String(http.StatusServiceUnavailable, "failure")
		return
	}
	values, err := url.ParseQuery(string(body))
	if err != nil {
		c.String(http.StatusBadRequest, "failure")
		return
	}
	notification, err := parseAlipayForm(values)
	if err != nil || !verifyAlipayParameters(notification, config.publicKey) ||
		notification["app_id"] != config.appID || notification["auth_app_id"] != config.appID ||
		notification["seller_id"] != config.sellerID || notification["notify_type"] != "trade_status_sync" {
		c.String(http.StatusBadRequest, "failure")
		return
	}
	orderID, err := d2SOrderIDFromAlipayOutTradeNo(notification["out_trade_no"])
	eventID := strings.TrimSpace(notification["notify_id"])
	if err != nil || eventID == "" {
		c.String(http.StatusBadRequest, "failure")
		return
	}
	switch notification["trade_status"] {
	case "TRADE_SUCCESS", "TRADE_FINISHED":
	default:
		c.String(http.StatusOK, "success")
		return
	}
	amountMinor, err := alipayAmountMinor(notification["total_amount"])
	if err != nil || strings.TrimSpace(notification["trade_no"]) == "" {
		c.String(http.StatusBadRequest, "failure")
		return
	}
	handled, err := processVerifiedD2SPayment("alipay_pc", eventID, orderID, "paid", amountMinor, "CNY", body)
	if err != nil || !handled {
		c.String(http.StatusBadRequest, "failure")
		return
	}
	c.String(http.StatusOK, "success")
}

func AlipayPCApplicationGateway(c *gin.Context) {
	if c.Request.Method == http.MethodGet {
		c.String(http.StatusOK, "success")
		return
	}
	if c.Request.Method != http.MethodPost {
		c.String(http.StatusMethodNotAllowed, "failure")
		return
	}
	query, err := url.ParseQuery(c.Request.URL.RawQuery)
	if err != nil || len(query) > 0 {
		c.String(http.StatusBadRequest, "failure")
		return
	}
	body, err := io.ReadAll(io.LimitReader(c.Request.Body, (1<<20)+1))
	if err != nil || len(body) > 1<<20 {
		c.String(http.StatusBadRequest, "failure")
		return
	}
	form, err := url.ParseQuery(string(body))
	if err != nil {
		c.String(http.StatusBadRequest, "failure")
		return
	}
	values, err := parseAlipayForm(form)
	if err != nil {
		c.String(http.StatusBadRequest, "failure")
		return
	}
	if values["notify_type"] != "trade_status_sync" {
		notifyType := strings.Map(func(r rune) rune {
			if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '_' || r == '.' {
				return r
			}
			return -1
		}, values["notify_type"])
		if len(notifyType) > 64 {
			notifyType = notifyType[:64]
		}
		common.SysLog("rejected unsupported Alipay application gateway notification type: " + notifyType)
		c.String(http.StatusBadRequest, "failure")
		return
	}
	c.Request.Body = io.NopCloser(strings.NewReader(string(body)))
	AlipayPCD2SNotify(c)
}

func StartD2SAlipayReconciliation() {
	if !common.IsMasterNode {
		return
	}
	go func() {
		runD2SAlipayReconciliation()
		ticker := time.NewTicker(time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			runD2SAlipayReconciliation()
		}
	}()
}

func runD2SAlipayReconciliation() {
	if _, err := getAlipayPCConfig(); err != nil {
		return
	}
	var orders []model.D2SOrder
	if err := model.DB.Where("provider = ? AND status = ?", "alipay_pc", model.D2SOrderPending).
		Order("created_at ASC").Limit(100).Find(&orders).Error; err != nil {
		common.SysError("failed to list pending Alipay PC orders: " + err.Error())
		return
	}
	for i := range orders {
		reconcileAlipayPCOrder(&orders[i], time.Now().UTC().Unix())
	}
}

func reconcileAlipayPCOrder(order *model.D2SOrder, now int64) {
	result, err := queryAlipayPCTrade(order)
	if err != nil {
		common.SysError("failed to query Alipay PC order " + order.ID + ": " + err.Error())
		return
	}
	var payload []byte
	encoded, err := common.Marshal(result)
	if err != nil {
		return
	}
	payload = encoded
	payloadDigest := sha256.Sum256(payload)
	payloadHash := hex.EncodeToString(payloadDigest[:])
	switch result.TradeStatus {
	case "TRADE_SUCCESS", "TRADE_FINISHED":
		amountMinor, amountErr := alipayAmountMinor(result.TotalAmount)
		if amountErr != nil {
			return
		}
		if _, err := model.ProcessD2SPaymentEvent("alipay_pc", "query:"+result.TradeNo, order.ID, "paid", amountMinor, "CNY", payloadHash, now); err != nil {
			common.SysError("failed to settle queried Alipay PC order " + order.ID + ": " + err.Error())
		}
	case "TRADE_CLOSED":
		if _, err := model.ProcessD2SPaymentEvent("alipay_pc", "query-closed:"+order.ID, order.ID, "canceled", order.GatewayMinor, "CNY", payloadHash, now); err != nil {
			common.SysError("failed to cancel closed Alipay PC order " + order.ID + ": " + err.Error())
		}
	case "WAIT_BUYER_PAY":
		if order.ExpiresAt > now {
			return
		}
		if err := closeAlipayPCTrade(order); err != nil {
			common.SysError("failed to close expired Alipay PC order " + order.ID + ": " + err.Error())
			return
		}
		if _, err := model.ProcessD2SPaymentEvent("alipay_pc", "close:"+order.ID, order.ID, "canceled", order.GatewayMinor, "CNY", payloadHash, now); err != nil {
			common.SysError("failed to apply Alipay PC close for order " + order.ID + ": " + err.Error())
		}
	default:
		common.SysLog("Alipay PC order remains pending with trade status " + strconv.Quote(result.TradeStatus))
	}
}
