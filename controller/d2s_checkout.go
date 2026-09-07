package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/Calcium-Ion/go-epay/epay"
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting"
	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
	"github.com/stripe/stripe-go/v81"
	"github.com/stripe/stripe-go/v81/checkout/session"
	waffoOrder "github.com/waffo-com/waffo-go/types/order"
)

func D2SOrderCheckout(c *gin.Context) {
	var order model.D2SOrder
	if err := model.DB.Where("id = ? AND user_id = ?", c.Param("id"), c.GetInt("id")).First(&order).Error; err != nil {
		d2sError(c, model.ErrD2SOrderNotFound)
		return
	}
	if order.GatewayMinor <= 0 || order.Status != model.D2SOrderPending || order.ExpiresAt <= time.Now().Unix() {
		d2sError(c, model.ErrD2SCheckoutUnavailable)
		return
	}
	user, err := model.GetUserById(c.GetInt("id"), false)
	if err != nil || user == nil {
		d2sError(c, model.ErrD2SCheckoutUnavailable)
		return
	}
	var checkoutURL string
	var epayCheckout *d2SEpayCheckout
	if order.Provider == "stripe" {
		checkoutURL, err = createD2SStripeCheckout(&order, user)
	} else if order.Provider == "creem" {
		checkoutURL, err = createD2SCreemCheckout(&order, user)
	} else if order.Provider == "waffo_pancake" {
		checkoutURL, err = createD2SWaffoPancakeCheckout(c.Request.Context(), &order, user)
	} else if order.Provider == "waffo" {
		checkoutURL, err = createD2SWaffoCheckout(c.Request.Context(), &order, user)
	} else {
		epayCheckout, err = createD2SEpayCheckout(&order)
	}
	if err != nil {
		d2sError(c, err)
		return
	}
	if epayCheckout != nil {
		d2sSuccess(c, http.StatusOK, gin.H{"order_id": order.ID, "checkout_url": epayCheckout.URL, "method": "POST", "params": epayCheckout.Params})
		return
	}
	d2sSuccess(c, http.StatusOK, gin.H{"order_id": order.ID, "checkout_url": checkoutURL})
}

func createD2SWaffoCheckout(ctx context.Context, order *model.D2SOrder, user *model.User) (string, error) {
	if order == nil || user == nil || order.Provider != "waffo" || order.Region != model.D2SRegionCN || order.Currency != "CNY" || order.GatewayMinor <= 0 || !isWaffoTopUpEnabled() {
		return "", model.ErrD2SCheckoutUnavailable
	}
	sdk, err := getWaffoSDK()
	if err != nil {
		return "", model.ErrD2SCheckoutUnavailable
	}
	notifyURL := service.GetCallbackAddress() + "/api/waffo/webhook"
	if strings.TrimSpace(setting.WaffoNotifyUrl) != "" {
		notifyURL = setting.WaffoNotifyUrl
	}
	returnURL := paymentReturnPath("/d2s")
	if strings.TrimSpace(setting.WaffoReturnUrl) != "" {
		returnURL = setting.WaffoReturnUrl
	}
	response, err := sdk.Order().Create(ctx, &waffoOrder.CreateOrderParams{
		PaymentRequestID: order.ID,
		MerchantOrderID:  order.ID,
		OrderAmount:      decimal.NewFromInt(order.GatewayMinor).Div(decimal.NewFromInt(100)).StringFixed(2),
		OrderCurrency:    order.Currency,
		OrderDescription: "Desktop2Stereo license",
		OrderRequestedAt: time.Now().UTC().Format("2006-01-02T15:04:05.000Z"),
		NotifyURL:        notifyURL,
		MerchantInfo:     &waffoOrder.MerchantInfo{MerchantID: setting.WaffoMerchantId},
		UserInfo: &waffoOrder.UserInfo{
			UserID:       fmt.Sprintf("%d", user.Id),
			UserEmail:    getWaffoUserEmail(user),
			UserTerminal: "WEB",
		},
		PaymentInfo:        &waffoOrder.PaymentInfo{ProductName: "ONE_TIME_PAYMENT"},
		GoodsInfo:          &waffoOrder.GoodsInfo{GoodsName: "Desktop2Stereo license", AppName: common.SystemName},
		SuccessRedirectURL: returnURL,
		FailedRedirectURL:  returnURL,
	}, nil)
	if err != nil || response == nil || !response.IsSuccess() {
		return "", model.ErrD2SCheckoutUnavailable
	}
	data := response.GetData()
	if data == nil {
		return "", model.ErrD2SCheckoutUnavailable
	}
	checkoutURL := data.FetchRedirectURL()
	if strings.TrimSpace(checkoutURL) == "" {
		checkoutURL = data.OrderAction
	}
	if strings.TrimSpace(checkoutURL) == "" {
		return "", model.ErrD2SCheckoutUnavailable
	}
	return checkoutURL, nil
}

type d2SEpayCheckout struct {
	URL    string
	Params map[string]string
}

func createD2SEpayCheckout(order *model.D2SOrder) (*d2SEpayCheckout, error) {
	if order == nil || order.Region != model.D2SRegionCN || order.Currency != "CNY" || order.GatewayMinor <= 0 {
		return nil, model.ErrD2SCheckoutUnavailable
	}
	client := GetEpayClient()
	if client == nil {
		return nil, model.ErrD2SCheckoutUnavailable
	}
	method := order.Provider
	switch method {
	case "wechat":
		method = "wxpay"
	case "epay":
		method = "alipay"
	case "alipay", "wxpay", "paymentfm":
	default:
		return nil, model.ErrD2SCheckoutUnavailable
	}
	callback, err := url.Parse(service.GetCallbackAddress() + "/api/user/epay/notify")
	if err != nil {
		return nil, model.ErrD2SCheckoutUnavailable
	}
	returnURL, err := url.Parse(paymentReturnPath("/d2s"))
	if err != nil {
		return nil, model.ErrD2SCheckoutUnavailable
	}
	uri, params, err := client.Purchase(&epay.PurchaseArgs{
		Type:           method,
		ServiceTradeNo: order.ID,
		Name:           "Desktop2Stereo license",
		Money:          decimal.NewFromInt(order.GatewayMinor).Div(decimal.NewFromInt(100)).StringFixed(2),
		Device:         epay.PC,
		NotifyUrl:      callback,
		ReturnUrl:      returnURL,
	})
	if err != nil || strings.TrimSpace(uri) == "" || len(params) == 0 {
		return nil, model.ErrD2SCheckoutUnavailable
	}
	return &d2SEpayCheckout{URL: uri, Params: params}, nil
}

func createD2SWaffoPancakeCheckout(ctx context.Context, order *model.D2SOrder, user *model.User) (string, error) {
	if order == nil || user == nil || order.Provider != "waffo_pancake" || order.Region != model.D2SRegionINTL || order.Currency != "USD" || order.GatewayMinor <= 0 || !isWaffoPancakeTopUpEnabled() {
		return "", model.ErrD2SCheckoutUnavailable
	}
	expiresInSeconds := 45 * 60
	session, err := service.CreateWaffoPancakeCheckoutSession(ctx, &service.WaffoPancakeCreateSessionParams{
		ProductID:     setting.WaffoPancakeProductID,
		BuyerIdentity: getWaffoPancakeBuyerIdentity(user),
		PriceSnapshot: &service.WaffoPancakePriceSnapshot{
			Amount:      decimal.NewFromInt(order.GatewayMinor).Div(decimal.NewFromInt(100)).StringFixed(2),
			TaxCategory: "saas",
		},
		BuyerEmail:              getWaffoPancakeBuyerEmail(user),
		ExpiresInSeconds:        &expiresInSeconds,
		OrderMerchantExternalID: order.ID,
	})
	if err != nil || session == nil || strings.TrimSpace(session.CheckoutURL) == "" {
		return "", model.ErrD2SCheckoutUnavailable
	}
	return session.CheckoutURL, nil
}

func createD2SCreemCheckout(order *model.D2SOrder, user *model.User) (string, error) {
	if order == nil || user == nil || order.Provider != "creem" || order.Region != model.D2SRegionINTL || order.Currency != "USD" || order.GatewayMinor <= 0 {
		return "", model.ErrD2SCheckoutUnavailable
	}
	var products []CreemProduct
	if err := json.Unmarshal([]byte(setting.CreemProducts), &products); err != nil {
		return "", model.ErrD2SCheckoutUnavailable
	}
	var selected *CreemProduct
	for index := range products {
		product := &products[index]
		price := decimal.NewFromFloat(product.Price).Mul(decimal.NewFromInt(100))
		if product.Currency != "" && strings.EqualFold(product.Currency, order.Currency) && price.IsInteger() && price.IntPart() == order.GatewayMinor {
			selected = product
			break
		}
	}
	if selected == nil || strings.TrimSpace(selected.ProductId) == "" {
		return "", model.ErrD2SCheckoutUnavailable
	}
	if strings.TrimSpace(selected.Name) == "" {
		selected.Name = "Desktop2Stereo license"
	}
	return genCreemLink(context.Background(), order.ID, selected, user.Email, user.Username)
}

func createD2SStripeCheckout(order *model.D2SOrder, user *model.User) (string, error) {
	if order == nil || user == nil || order.Provider != "stripe" || order.Region != model.D2SRegionINTL || order.Currency != "USD" || order.GatewayMinor <= 0 {
		return "", model.ErrD2SCheckoutUnavailable
	}
	if !strings.HasPrefix(setting.StripeApiSecret, "sk_") && !strings.HasPrefix(setting.StripeApiSecret, "rk_") {
		return "", model.ErrD2SCheckoutUnavailable
	}
	stripe.Key = setting.StripeApiSecret
	params := &stripe.CheckoutSessionParams{
		ClientReferenceID: stripe.String(order.ID),
		SuccessURL:        stripe.String(paymentReturnPath("/d2s")),
		CancelURL:         stripe.String(paymentReturnPath("/d2s")),
		LineItems: []*stripe.CheckoutSessionLineItemParams{{
			PriceData: &stripe.CheckoutSessionLineItemPriceDataParams{
				Currency:   stripe.String(strings.ToLower(order.Currency)),
				UnitAmount: stripe.Int64(order.GatewayMinor),
				ProductData: &stripe.CheckoutSessionLineItemPriceDataProductDataParams{
					Name: stripe.String("Desktop2Stereo license"),
				},
			},
			Quantity: stripe.Int64(1),
		}},
		Mode:                stripe.String(string(stripe.CheckoutSessionModePayment)),
		AllowPromotionCodes: stripe.Bool(setting.StripePromotionCodesEnabled),
		Metadata:            map[string]string{"d2s_order_id": order.ID},
	}
	if strings.TrimSpace(user.Email) != "" {
		params.CustomerEmail = stripe.String(user.Email)
	}
	result, err := session.New(params)
	if err != nil || result == nil || strings.TrimSpace(result.URL) == "" {
		return "", model.ErrD2SCheckoutUnavailable
	}
	return result.URL, nil
}
