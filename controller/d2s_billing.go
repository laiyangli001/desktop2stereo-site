package controller

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type d2sOrderRequest struct {
	Product        string `json:"product"`
	LicenseID      string `json:"license_id"`
	Provider       string `json:"provider"`
	BalanceMinor   int64  `json:"balance_minor"`
	IdempotencyKey string `json:"idempotency_key"`
}

func D2SOrderPreview(c *gin.Context) {
	var request d2sOrderRequest
	if err := common.DecodeJson(c.Request.Body, &request); err != nil {
		d2sInvalidInput(c, "invalid JSON body")
		return
	}
	quote, err := model.QuoteD2SOrder(c.GetInt("id"), request.Product, request.LicenseID, request.Provider, time.Now().Unix())
	if err != nil {
		d2sError(c, err)
		return
	}
	d2sSuccess(c, http.StatusOK, quote)
}

func D2SOrderCreate(c *gin.Context) {
	var request d2sOrderRequest
	if err := common.DecodeJson(c.Request.Body, &request); err != nil || strings.TrimSpace(request.IdempotencyKey) == "" {
		d2sInvalidInput(c, "product, provider and idempotency_key are required")
		return
	}
	now := time.Now().Unix()
	quote, err := model.QuoteD2SOrder(c.GetInt("id"), request.Product, request.LicenseID, request.Provider, now)
	if err != nil {
		d2sError(c, err)
		return
	}
	order, err := model.CreateD2SOrder(c.GetInt("id"), quote, request.BalanceMinor, request.IdempotencyKey, now)
	if err != nil {
		d2sError(c, err)
		return
	}
	d2sSuccess(c, http.StatusCreated, order)
}

func D2SOrderGet(c *gin.Context) {
	var order model.D2SOrder
	if err := model.DB.Where("id = ? AND user_id = ?", c.Param("id"), c.GetInt("id")).First(&order).Error; err != nil {
		d2sError(c, model.ErrD2SOrderNotFound)
		return
	}
	d2sSuccess(c, http.StatusOK, order)
}

func D2SOrderList(c *gin.Context) {
	var orders []model.D2SOrder
	if err := model.DB.Where("user_id = ?", c.GetInt("id")).Order("created_at DESC").Limit(100).Find(&orders).Error; err != nil {
		d2sError(c, err)
		return
	}
	d2sSuccess(c, http.StatusOK, gin.H{"orders": orders})
}

func D2SOrderProviders(c *gin.Context) {
	var profile model.D2SUserProfile
	if err := model.DB.Where("user_id = ?", c.GetInt("id")).First(&profile).Error; err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		d2sError(c, err)
		return
	}
	providers := make([]string, 0, 7)
	appendProvider := func(provider string) {
		if provider == model.D2SProviderBalance {
			if profile.Region != "" {
				providers = append(providers, provider)
			}
			return
		}
		region, ok := model.D2SProviderRegion(provider)
		if ok && (profile.Region == "" || profile.Region == region) {
			providers = append(providers, provider)
		}
	}
	appendProvider(model.D2SProviderBalance)
	if isStripeTopUpEnabled() {
		appendProvider("stripe")
	}
	if isCreemTopUpEnabled() {
		appendProvider("creem")
	}
	if isWaffoPancakeTopUpEnabled() {
		appendProvider("waffo_pancake")
	}
	if isWaffoTopUpEnabled() {
		appendProvider("waffo")
	}
	if isEpayTopUpEnabled() {
		for _, method := range operation_setting.PayMethods {
			switch method["type"] {
			case "paymentfm":
				appendProvider("paymentfm")
			case "alipay":
				appendProvider("alipay")
			case "wxpay":
				appendProvider("wechat")
			}
		}
	}
	d2sSuccess(c, http.StatusOK, gin.H{"providers": providers})
}

func D2SLicensePaidRevoke(c *gin.Context) { D2SOrderCreate(c) }

func D2SLicenseOfflineExtend(c *gin.Context) { D2SOrderCreate(c) }

func D2SBalanceInfo(c *gin.Context) {
	var accounts []model.D2SBalanceAccount
	if err := model.DB.Where("user_id = ?", c.GetInt("id")).Order("currency ASC").Find(&accounts).Error; err != nil {
		d2sError(c, err)
		return
	}
	d2sSuccess(c, http.StatusOK, gin.H{"accounts": accounts})
}

func D2SBalanceTransactions(c *gin.Context) {
	var rows []model.D2SBalanceTransaction
	if err := model.DB.Where("user_id = ?", c.GetInt("id")).Order("created_at DESC").Limit(500).Find(&rows).Error; err != nil {
		d2sError(c, err)
		return
	}
	d2sSuccess(c, http.StatusOK, gin.H{"transactions": rows})
}

func D2SInviteInfo(c *gin.Context) {
	user, err := model.GetUserById(c.GetInt("id"), false)
	if err != nil {
		d2sError(c, err)
		return
	}
	if user.AffCode == "" {
		user.AffCode = common.GetRandomString(8)
		if err := user.Update(false); err != nil {
			d2sError(c, err)
			return
		}
	}
	var count int64
	_ = model.DB.Model(&model.D2SInviteReward{}).Where("inviter_user_id = ? AND status = ?", user.Id, "credited").Count(&count).Error
	d2sSuccess(c, http.StatusOK, gin.H{"invite_code": user.AffCode, "rewarded_invitees": count})
}

func D2SInviteRecords(c *gin.Context) {
	var rows []model.D2SInviteReward
	if err := model.DB.Where("inviter_user_id = ?", c.GetInt("id")).Order("created_at DESC").Find(&rows).Error; err != nil {
		d2sError(c, err)
		return
	}
	d2sSuccess(c, http.StatusOK, gin.H{"records": rows})
}

type d2sWithdrawalRequest struct {
	AmountMinor   int64  `json:"amount_minor"`
	AlipayAccount string `json:"alipay_account"`
	RealName      string `json:"real_name"`
}

func D2SWithdrawalCreate(c *gin.Context) {
	var request d2sWithdrawalRequest
	if err := common.DecodeJson(c.Request.Body, &request); err != nil {
		d2sInvalidInput(c, "invalid JSON body")
		return
	}
	row, err := model.CreateD2SWithdrawal(c.GetInt("id"), request.AmountMinor, request.AlipayAccount, request.RealName, time.Now().Unix())
	if err != nil {
		d2sError(c, err)
		return
	}
	d2sSuccess(c, http.StatusCreated, row)
}

func D2SWithdrawalStatus(c *gin.Context) {
	var rows []model.D2SWithdrawalRequest
	if err := model.DB.Where("user_id = ?", c.GetInt("id")).Order("created_at DESC").Find(&rows).Error; err != nil {
		d2sError(c, err)
		return
	}
	d2sSuccess(c, http.StatusOK, gin.H{"withdrawals": rows})
}

type d2sPaymentBridgeEvent struct {
	EventID     string `json:"event_id"`
	OrderID     string `json:"order_id"`
	EventType   string `json:"event_type"`
	AmountMinor int64  `json:"amount_minor"`
	Currency    string `json:"currency"`
}

func D2SPaymentWebhook(c *gin.Context) {
	provider := strings.ToLower(strings.TrimSpace(c.Param("provider")))
	secret := strings.TrimSpace(os.Getenv("D2S_PAYMENT_BRIDGE_SECRET_" + strings.ToUpper(strings.ReplaceAll(provider, "-", "_"))))
	if secret == "" {
		secret = strings.TrimSpace(os.Getenv("D2S_PAYMENT_BRIDGE_SECRET"))
	}
	if secret == "" {
		d2sError(c, model.ErrD2SPaymentMismatch)
		return
	}
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		d2sInvalidInput(c, "cannot read webhook body")
		return
	}
	expected := hmac.New(sha256.New, []byte(secret))
	_, _ = expected.Write(body)
	provided, err := hex.DecodeString(strings.TrimSpace(c.GetHeader("X-D2S-Signature")))
	if err != nil || !hmac.Equal(expected.Sum(nil), provided) {
		d2sError(c, model.ErrD2SPaymentMismatch)
		return
	}
	var event d2sPaymentBridgeEvent
	if err := common.Unmarshal(body, &event); err != nil {
		d2sInvalidInput(c, "invalid webhook JSON")
		return
	}
	payloadDigest := sha256.Sum256(body)
	order, err := model.ProcessD2SPaymentEvent(provider, event.EventID, event.OrderID, event.EventType, event.AmountMinor, event.Currency, hex.EncodeToString(payloadDigest[:]), time.Now().Unix())
	if err != nil {
		d2sError(c, err)
		return
	}
	d2sSuccess(c, http.StatusOK, order)
}

func D2SAdminWithdrawals(c *gin.Context) {
	var rows []model.D2SWithdrawalRequest
	query := model.DB.Order("created_at DESC").Limit(500)
	if status := strings.TrimSpace(c.Query("status")); status != "" {
		query = query.Where("status = ?", status)
	}
	if err := query.Find(&rows).Error; err != nil {
		d2sError(c, err)
		return
	}
	d2sSuccess(c, http.StatusOK, gin.H{"withdrawals": rows})
}

func D2SAdminOrders(c *gin.Context) {
	var rows []model.D2SOrder
	query := model.DB.Order("created_at DESC").Limit(500)
	if status := strings.TrimSpace(c.Query("status")); status != "" {
		query = query.Where("status = ?", status)
	}
	if userID := strings.TrimSpace(c.Query("user_id")); userID != "" {
		query = query.Where("user_id = ?", userID)
	}
	if err := query.Find(&rows).Error; err != nil {
		d2sError(c, err)
		return
	}
	d2sSuccess(c, http.StatusOK, gin.H{"orders": rows})
}

func D2SAdminBalances(c *gin.Context) {
	var rows []model.D2SBalanceAccount
	query := model.DB.Order("updated_at DESC").Limit(500)
	if c.Query("negative") == "true" {
		query = query.Where("available_minor < 0")
	}
	if userID := strings.TrimSpace(c.Query("user_id")); userID != "" {
		query = query.Where("user_id = ?", userID)
	}
	if err := query.Find(&rows).Error; err != nil {
		d2sError(c, err)
		return
	}
	d2sSuccess(c, http.StatusOK, gin.H{"accounts": rows})
}

func D2SAdminSigningKeys(c *gin.Context) {
	var rows []model.D2SSigningKey
	if err := model.DB.Select("key_id, algorithm, public_jwk, status, created_at, retired_at").Order("created_at DESC").Find(&rows).Error; err != nil {
		d2sError(c, err)
		return
	}
	type signingKeyResponse struct {
		KeyID     string `json:"key_id"`
		Algorithm string `json:"algorithm"`
		PublicJWK string `json:"public_jwk"`
		Status    string `json:"status"`
		CreatedAt int64  `json:"created_at"`
		RetiredAt int64  `json:"retired_at"`
		IsCurrent bool   `json:"is_current"`
	}
	keys := make([]signingKeyResponse, 0, len(rows))
	for _, row := range rows {
		keys = append(keys, signingKeyResponse{
			KeyID: row.KeyID, Algorithm: row.Algorithm, PublicJWK: row.PublicJWK,
			Status: row.Status, CreatedAt: row.CreatedAt, RetiredAt: row.RetiredAt,
			IsCurrent: service.IsD2SCurrentSigningKey(row.KeyID),
		})
	}
	d2sSuccess(c, http.StatusOK, gin.H{"keys": keys})
}

func D2SAdminSigningKeyReview(c *gin.Context) {
	var request struct {
		Status string `json:"status"`
	}
	if err := common.DecodeJson(c.Request.Body, &request); err != nil || strings.TrimSpace(request.Status) != "retired" {
		d2sInvalidInput(c, "status must be retired")
		return
	}
	if err := service.RetireD2SSigningKey(c.Param("id"), time.Now().Unix()); err != nil {
		d2sError(c, err)
		return
	}
	D2SAdminSigningKeys(c)
}

func D2SAdminReconciliation(c *gin.Context) {
	startAt, endAt := model.D2SPreviousUTCWindow(time.Now())
	if value := strings.TrimSpace(c.Query("start_at")); value != "" {
		parsed, err := strconv.ParseInt(value, 10, 64)
		if err != nil {
			d2sInvalidInput(c, "start_at must be a Unix timestamp")
			return
		}
		startAt = parsed
	}
	if value := strings.TrimSpace(c.Query("end_at")); value != "" {
		parsed, err := strconv.ParseInt(value, 10, 64)
		if err != nil {
			d2sInvalidInput(c, "end_at must be a Unix timestamp")
			return
		}
		endAt = parsed
	}
	report, err := model.ReconcileD2SPayments(startAt, endAt)
	if err != nil {
		d2sError(c, err)
		return
	}
	d2sSuccess(c, http.StatusOK, report)
}

type d2sAdminReviewRequest struct {
	Status string `json:"status"`
	Note   string `json:"note"`
}

func D2SAdminWithdrawalReview(c *gin.Context) {
	var request d2sAdminReviewRequest
	if err := common.DecodeJson(c.Request.Body, &request); err != nil {
		d2sInvalidInput(c, "status is required")
		return
	}
	row, err := model.ReviewD2SWithdrawal(c.GetInt("id"), c.Param("id"), request.Status, request.Note, time.Now().Unix())
	if err != nil {
		d2sError(c, err)
		return
	}
	d2sSuccess(c, http.StatusOK, row)
}

func D2SAdminUnbindRequests(c *gin.Context) {
	var rows []model.D2SManualUnbindRequest
	query := model.DB.Order("created_at DESC").Limit(500)
	if status := strings.TrimSpace(c.Query("status")); status != "" {
		query = query.Where("status = ?", status)
	}
	if err := query.Find(&rows).Error; err != nil {
		d2sError(c, err)
		return
	}
	d2sSuccess(c, http.StatusOK, gin.H{"requests": rows})
}

func D2SAdminUnbindReview(c *gin.Context) {
	var request d2sAdminReviewRequest
	if err := common.DecodeJson(c.Request.Body, &request); err != nil {
		d2sInvalidInput(c, "status is required")
		return
	}
	row, err := model.ReviewD2SManualUnbind(c.GetInt("id"), c.Param("id"), request.Status, request.Note, time.Now().Unix())
	if err != nil {
		d2sError(c, err)
		return
	}
	d2sSuccess(c, http.StatusOK, row)
}

type d2sAdminRegionRequest struct {
	Region string `json:"region"`
}

func D2SAdminSetRegion(c *gin.Context) {
	userID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		d2sInvalidInput(c, "invalid user id")
		return
	}
	var request d2sAdminRegionRequest
	if err := common.DecodeJson(c.Request.Body, &request); err != nil {
		d2sInvalidInput(c, "region is required")
		return
	}
	profile, err := model.SetD2SUserRegion(c.GetInt("id"), userID, request.Region, time.Now().Unix())
	if err != nil {
		d2sError(c, err)
		return
	}
	d2sSuccess(c, http.StatusOK, profile)
}
