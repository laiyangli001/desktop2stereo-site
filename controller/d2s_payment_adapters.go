package controller

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/Calcium-Ion/go-epay/epay"
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/shopspring/decimal"
	"github.com/stripe/stripe-go/v81"
	"github.com/waffo-com/waffo-go/core"
	"gorm.io/gorm"
)

func d2sWebhookPayloadInfo(payload []byte) string {
	digest := sha256.Sum256(payload)
	return fmt.Sprintf("payload_bytes=%d payload_sha256=%s", len(payload), hex.EncodeToString(digest[:]))
}

// processVerifiedD2SPayment dispatches a provider event only when the
// provider webhook has already passed its native signature verification.
// Non-D2S order IDs return handled=false so existing new-api billing keeps its
// current path.
func processVerifiedD2SPayment(provider, eventID, orderID, eventType string, amountMinor int64, currency string, payload []byte) (bool, error) {
	orderID = strings.TrimSpace(orderID)
	if orderID == "" {
		return false, nil
	}

	var order model.D2SOrder
	if err := model.DB.Where("id = ?", orderID).First(&order).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return false, nil
		}
		return true, err
	}

	digest := sha256.Sum256(payload)
	_, err := model.ProcessD2SPaymentEvent(
		provider,
		eventID,
		orderID,
		eventType,
		amountMinor,
		currency,
		hex.EncodeToString(digest[:]),
		time.Now().Unix(),
	)
	return true, err
}

func d2sOrderExists(orderID string) (bool, error) {
	if strings.TrimSpace(orderID) == "" {
		return false, nil
	}
	var order model.D2SOrder
	if err := model.DB.Where("id = ?", orderID).First(&order).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return false, nil
		}
		return true, err
	}
	return true, nil
}

func stripeD2SPaymentEvent(event stripe.Event) (eventType string, ok bool) {
	switch event.Type {
	case stripe.EventTypeCheckoutSessionCompleted, stripe.EventTypeCheckoutSessionAsyncPaymentSucceeded:
		if event.GetObjectValue("payment_status") == "paid" || event.Type == stripe.EventTypeCheckoutSessionAsyncPaymentSucceeded {
			return "paid", true
		}
	case stripe.EventTypeCheckoutSessionExpired:
		if event.GetObjectValue("status") == "expired" {
			return "canceled", true
		}
	case stripe.EventTypeCheckoutSessionAsyncPaymentFailed:
		return "failed", true
	case "charge.refunded":
		return "reversed", true
	case "charge.dispute.created":
		return "chargeback", true
	case "charge.dispute.closed":
		// A won dispute restores the payment and must not suspend the license.
		if event.GetObjectValue("status") == "lost" {
			return "chargeback", true
		}
	}
	return "", false
}

// stripeD2SOrderID reads the stable order reference from the event object.
// Checkout sessions use client_reference_id; refund/dispute objects use the
// d2s_order_id metadata copied onto the charge by the payment integration.
func stripeD2SOrderID(event stripe.Event) string {
	if reference := strings.TrimSpace(event.GetObjectValue("client_reference_id")); reference != "" {
		return reference
	}
	var object map[string]json.RawMessage
	if err := common.Unmarshal(event.Data.Raw, &object); err != nil {
		return ""
	}
	var metadata map[string]string
	if raw, ok := object["metadata"]; ok && common.Unmarshal(raw, &metadata) == nil {
		return strings.TrimSpace(metadata["d2s_order_id"])
	}
	return ""
}

func processStripeD2SPayment(event stripe.Event, payload []byte) (bool, error) {
	eventType, ok := stripeD2SPaymentEvent(event)
	if !ok {
		return false, nil
	}
	orderID := stripeD2SOrderID(event)
	exists, err := d2sOrderExists(orderID)
	if err != nil || !exists {
		return exists, err
	}
	rawAmount := event.GetObjectValue("amount_total")
	if event.Type == "charge.refunded" || event.Type == "charge.dispute.created" || event.Type == "charge.dispute.closed" {
		rawAmount = event.GetObjectValue("amount")
		if event.Type == "charge.refunded" {
			rawAmount = event.GetObjectValue("amount_refunded")
		}
	}
	amountMinor, err := strconv.ParseInt(strings.TrimSpace(rawAmount), 10, 64)
	if err != nil || amountMinor < 0 {
		return true, fmt.Errorf("invalid Stripe D2S amount_total %q: %w", rawAmount, model.ErrD2SPaymentMismatch)
	}
	currency := strings.ToUpper(strings.TrimSpace(event.GetObjectValue("currency")))
	return processVerifiedD2SPayment("stripe", event.ID, orderID, eventType, amountMinor, currency, payload)
}

func logD2SProviderError(ctx context.Context, provider string, err error) {
	if err != nil {
		logger.LogError(ctx, fmt.Sprintf("D2S %s payment event failed: %v", provider, err))
	}
}

func processEpayD2SPayment(result *epay.VerifyRes, payload []byte) (bool, error) {
	if result == nil || strings.TrimSpace(result.ServiceTradeNo) == "" {
		return false, nil
	}
	exists, err := d2sOrderExists(result.ServiceTradeNo)
	if err != nil || !exists {
		return exists, err
	}

	var eventType string
	switch result.TradeStatus {
	case epay.StatusTradeSuccess:
		eventType = "paid"
	case "TRADE_CLOSED":
		eventType = "canceled"
	default:
		return true, nil
	}
	amount, err := decimal.NewFromString(strings.TrimSpace(result.Money))
	if err != nil || amount.IsNegative() {
		return true, fmt.Errorf("invalid Epay amount %q: %w", result.Money, model.ErrD2SPaymentMismatch)
	}
	minor := amount.Mul(decimal.NewFromInt(100))
	if !minor.IsInteger() {
		return true, fmt.Errorf("invalid Epay minor amount %q: %w", result.Money, model.ErrD2SPaymentMismatch)
	}
	minorValue, err := strconv.ParseInt(minor.String(), 10, 64)
	if err != nil || minorValue < 0 {
		return true, fmt.Errorf("invalid Epay minor amount %q: %w", result.Money, model.ErrD2SPaymentMismatch)
	}
	if strings.TrimSpace(result.TradeNo) == "" {
		return true, fmt.Errorf("missing Epay provider event ID: %w", model.ErrD2SPaymentMismatch)
	}

	var order model.D2SOrder
	if err := model.DB.Where("id = ?", result.ServiceTradeNo).First(&order).Error; err != nil {
		return true, err
	}
	if !isEpayD2SProvider(order.Provider) {
		return true, fmt.Errorf("Epay event cannot settle provider %q: %w", order.Provider, model.ErrD2SPaymentMismatch)
	}
	return processVerifiedD2SPayment(
		order.Provider, result.TradeNo, result.ServiceTradeNo, eventType,
		minorValue, "CNY", payload,
	)
}

func isEpayD2SProvider(provider string) bool {
	switch strings.ToLower(strings.TrimSpace(provider)) {
	case "epay", "paymentfm", "alipay", "wechat":
		return true
	default:
		return false
	}
}

func processWaffoD2SPayment(result *core.PaymentNotificationResult, payload []byte) (bool, error) {
	if result == nil || strings.TrimSpace(result.MerchantOrderID) == "" {
		return false, nil
	}
	exists, err := d2sOrderExists(result.MerchantOrderID)
	if err != nil || !exists {
		return exists, err
	}

	eventType := ""
	switch result.OrderStatus {
	case core.OrderStatusPaySuccess:
		eventType = "paid"
	case core.OrderStatusOrderClose:
		eventType = "canceled"
	default:
		return true, nil
	}
	eventID := strings.TrimSpace(result.PaymentRequestID)
	if eventID == "" {
		eventID = strings.TrimSpace(result.AcquiringOrderID)
	}
	amount, err := decimal.NewFromString(strings.TrimSpace(result.OrderAmount))
	if err != nil || amount.IsNegative() {
		return true, fmt.Errorf("invalid Waffo amount %q: %w", result.OrderAmount, model.ErrD2SPaymentMismatch)
	}
	minor := amount.Mul(decimal.NewFromInt(100))
	if !minor.IsInteger() {
		return true, fmt.Errorf("invalid Waffo minor amount %q: %w", result.OrderAmount, model.ErrD2SPaymentMismatch)
	}
	minorValue, err := strconv.ParseInt(minor.String(), 10, 64)
	if err != nil || minorValue < 0 || eventID == "" {
		return true, fmt.Errorf("invalid Waffo payment identity or amount: %w", model.ErrD2SPaymentMismatch)
	}
	return processVerifiedD2SPayment(
		"waffo", eventID, result.MerchantOrderID, eventType,
		minorValue, strings.ToUpper(strings.TrimSpace(result.OrderCurrency)), payload,
	)
}

func processWaffoD2SRefund(notification *core.RefundNotification, payload []byte) (bool, error) {
	if notification == nil || notification.Result == nil {
		return false, nil
	}
	result := notification.Result
	if result.RefundStatus == core.RefundStatusFailed || result.RefundStatus == core.RefundStatusInProgress {
		return false, nil
	}
	if result.RefundStatus == core.RefundStatusPartiallyRefunded {
		return true, fmt.Errorf("partial Waffo refund requires reconciliation: %w", model.ErrD2SPaymentMismatch)
	}
	if result.RefundStatus != core.RefundStatusFullyRefunded {
		return false, nil
	}

	originalEventID := strings.TrimSpace(result.OrigPaymentRequestID)
	refundEventID := strings.TrimSpace(result.RefundRequestID)
	if originalEventID == "" || refundEventID == "" {
		return true, fmt.Errorf("missing Waffo refund identity: %w", model.ErrD2SPaymentMismatch)
	}
	var originalEvent model.D2SPaymentEvent
	if err := model.DB.Where("provider = ? AND provider_event_id = ?", "waffo", originalEventID).First(&originalEvent).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return false, nil
		}
		return true, err
	}
	if originalEvent.EventType != "paid" {
		return true, fmt.Errorf("Waffo refund references a non-paid event: %w", model.ErrD2SPaymentMismatch)
	}

	amount, err := decimal.NewFromString(strings.TrimSpace(result.RefundAmount))
	if err != nil || amount.IsNegative() {
		return true, fmt.Errorf("invalid Waffo refund amount %q: %w", result.RefundAmount, model.ErrD2SPaymentMismatch)
	}
	minor := amount.Mul(decimal.NewFromInt(100))
	if !minor.IsInteger() {
		return true, fmt.Errorf("invalid Waffo refund minor amount %q: %w", result.RefundAmount, model.ErrD2SPaymentMismatch)
	}
	minorValue, err := strconv.ParseInt(minor.String(), 10, 64)
	if err != nil || minorValue <= 0 {
		return true, fmt.Errorf("invalid Waffo refund amount %q: %w", result.RefundAmount, model.ErrD2SPaymentMismatch)
	}
	currency := strings.TrimSpace(result.UserCurrency)
	if currency == "" {
		currency = originalEvent.Currency
	}
	return processVerifiedD2SPayment("waffo", refundEventID, originalEvent.OrderID, "reversed", minorValue, currency, payload)
}

func processWaffoPancakeD2SPayment(event *service.WaffoPancakeWebhookEvent, payload []byte) (bool, error) {
	if event == nil || event.NormalizedEventType() != "order.completed" {
		return false, nil
	}
	orderID := strings.TrimSpace(event.Data.OrderMerchantExternalID)
	exists, err := d2sOrderExists(orderID)
	if err != nil || !exists {
		return exists, err
	}
	eventID := strings.TrimSpace(event.EventID)
	if eventID == "" {
		eventID = strings.TrimSpace(event.ID)
	}
	amount, err := decimal.NewFromString(strings.TrimSpace(event.Data.Amount))
	if err != nil || amount.IsNegative() {
		return true, fmt.Errorf("invalid Waffo Pancake amount %q: %w", event.Data.Amount, model.ErrD2SPaymentMismatch)
	}
	minor := amount.Mul(decimal.NewFromInt(100))
	if !minor.IsInteger() {
		return true, fmt.Errorf("invalid Waffo Pancake minor amount %q: %w", event.Data.Amount, model.ErrD2SPaymentMismatch)
	}
	minorValue, err := strconv.ParseInt(minor.String(), 10, 64)
	if err != nil || minorValue < 0 || eventID == "" {
		return true, fmt.Errorf("invalid Waffo Pancake payment identity or amount: %w", model.ErrD2SPaymentMismatch)
	}
	return processVerifiedD2SPayment(
		"waffo_pancake", eventID, orderID, "paid", minorValue,
		strings.ToUpper(strings.TrimSpace(event.Data.Currency)), payload,
	)
}

func processWaffoPancakeD2SRefund(event *service.WaffoPancakeWebhookEvent, payload []byte) (bool, error) {
	if event == nil || event.NormalizedEventType() != "refund.succeeded" {
		return false, nil
	}
	switch event.Data.RefundStatus {
	case core.RefundStatusFailed, core.RefundStatusInProgress:
		return false, nil
	case core.RefundStatusPartiallyRefunded:
		return true, fmt.Errorf("partial Waffo Pancake refund requires reconciliation: %w", model.ErrD2SPaymentMismatch)
	case "":
		// Older webhook payloads may omit RefundStatus; the exact amount check
		// in ProcessD2SPaymentEvent remains the final guard in that case.
	case core.RefundStatusFullyRefunded:
	default:
		return false, nil
	}
	orderID := strings.TrimSpace(event.Data.OrderMerchantExternalID)
	eventID := strings.TrimSpace(event.EventID)
	if eventID == "" {
		eventID = strings.TrimSpace(event.ID)
	}
	if orderID == "" || eventID == "" {
		return true, fmt.Errorf("missing Waffo Pancake refund identity: %w", model.ErrD2SPaymentMismatch)
	}
	amount, err := decimal.NewFromString(strings.TrimSpace(event.Data.Amount))
	if err != nil || amount.IsNegative() {
		return true, fmt.Errorf("invalid Waffo Pancake refund amount %q: %w", event.Data.Amount, model.ErrD2SPaymentMismatch)
	}
	minor := amount.Mul(decimal.NewFromInt(100))
	if !minor.IsInteger() {
		return true, fmt.Errorf("invalid Waffo Pancake refund minor amount %q: %w", event.Data.Amount, model.ErrD2SPaymentMismatch)
	}
	minorValue, err := strconv.ParseInt(minor.String(), 10, 64)
	if err != nil || minorValue <= 0 {
		return true, fmt.Errorf("invalid Waffo Pancake refund amount %q: %w", event.Data.Amount, model.ErrD2SPaymentMismatch)
	}
	exists, err := d2sOrderExists(orderID)
	if err != nil || !exists {
		return exists, err
	}
	return processVerifiedD2SPayment("waffo_pancake", eventID, orderID, "reversed", minorValue, event.Data.Currency, payload)
}
