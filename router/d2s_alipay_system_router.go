package router

import (
	"github.com/QuantumNous/new-api/controller"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/gin-gonic/gin"
)

// The parent option group uses the same RootAuth as the other payment settings.
func registerSystemAlipayRoutes(options *gin.RouterGroup) {
	purchases := options.Group("/d2s-purchases", middleware.D2SSystemPaymentIdentity())
	purchases.GET("", controller.D2SSystemPurchaseSettings)
	purchases.PUT("", controller.D2SSystemPurchaseSettings)
	alipay := options.Group("/alipay", middleware.D2SSystemPaymentIdentity())
	alipay.GET("/settings", controller.D2SAdminAlipayPCSettings)
	alipay.PUT("/settings", controller.D2SAdminAlipayPCSettings)
	alipay.PUT("/config", controller.D2SAdminConfigureAlipayPC)
	alipay.GET("/orders/:id/refunds", controller.D2SAdminAlipayRefunds)
	alipay.POST("/orders/:id/refunds", controller.D2SAdminAlipayRefund)
	alipay.POST("/refunds/:out_request_no/reconcile", controller.D2SAdminAlipayRefundReconcile)
	alipay.POST("/orders/:id/close", controller.D2SAdminAlipayCloseOrder)
	alipay.GET("/bill", controller.D2SAdminAlipayBill)
}
