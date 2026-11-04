package controller

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type d2SAlipayRefundRequest struct {
	AmountMinor    int64  `json:"amount_minor"`
	Reason         string `json:"reason"`
	IdempotencyKey string `json:"idempotency_key"`
}

func D2SAdminAlipayRefunds(c *gin.Context) {
	var refunds []model.D2SAlipayRefund
	if err := model.DB.Where("order_id = ?", c.Param("id")).Order("created_at DESC").Find(&refunds).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "code": "REFUNDS_UNAVAILABLE"})
		return
	}
	if err := writeD2SAdminAudit(c, "VIEW_ALIPAY_REFUNDS", c.Param("id"), "SUCCESS", map[string]any{"count": len(refunds)}); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "code": "AUDIT_WRITE_FAILED"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"refunds": refunds}})
}

func D2SAdminAlipayRefundReconcile(c *gin.Context) {
	outRequestNo := strings.TrimSpace(c.Param("out_request_no"))
	var refund model.D2SAlipayRefund
	if err := model.DB.Where("out_request_no = ?", outRequestNo).First(&refund).Error; err != nil {
		respondD2SAdminAuditedFailure(c, "QUERY_ALIPAY_REFUND", outRequestNo, "refund_not_found", http.StatusNotFound, "REFUND_NOT_FOUND")
		return
	}
	if refund.Status == "completed" {
		if err := writeD2SAdminAudit(c, "QUERY_ALIPAY_REFUND", outRequestNo, "SUCCESS", map[string]any{"status": "completed"}); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"success": false, "code": "AUDIT_WRITE_FAILED"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"success": true, "data": refund})
		return
	}
	var order model.D2SOrder
	if err := model.DB.Where("id = ?", refund.OrderID).First(&order).Error; err != nil {
		respondD2SAdminAuditedFailure(c, "QUERY_ALIPAY_REFUND", outRequestNo, "order_not_found", http.StatusNotFound, "ORDER_NOT_FOUND")
		return
	}
	trade, err := queryAlipayPCTrade(&order)
	if err != nil || (trade.TradeStatus != "TRADE_SUCCESS" && trade.TradeStatus != "TRADE_FINISHED") {
		respondD2SAdminAuditedFailure(c, "QUERY_ALIPAY_REFUND", outRequestNo, "payment_not_verified", http.StatusBadGateway, "REFUND_QUERY_FAILED")
		return
	}
	outTradeNo, err := alipayOutTradeNo(order.ID)
	if err != nil {
		respondD2SAdminAuditedFailure(c, "QUERY_ALIPAY_REFUND", outRequestNo, "invalid_order", http.StatusBadRequest, "ORDER_INVALID")
		return
	}
	raw, err := callAlipayPCAPI("alipay.trade.fastpay.refund.query", map[string]string{"out_trade_no": outTradeNo, "out_request_no": outRequestNo})
	if err != nil {
		respondD2SAdminAuditedFailure(c, "QUERY_ALIPAY_REFUND", outRequestNo, "provider_query_failed", http.StatusBadGateway, "REFUND_QUERY_FAILED")
		return
	}
	var result struct {
		Code         string `json:"code"`
		OutTradeNo   string `json:"out_trade_no"`
		OutRequestNo string `json:"out_request_no"`
		TradeNo      string `json:"trade_no"`
		RefundAmount string `json:"refund_amount"`
		RefundStatus string `json:"refund_status"`
	}
	if err := common.Unmarshal(raw, &result); err != nil || result.Code != "10000" || result.OutTradeNo != outTradeNo || result.OutRequestNo != outRequestNo || result.TradeNo != trade.TradeNo {
		respondD2SAdminAuditedFailure(c, "QUERY_ALIPAY_REFUND", outRequestNo, "provider_result_mismatch", http.StatusBadGateway, "REFUND_QUERY_FAILED")
		return
	}
	amount, amountErr := alipayAmountMinor(result.RefundAmount)
	if amountErr != nil || amount != refund.AmountMinor || result.RefundStatus != "REFUND_SUCCESS" {
		if err := writeD2SAdminAudit(c, "QUERY_ALIPAY_REFUND", outRequestNo, "PENDING", map[string]any{"provider_status": result.RefundStatus}); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"success": false, "code": "AUDIT_WRITE_FAILED"})
			return
		}
		c.JSON(http.StatusAccepted, gin.H{"success": true, "data": refund, "pending": true})
		return
	}
	if err := model.CompleteD2SAlipayRefund(outRequestNo, result.TradeNo, time.Now().UTC().Unix()); err != nil {
		respondD2SAdminAuditedFailure(c, "QUERY_ALIPAY_REFUND", outRequestNo, "local_commit_failed", http.StatusInternalServerError, "REFUND_RECONCILIATION_REQUIRED")
		return
	}
	if err := writeD2SAdminAudit(c, "QUERY_ALIPAY_REFUND", outRequestNo, "SUCCESS", map[string]any{"amount_minor": amount}); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "code": "AUDIT_WRITE_FAILED"})
		return
	}
	refund.Status = "completed"
	refund.ProviderTrade = result.TradeNo
	c.JSON(http.StatusOK, gin.H{"success": true, "data": refund})
}

func D2SAdminAlipayRefund(c *gin.Context) {
	orderID := strings.TrimSpace(c.Param("id"))
	var request d2SAlipayRefundRequest
	if err := common.DecodeJson(c.Request.Body, &request); err != nil {
		respondD2SAdminAuditedFailure(c, "REFUND_ALIPAY_ORDER", orderID, "invalid_request", http.StatusBadRequest, "INVALID_REQUEST")
		return
	}
	request.Reason = strings.TrimSpace(request.Reason)
	request.IdempotencyKey = strings.TrimSpace(request.IdempotencyKey)
	if request.AmountMinor <= 0 || request.AmountMinor > 10_000_000_000 || len(request.Reason) > 500 || !alipayRefundRequestNoPattern.MatchString(request.IdempotencyKey) {
		respondD2SAdminAuditedFailure(c, "REFUND_ALIPAY_ORDER", orderID, "invalid_refund_parameters", http.StatusBadRequest, "INVALID_REFUND")
		return
	}
	var prior model.D2SAlipayRefund
	priorErr := model.DB.Where("out_request_no = ?", request.IdempotencyKey).First(&prior).Error
	if priorErr == nil {
		if prior.OrderID != orderID || prior.AmountMinor != request.AmountMinor {
			respondD2SAdminAuditedFailure(c, "REFUND_ALIPAY_ORDER", orderID, "idempotency_conflict", http.StatusConflict, "IDEMPOTENCY_CONFLICT")
			return
		}
		if err := writeD2SAdminAudit(c, "REFUND_ALIPAY_ORDER", orderID, "IDEMPOTENT_REPLAY", map[string]any{"out_request_no": request.IdempotencyKey, "status": prior.Status}); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"success": false, "code": "AUDIT_WRITE_FAILED"})
			return
		}
		c.JSON(http.StatusAccepted, gin.H{"success": true, "data": prior, "pending": prior.Status == "pending"})
		return
	} else if !errors.Is(priorErr, gorm.ErrRecordNotFound) {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "code": "REFUNDS_UNAVAILABLE"})
		return
	}
	var order model.D2SOrder
	if err := model.DB.Where("id = ? AND provider = ?", orderID, "alipay_pc").First(&order).Error; err != nil {
		respondD2SAdminAuditedFailure(c, "REFUND_ALIPAY_ORDER", orderID, "order_not_found", http.StatusNotFound, "ORDER_NOT_FOUND")
		return
	}
	trade, err := queryAlipayPCTrade(&order)
	if err != nil || order.Status != model.D2SOrderPaid || (trade.TradeStatus != "TRADE_SUCCESS" && trade.TradeStatus != "TRADE_FINISHED") {
		respondD2SAdminAuditedFailure(c, "REFUND_ALIPAY_ORDER", orderID, "payment_not_confirmed", http.StatusConflict, "ORDER_NOT_REFUNDABLE")
		return
	}
	now := time.Now().UTC().Unix()
	refund := &model.D2SAlipayRefund{ID: uuid.NewString(), OrderID: order.ID, OutRequestNo: request.IdempotencyKey, AmountMinor: request.AmountMinor, Reason: request.Reason, Status: "pending", OperatorID: c.GetInt("d2s_admin_id"), CreatedAt: now, UpdatedAt: now}
	if err := model.CreateD2SAlipayRefund(refund); err != nil {
		respondD2SAdminAuditedFailure(c, "REFUND_ALIPAY_ORDER", orderID, "refund_limit_or_state", http.StatusConflict, "REFUND_NOT_ALLOWED")
		return
	}
	if err := writeD2SAdminAudit(c, "REFUND_ALIPAY_ORDER", orderID, "REQUESTED", map[string]any{"amount_minor": request.AmountMinor, "out_request_no": request.IdempotencyKey}); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "code": "AUDIT_WRITE_FAILED"})
		return
	}
	outTradeNo, _ := alipayOutTradeNo(order.ID)
	raw, err := callAlipayPCAPI("alipay.trade.refund", map[string]string{
		"out_trade_no": outTradeNo, "refund_amount": fmt.Sprintf("%d.%02d", request.AmountMinor/100, request.AmountMinor%100),
		"out_request_no": request.IdempotencyKey, "refund_reason": request.Reason,
	})
	if err != nil {
		if auditErr := writeD2SAdminAudit(c, "REFUND_ALIPAY_ORDER_RESULT", orderID, "PENDING", map[string]any{"amount_minor": request.AmountMinor, "out_request_no": request.IdempotencyKey}); auditErr != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"success": false, "code": "AUDIT_WRITE_FAILED"})
			return
		}
		c.JSON(http.StatusAccepted, gin.H{"success": true, "data": refund, "pending": true})
		return
	}
	var providerResult struct {
		Code         string `json:"code"`
		OutTradeNo   string `json:"out_trade_no"`
		TradeNo      string `json:"trade_no"`
		OutRequestNo string `json:"out_request_no"`
		RefundFee    string `json:"refund_fee"`
	}
	if err := common.Unmarshal(raw, &providerResult); err != nil {
		providerResult.Code = ""
	}
	actualMinor, amountErr := alipayAmountMinor(providerResult.RefundFee)
	if amountErr != nil || providerResult.Code != "10000" || providerResult.OutTradeNo != outTradeNo || (providerResult.OutRequestNo != "" && providerResult.OutRequestNo != request.IdempotencyKey) || providerResult.TradeNo != trade.TradeNo || actualMinor != request.AmountMinor {
		if auditErr := writeD2SAdminAudit(c, "REFUND_ALIPAY_ORDER_RESULT", orderID, "PENDING", map[string]any{"out_request_no": request.IdempotencyKey, "reason": "provider_result_requires_reconciliation"}); auditErr != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"success": false, "code": "AUDIT_WRITE_FAILED"})
			return
		}
		c.JSON(http.StatusAccepted, gin.H{"success": true, "data": refund, "pending": true})
		return
	}
	if err := model.CompleteD2SAlipayRefund(request.IdempotencyKey, providerResult.TradeNo, time.Now().UTC().Unix()); err != nil {
		_ = writeD2SAdminAudit(c, "REFUND_ALIPAY_ORDER", orderID, "FAILURE", map[string]any{"out_request_no": request.IdempotencyKey, "reason": "local_commit_failed"})
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "code": "REFUND_RECONCILIATION_REQUIRED"})
		return
	}
	if err := writeD2SAdminAudit(c, "REFUND_ALIPAY_ORDER", orderID, "SUCCESS", map[string]any{"amount_minor": request.AmountMinor, "out_request_no": request.IdempotencyKey}); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "code": "AUDIT_WRITE_FAILED"})
		return
	}
	refund.Status = "completed"
	refund.ProviderTrade = providerResult.TradeNo
	c.JSON(http.StatusOK, gin.H{"success": true, "data": refund})
}

func D2SAdminAlipayCloseOrder(c *gin.Context) {
	orderID := strings.TrimSpace(c.Param("id"))
	var order model.D2SOrder
	if err := model.DB.Where("id = ? AND provider = ?", orderID, "alipay_pc").First(&order).Error; err != nil {
		respondD2SAdminAuditedFailure(c, "CLOSE_ALIPAY_ORDER", orderID, "order_not_found", http.StatusNotFound, "ORDER_NOT_FOUND")
		return
	}
	if order.Status != model.D2SOrderPending {
		respondD2SAdminAuditedFailure(c, "CLOSE_ALIPAY_ORDER", orderID, "order_not_pending", http.StatusConflict, "ORDER_NOT_PENDING")
		return
	}
	trade, err := queryAlipayPCTrade(&order)
	if err != nil {
		respondD2SAdminAuditedFailure(c, "CLOSE_ALIPAY_ORDER", orderID, "provider_state_uncertain", http.StatusBadGateway, "PROVIDER_STATE_UNCERTAIN")
		return
	}
	if trade.TradeStatus == "TRADE_SUCCESS" || trade.TradeStatus == "TRADE_FINISHED" {
		amount, amountErr := alipayAmountMinor(trade.TotalAmount)
		if amountErr == nil {
			digest := sha256.Sum256([]byte("manual-query:" + trade.TradeNo))
			_, _ = model.ProcessD2SPaymentEvent("alipay_pc", "query:"+trade.TradeNo, order.ID, "paid", amount, "CNY", hex.EncodeToString(digest[:]), time.Now().UTC().Unix())
		}
		respondD2SAdminAuditedFailure(c, "CLOSE_ALIPAY_ORDER", orderID, "already_paid", http.StatusConflict, "ORDER_ALREADY_PAID")
		return
	}
	if trade.TradeStatus == "TRADE_CLOSED" {
		digest := sha256.Sum256([]byte("query-closed:" + order.ID))
		_, err := model.ProcessD2SPaymentEvent("alipay_pc", "query-closed:"+order.ID, order.ID, "canceled", order.GatewayMinor, "CNY", hex.EncodeToString(digest[:]), time.Now().UTC().Unix())
		result := "SUCCESS"
		if err != nil {
			result = "FAILURE"
		}
		if auditErr := writeD2SAdminAudit(c, "CLOSE_ALIPAY_ORDER", orderID, result, map[string]any{"provider_status": trade.TradeStatus}); auditErr != nil || err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"success": false, "code": "CLOSE_RECONCILIATION_REQUIRED"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"success": true})
		return
	}
	if trade.TradeStatus != "WAIT_BUYER_PAY" {
		respondD2SAdminAuditedFailure(c, "CLOSE_ALIPAY_ORDER", orderID, "provider_close_unconfirmed", http.StatusBadGateway, "CLOSE_UNCONFIRMED")
		return
	}
	if err := writeD2SAdminAudit(c, "CLOSE_ALIPAY_ORDER", orderID, "REQUESTED", map[string]any{"provider_status": trade.TradeStatus}); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "code": "AUDIT_WRITE_FAILED"})
		return
	}
	if err := closeAlipayPCTrade(&order); err != nil {
		respondD2SAdminAuditedFailure(c, "CLOSE_ALIPAY_ORDER_RESULT", orderID, "provider_close_unconfirmed", http.StatusBadGateway, "CLOSE_UNCONFIRMED")
		return
	}
	digest := sha256.Sum256([]byte("manual-close:" + order.ID))
	_, err = model.ProcessD2SPaymentEvent("alipay_pc", "close:"+order.ID, order.ID, "canceled", order.GatewayMinor, "CNY", hex.EncodeToString(digest[:]), time.Now().UTC().Unix())
	result := "SUCCESS"
	if err != nil {
		result = "FAILURE"
	}
	auditErr := writeD2SAdminAudit(c, "CLOSE_ALIPAY_ORDER", orderID, result, map[string]any{"provider_status": "TRADE_CLOSED"})
	if auditErr != nil || err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "code": "CLOSE_RECONCILIATION_REQUIRED"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}

var alipayBillDatePattern = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)
var alipayRefundRequestNoPattern = regexp.MustCompile(`^[A-Za-z0-9._:-]{8,128}$`)

func isTrustedAlipayBillHost(host string) bool {
	host = strings.ToLower(strings.TrimSuffix(strings.TrimSpace(host), "."))
	return host == "alipay.com" || strings.HasSuffix(host, ".alipay.com") || host == "alipayobjects.com" || strings.HasSuffix(host, ".alipayobjects.com")
}

func isFutureAlipayBillDate(date time.Time) bool {
	location, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		location = time.FixedZone("CST", 8*60*60)
	}
	year, month, day := date.Date()
	requested := time.Date(year, month, day, 0, 0, 0, 0, location)
	today := time.Now().In(location)
	today = time.Date(today.Year(), today.Month(), today.Day(), 0, 0, 0, 0, location)
	return requested.After(today)
}

func D2SAdminAlipayBill(c *gin.Context) {
	date := strings.TrimSpace(c.Query("date"))
	parsed, err := time.Parse("2006-01-02", date)
	if err != nil || !alipayBillDatePattern.MatchString(date) || parsed.Format("2006-01-02") != date || isFutureAlipayBillDate(parsed) {
		respondD2SAdminAuditedFailure(c, "DOWNLOAD_ALIPAY_BILL", "alipay_bill", "invalid_date", http.StatusBadRequest, "INVALID_DATE")
		return
	}
	raw, err := callAlipayPCAPI("alipay.data.dataservice.bill.downloadurl.query", map[string]string{"bill_type": "trade", "bill_date": date})
	if err != nil {
		respondD2SAdminAuditedFailure(c, "DOWNLOAD_ALIPAY_BILL", date, "provider_query_failed", http.StatusBadGateway, "BILL_UNAVAILABLE")
		return
	}
	var result struct {
		Code string `json:"code"`
		URL  string `json:"bill_download_url"`
	}
	if err := common.Unmarshal(raw, &result); err != nil || result.Code != "10000" {
		respondD2SAdminAuditedFailure(c, "DOWNLOAD_ALIPAY_BILL", date, "bill_not_ready", http.StatusNotFound, "BILL_UNAVAILABLE")
		return
	}
	billURL, err := url.Parse(result.URL)
	if err != nil || billURL.Scheme != "https" || billURL.User != nil || billURL.Hostname() == "" || !isTrustedAlipayBillHost(billURL.Hostname()) {
		respondD2SAdminAuditedFailure(c, "DOWNLOAD_ALIPAY_BILL", date, "unsafe_download_url", http.StatusBadGateway, "BILL_UNAVAILABLE")
		return
	}
	client := &http.Client{Timeout: 30 * time.Second, CheckRedirect: func(request *http.Request, via []*http.Request) error {
		if len(via) >= 5 || request.URL.Scheme != "https" || !isTrustedAlipayBillHost(request.URL.Hostname()) {
			return http.ErrUseLastResponse
		}
		return nil
	}}
	response, err := client.Get(billURL.String())
	if err != nil {
		respondD2SAdminAuditedFailure(c, "DOWNLOAD_ALIPAY_BILL", date, "download_failed", http.StatusBadGateway, "BILL_UNAVAILABLE")
		return
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		respondD2SAdminAuditedFailure(c, "DOWNLOAD_ALIPAY_BILL", date, "download_failed", http.StatusBadGateway, "BILL_UNAVAILABLE")
		return
	}
	if err := writeD2SAdminAudit(c, "DOWNLOAD_ALIPAY_BILL", date, "SUCCESS", map[string]any{"bill_type": "trade"}); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "code": "AUDIT_WRITE_FAILED"})
		return
	}
	c.Header("Content-Type", "application/octet-stream")
	c.Header("Content-Disposition", `attachment; filename="alipay_trade_bill_`+strings.ReplaceAll(date, "-", "")+`.zip"`)
	c.Header("X-Content-Type-Options", "nosniff")
	_, _ = io.Copy(c.Writer, io.LimitReader(response.Body, 100<<20))
}
