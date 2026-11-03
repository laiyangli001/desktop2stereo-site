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
	Product             string `json:"product"`
	LicenseID           string `json:"license_id"`
	Provider            string `json:"provider"`
	BalanceMinor        int64  `json:"balance_minor"`
	IdempotencyKey      string `json:"idempotency_key"`
	ExpectedAmountMinor *int64 `json:"expected_amount_minor"`
	ExpectedCurrency    string `json:"expected_currency"`
	ExpectedRegion      string `json:"expected_region"`
}

func D2SOrderPreview(c *gin.Context) {
	if d2sRejectBlockedPurchase(c, "preview") {
		return
	}
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

func replayD2SOrder(c *gin.Context, userID int, request d2sOrderRequest) bool {
	order, err := model.GetD2SOrderByIdempotencyKey(userID, request.IdempotencyKey)
	if err != nil {
		d2sError(c, err)
		return true
	}
	if order == nil {
		return false
	}
	if request.ExpectedAmountMinor == nil ||
		order.Product != request.Product ||
		order.LicenseID != request.LicenseID ||
		order.Provider != strings.ToLower(strings.TrimSpace(request.Provider)) ||
		order.AmountMinor != *request.ExpectedAmountMinor ||
		order.Currency != request.ExpectedCurrency ||
		order.Region != request.ExpectedRegion ||
		order.BalanceMinor != request.BalanceMinor {
		d2sError(c, model.ErrD2SOrderInvalid)
		return true
	}
	d2sSuccess(c, http.StatusOK, order)
	return true
}

func createD2SOrder(c *gin.Context, requiredProduct string) {
	if d2sRejectBlockedPurchase(c, "create") {
		return
	}
	var request d2sOrderRequest
	if err := common.DecodeJson(c.Request.Body, &request); err != nil || strings.TrimSpace(request.IdempotencyKey) == "" {
		d2sInvalidInput(c, "product, provider and idempotency_key are required")
		return
	}
	if requiredProduct != "" && request.Product != requiredProduct {
		d2sInvalidInput(c, "product must be "+requiredProduct)
		return
	}
	if request.ExpectedAmountMinor == nil {
		d2sError(c, model.ErrD2SQuoteChanged)
		return
	}
	userID := c.GetInt("id")
	if replayD2SOrder(c, userID, request) {
		return
	}
	now := time.Now().Unix()
	quote, err := model.QuoteD2SOrder(userID, request.Product, request.LicenseID, request.Provider, now)
	if err != nil {
		if replayD2SOrder(c, userID, request) {
			return
		}
		d2sError(c, err)
		return
	}
	if *request.ExpectedAmountMinor != quote.AmountMinor || request.ExpectedCurrency != quote.Currency || request.ExpectedRegion != quote.Region {
		if replayD2SOrder(c, userID, request) {
			return
		}
		d2sError(c, model.ErrD2SQuoteChanged)
		return
	}
	user, userErr := model.GetUserById(userID, false)
	if userErr != nil || user == nil {
		d2sError(c, model.ErrD2SOrderInvalid)
		return
	}
	purchaseAudit := d2SUserActionLogInput(c, &userID, user.Email, model.D2SActionPurchase, model.D2SActionResultSuccess)
	order, err := model.CreateD2SOrderWithMetadataAndAudit(userID, quote, request.BalanceMinor, request.IdempotencyKey, user.Email, c.ClientIP(), now, &purchaseAudit)
	if err != nil {
		if replayD2SOrder(c, userID, request) {
			return
		}
		d2sError(c, err)
		return
	}
	c.Set("d2s_user_action_recorded", true)
	d2sSuccess(c, http.StatusCreated, order)
}

func D2SOrderCreate(c *gin.Context) { createD2SOrder(c, "") }

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
	seenProviders := make(map[string]struct{}, 7)
	appendProvider := func(provider string) {
		if _, seen := seenProviders[provider]; seen {
			return
		}
		if provider == model.D2SProviderBalance {
			if profile.Region != "" {
				providers = append(providers, provider)
				seenProviders[provider] = struct{}{}
			}
			return
		}
		region, ok := model.D2SProviderRegion(provider)
		if ok && (profile.Region == "" || profile.Region == region) {
			providers = append(providers, provider)
			seenProviders[provider] = struct{}{}
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

func D2SLicensePaidRevoke(c *gin.Context) {
	createD2SOrder(c, model.D2SOrderProductPaidRevoke)
}

func D2SLicenseOfflineExtend(c *gin.Context) {
	createD2SOrder(c, model.D2SOrderProductOfflineExtension)
}

func D2SBalanceInfo(c *gin.Context) {
	var accounts []model.D2SBalanceAccount
	if err := model.DB.Where("user_id = ?", c.GetInt("id")).Order("currency ASC").Find(&accounts).Error; err != nil {
		d2sError(c, err)
		return
	}
	d2sSuccess(c, http.StatusOK, gin.H{
		"accounts": accounts, "min_withdrawal_minor": common.D2SMinWithdrawalMinor,
	})
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
	if err := model.DB.Where("inviter_user_id = ?", c.GetInt("id")).Order("created_at DESC").Limit(500).Find(&rows).Error; err != nil {
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
	if err := model.DB.Select("id", "user_id", "currency", "amount_minor", "status", "reviewed_by", "review_note", "created_at", "updated_at", "paid_at").Where("user_id = ?", c.GetInt("id")).Order("created_at DESC").Find(&rows).Error; err != nil {
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
	query := model.DB.Select("id", "user_id", "currency", "amount_minor", "status", "reviewed_by", "review_note", "created_at", "updated_at", "paid_at").Order("created_at DESC").Limit(500)
	status := strings.TrimSpace(c.Query("status"))
	if status != "" {
		query = query.Where("status = ?", status)
	}
	if err := query.Find(&rows).Error; err != nil {
		respondD2SAdminAuditedFailure(c, "VIEW_WITHDRAWALS", "d2s_withdrawal_requests", "query_failed", http.StatusInternalServerError, "WITHDRAWAL_QUERY_FAILED")
		return
	}
	if err := writeD2SAdminAudit(c, "VIEW_WITHDRAWALS", "d2s_withdrawal_requests", "SUCCESS", map[string]any{
		"status": status, "limit": 500, "count": len(rows),
	}); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "code": "AUDIT_WRITE_FAILED"})
		return
	}
	d2sSuccess(c, http.StatusOK, gin.H{"withdrawals": rows})
}

func D2SAdminWithdrawalPayout(c *gin.Context) {
	withdrawalID := strings.TrimSpace(c.Param("id"))
	if withdrawalID == "" {
		respondD2SAdminAuditedFailure(c, "VIEW_WITHDRAWAL_PAYOUT", "d2s_withdrawal_requests", "invalid_id", http.StatusBadRequest, "INVALID_WITHDRAWAL_ID")
		return
	}
	var row model.D2SWithdrawalRequest
	if err := model.DB.Select("id", "alipay_account", "real_name").Where("id = ? AND status = ?", withdrawalID, "pending").Take(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			respondD2SAdminAuditedFailure(c, "VIEW_WITHDRAWAL_PAYOUT", "d2s_withdrawal_requests:"+withdrawalID, "not_found", http.StatusNotFound, "WITHDRAWAL_NOT_FOUND")
			return
		}
		respondD2SAdminAuditedFailure(c, "VIEW_WITHDRAWAL_PAYOUT", "d2s_withdrawal_requests:"+withdrawalID, "query_failed", http.StatusInternalServerError, "WITHDRAWAL_QUERY_FAILED")
		return
	}
	if err := writeD2SAdminAudit(c, "VIEW_WITHDRAWAL_PAYOUT", "d2s_withdrawal_requests:"+withdrawalID, "SUCCESS", map[string]any{
		"withdrawal_id": withdrawalID,
	}); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "code": "AUDIT_WRITE_FAILED"})
		return
	}
	d2sSuccess(c, http.StatusOK, gin.H{
		"withdrawal_id":  row.ID,
		"alipay_account": row.AlipayAccount,
		"real_name":      row.RealName,
	})
}

func D2SAdminOrders(c *gin.Context) {
	status := strings.ToLower(strings.TrimSpace(c.Query("status")))
	if status != "" {
		switch status {
		case model.D2SOrderPending, model.D2SOrderPaid, model.D2SOrderCanceled, model.D2SOrderChargeback:
		default:
			respondD2SAdminAuditedFailure(c, "VIEW_ADMIN_ORDERS", "d2s_orders", "invalid_status", http.StatusBadRequest, "INVALID_STATUS")
			return
		}
	}

	var parsedUserID *int
	if userID := strings.TrimSpace(c.Query("user_id")); userID != "" {
		value, err := strconv.Atoi(userID)
		if err != nil || value <= 0 {
			respondD2SAdminAuditedFailure(c, "VIEW_ADMIN_ORDERS", "d2s_orders", "invalid_user_id", http.StatusBadRequest, "INVALID_USER_ID")
			return
		}
		parsedUserID = &value
	}

	var rows []model.D2SOrder
	query := model.DB.Order("created_at DESC").Limit(500)
	if status != "" {
		query = query.Where("status = ?", status)
	}
	if parsedUserID != nil {
		query = query.Where("user_id = ?", *parsedUserID)
	}
	if err := query.Find(&rows).Error; err != nil {
		respondD2SAdminAuditedFailure(c, "VIEW_ADMIN_ORDERS", "d2s_orders", "query_failed", http.StatusInternalServerError, "ORDER_QUERY_FAILED")
		return
	}
	if err := writeD2SAdminAudit(c, "VIEW_ADMIN_ORDERS", "d2s_orders", "SUCCESS", map[string]any{
		"status": status, "user_id": parsedUserID, "limit": 500, "count": len(rows),
	}); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "code": "AUDIT_WRITE_FAILED"})
		return
	}
	d2sSuccess(c, http.StatusOK, gin.H{"orders": rows})
}

func D2SAdminPaymentEvents(c *gin.Context) {
	var rows []model.D2SPaymentEvent
	query := model.DB.Order("processed_at DESC").Limit(500)
	if provider := strings.TrimSpace(c.Query("provider")); provider != "" {
		query = query.Where("provider = ?", strings.ToLower(provider))
	}
	if orderID := strings.TrimSpace(c.Query("order_id")); orderID != "" {
		query = query.Where("order_id = ?", orderID)
	}
	if eventType := strings.TrimSpace(c.Query("event_type")); eventType != "" {
		query = query.Where("event_type = ?", strings.ToLower(eventType))
	}
	if err := query.Find(&rows).Error; err != nil {
		respondD2SAdminAuditedFailure(c, "VIEW_PAYMENT_EVENTS", "d2s_payment_events", "query_failed", http.StatusInternalServerError, "PAYMENT_EVENT_QUERY_FAILED")
		return
	}
	if err := writeD2SAdminAudit(c, "VIEW_PAYMENT_EVENTS", "d2s_payment_events", "SUCCESS", map[string]any{
		"provider":   strings.ToLower(strings.TrimSpace(c.Query("provider"))),
		"order_id":   strings.TrimSpace(c.Query("order_id")),
		"event_type": strings.ToLower(strings.TrimSpace(c.Query("event_type"))), "limit": 500, "count": len(rows),
	}); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "code": "AUDIT_WRITE_FAILED"})
		return
	}
	d2sSuccess(c, http.StatusOK, gin.H{"events": rows})
}

func D2SAdminBalances(c *gin.Context) {
	var parsedUserID *int
	if userID := strings.TrimSpace(c.Query("user_id")); userID != "" {
		value, err := strconv.Atoi(userID)
		if err != nil || value <= 0 {
			respondD2SAdminAuditedFailure(c, "VIEW_BALANCES", "d2s_balance_accounts", "invalid_user_id", http.StatusBadRequest, "INVALID_USER_ID")
			return
		}
		parsedUserID = &value
	}

	var rows []model.D2SBalanceAccount
	query := model.DB.Order("updated_at DESC").Limit(500)
	// Negative balances are the audit default; pass negative=false when an
	// administrator explicitly needs the complete balance listing.
	if c.Query("negative") != "false" {
		query = query.Where("available_minor < 0")
	}
	if parsedUserID != nil {
		query = query.Where("user_id = ?", *parsedUserID)
	}
	if err := query.Find(&rows).Error; err != nil {
		respondD2SAdminAuditedFailure(c, "VIEW_BALANCES", "d2s_balance_accounts", "query_failed", http.StatusInternalServerError, "BALANCE_QUERY_FAILED")
		return
	}
	if err := writeD2SAdminAudit(c, "VIEW_BALANCES", "d2s_balance_accounts", "SUCCESS", map[string]any{
		"user_id": parsedUserID, "negative_only": c.Query("negative") != "false", "limit": 500, "count": len(rows),
	}); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "code": "AUDIT_WRITE_FAILED"})
		return
	}
	d2sSuccess(c, http.StatusOK, gin.H{"accounts": rows})
}

func D2SAdminSigningKeys(c *gin.Context) {
	var rows []model.D2SSigningKey
	if err := model.DB.Select("key_id, algorithm, public_jwk, status, created_at, retired_at").Order("created_at DESC").Find(&rows).Error; err != nil {
		respondD2SAdminAuditedFailure(c, "VIEW_SIGNING_KEYS", "d2s_signing_keys", "query_failed", http.StatusInternalServerError, "SIGNING_KEY_QUERY_FAILED")
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
	if err := writeD2SAdminAudit(c, "VIEW_SIGNING_KEYS", "d2s_signing_keys", "SUCCESS", map[string]any{"count": len(keys)}); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "code": "AUDIT_WRITE_FAILED"})
		return
	}
	d2sSuccess(c, http.StatusOK, gin.H{"keys": keys})
}

func D2SAdminSigningKeyReview(c *gin.Context) {
	var request struct {
		Status string `json:"status"`
	}
	if err := common.DecodeJson(c.Request.Body, &request); err != nil || strings.TrimSpace(request.Status) != "retired" {
		respondD2SAdminAuditedFailure(c, "RETIRE_SIGNING_KEY", c.Param("id"), "invalid_request", http.StatusBadRequest, "INVALID_REQUEST")
		return
	}
	keyID := strings.TrimSpace(c.Param("id"))
	audit, err := d2SAdminAuditEntry(c, "RETIRE_SIGNING_KEY", keyID, "SUCCESS", map[string]any{"status": "retired"})
	if err != nil {
		respondD2SAdminAuditedFailure(c, "RETIRE_SIGNING_KEY", keyID, "audit_serialize_failed", http.StatusInternalServerError, "AUDIT_WRITE_FAILED")
		return
	}
	if err := service.RetireD2SSigningKeyWithAudit(keyID, time.Now().UTC().Unix(), audit); err != nil {
		if auditErr := writeD2SAdminAudit(c, "RETIRE_SIGNING_KEY", keyID, "FAILURE", map[string]any{"reason": "retire_failed"}); auditErr != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"success": false, "code": "AUDIT_WRITE_FAILED"})
			return
		}
		d2sError(c, err)
		return
	}
	d2sSuccess(c, http.StatusOK, gin.H{"key_id": keyID, "status": "retired"})
}

func auditD2SReconciliation(c *gin.Context, startAt, endAt int64, result string, details map[string]any) bool {
	details["start_at"] = startAt
	details["end_at"] = endAt
	if err := writeD2SAdminAudit(c, "RUN_RECONCILIATION", "d2s_payment_reconciliation", result, details); err != nil {
		d2sError(c, err)
		return false
	}
	return true
}

func D2SAdminReconciliation(c *gin.Context) {
	startAt, endAt := model.D2SPreviousUTCWindow(time.Now())
	if value := strings.TrimSpace(c.Query("start_at")); value != "" {
		parsed, err := strconv.ParseInt(value, 10, 64)
		if err != nil {
			if auditD2SReconciliation(c, startAt, endAt, "FAILURE", map[string]any{"reason": "invalid_start_at"}) {
				d2sInvalidInput(c, "start_at must be a Unix timestamp")
			}
			return
		}
		startAt = parsed
	}
	if value := strings.TrimSpace(c.Query("end_at")); value != "" {
		parsed, err := strconv.ParseInt(value, 10, 64)
		if err != nil {
			if auditD2SReconciliation(c, startAt, endAt, "FAILURE", map[string]any{"reason": "invalid_end_at"}) {
				d2sInvalidInput(c, "end_at must be a Unix timestamp")
			}
			return
		}
		endAt = parsed
	}
	if startAt <= 0 || endAt <= startAt {
		if auditD2SReconciliation(c, startAt, endAt, "FAILURE", map[string]any{"reason": "invalid_window"}) {
			d2sInvalidInput(c, "end_at must be greater than start_at")
		}
		return
	}
	report, err := model.ReconcileD2SPayments(startAt, endAt)
	if err != nil {
		if !auditD2SReconciliation(c, startAt, endAt, "FAILURE", map[string]any{"reason": "reconciliation_error"}) {
			return
		}
		d2sError(c, err)
		return
	}
	if !auditD2SReconciliation(c, startAt, endAt, "SUCCESS", map[string]any{"mismatch_count": len(report.Mismatches)}) {
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
		respondD2SAdminAuditedFailure(c, "REVIEW_WITHDRAWAL", "d2s_withdrawal_requests:"+c.Param("id"), "invalid_request", http.StatusBadRequest, "INVALID_REQUEST")
		return
	}
	request.Status = strings.ToLower(strings.TrimSpace(request.Status))
	audit, err := d2SAdminAuditEntry(c, "REVIEW_WITHDRAWAL", "d2s_withdrawal_requests:"+c.Param("id"), "SUCCESS", map[string]any{
		"status": request.Status, "note_provided": strings.TrimSpace(request.Note) != "",
	})
	if err != nil {
		respondD2SAdminAuditedFailure(c, "REVIEW_WITHDRAWAL", "d2s_withdrawal_requests:"+c.Param("id"), "audit_serialize_failed", http.StatusInternalServerError, "AUDIT_WRITE_FAILED")
		return
	}
	row, err := model.ReviewD2SWithdrawalWithAudit(c.GetInt("d2s_admin_id"), c.Param("id"), request.Status, request.Note, time.Now().UTC().Unix(), audit)
	if err != nil {
		if auditErr := writeD2SAdminAudit(c, "REVIEW_WITHDRAWAL", "d2s_withdrawal_requests:"+c.Param("id"), "FAILURE", map[string]any{
			"status": request.Status, "reason": "review_failed",
		}); auditErr != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"success": false, "code": "AUDIT_WRITE_FAILED"})
			return
		}
		d2sError(c, err)
		return
	}
	d2sSuccess(c, http.StatusOK, row)
}

func D2SAdminUnbindRequests(c *gin.Context) {
	var rows []model.D2SManualUnbindRequest
	status := strings.TrimSpace(c.Query("status"))
	query := model.DB.Order("created_at DESC").Limit(500)
	if status != "" {
		query = query.Where("status = ?", status)
	}
	if err := query.Find(&rows).Error; err != nil {
		respondD2SAdminAuditedFailure(c, "VIEW_UNBIND_REQUESTS", "d2s_manual_unbind_requests", "query_failed", http.StatusInternalServerError, "UNBIND_QUERY_FAILED")
		return
	}
	if err := writeD2SAdminAudit(c, "VIEW_UNBIND_REQUESTS", "d2s_manual_unbind_requests", "SUCCESS", map[string]any{
		"status": status, "limit": 500, "count": len(rows),
	}); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "code": "AUDIT_WRITE_FAILED"})
		return
	}
	d2sSuccess(c, http.StatusOK, gin.H{"requests": rows})
}

func D2SAdminUnbindReview(c *gin.Context) {
	var request d2sAdminReviewRequest
	if err := common.DecodeJson(c.Request.Body, &request); err != nil {
		respondD2SAdminAuditedFailure(c, "REVIEW_MANUAL_UNBIND", "d2s_manual_unbind_requests:"+c.Param("id"), "invalid_request", http.StatusBadRequest, "INVALID_REQUEST")
		return
	}
	request.Status = strings.ToLower(strings.TrimSpace(request.Status))
	audit, err := d2SAdminAuditEntry(c, "REVIEW_MANUAL_UNBIND", "d2s_manual_unbind_requests:"+c.Param("id"), "SUCCESS", map[string]any{
		"status": request.Status, "note_provided": strings.TrimSpace(request.Note) != "",
	})
	if err != nil {
		respondD2SAdminAuditedFailure(c, "REVIEW_MANUAL_UNBIND", "d2s_manual_unbind_requests:"+c.Param("id"), "audit_serialize_failed", http.StatusInternalServerError, "AUDIT_WRITE_FAILED")
		return
	}
	row, err := model.ReviewD2SManualUnbindWithAudit(c.GetInt("d2s_admin_id"), c.Param("id"), request.Status, request.Note, time.Now().UTC().Unix(), audit)
	if err != nil {
		if auditErr := writeD2SAdminAudit(c, "REVIEW_MANUAL_UNBIND", "d2s_manual_unbind_requests:"+c.Param("id"), "FAILURE", map[string]any{
			"status": request.Status, "reason": "review_failed",
		}); auditErr != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"success": false, "code": "AUDIT_WRITE_FAILED"})
			return
		}
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
		respondD2SAdminAuditedFailure(c, "SET_USER_REGION", c.Param("id"), "invalid_user_id", http.StatusBadRequest, "INVALID_USER_ID")
		return
	}
	var request d2sAdminRegionRequest
	if err := common.DecodeJson(c.Request.Body, &request); err != nil {
		respondD2SAdminAuditedFailure(c, "SET_USER_REGION", strconv.Itoa(userID), "invalid_request", http.StatusBadRequest, "INVALID_REQUEST")
		return
	}
	request.Region = strings.ToUpper(strings.TrimSpace(request.Region))
	audit, err := d2SAdminAuditEntry(c, "SET_USER_REGION", strconv.Itoa(userID), "SUCCESS", map[string]any{"region": request.Region})
	if err != nil {
		respondD2SAdminAuditedFailure(c, "SET_USER_REGION", strconv.Itoa(userID), "audit_serialize_failed", http.StatusInternalServerError, "AUDIT_WRITE_FAILED")
		return
	}
	profile, err := model.SetD2SUserRegionWithAudit(c.GetInt("d2s_admin_id"), userID, request.Region, time.Now().UTC().Unix(), audit)
	if err != nil {
		if auditErr := writeD2SAdminAudit(c, "SET_USER_REGION", strconv.Itoa(userID), "FAILURE", map[string]any{
			"region": request.Region, "reason": "update_failed",
		}); auditErr != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"success": false, "code": "AUDIT_WRITE_FAILED"})
			return
		}
		d2sError(c, err)
		return
	}
	d2sSuccess(c, http.StatusOK, profile)
}
