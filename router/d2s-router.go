package router

import (
	"github.com/QuantumNous/new-api/controller"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/gin-gonic/gin"
)

func registerD2SRoutes(apiRouter *gin.RouterGroup, anonymousRequestBodyLimit gin.HandlerFunc) {
	v1 := apiRouter.Group("/v1")
	{
		auth := v1.Group("/auth")
		{
			auth.POST("/register", middleware.CriticalRateLimit(), anonymousRequestBodyLimit, middleware.TurnstileCheck(), controller.Register)
			auth.POST("/login", middleware.CriticalRateLimit(), middleware.DisableCache(), anonymousRequestBodyLimit, middleware.TurnstileCheck(), controller.Login)
			auth.POST("/logout", middleware.SessionCookieOriginGuard(), middleware.CriticalRateLimit(), middleware.DisableCache(), controller.AuthLogout)
			auth.POST("/refresh", middleware.SessionCookieOriginGuard(), middleware.CriticalRateLimit(), middleware.DisableCache(), controller.RefreshAuth)
			auth.POST("/password-reset/confirm", middleware.CriticalRateLimit(), anonymousRequestBodyLimit, controller.ResetPassword)
		}

		device := v1.Group("/device")
		{
			device.POST("/authorize", middleware.CriticalRateLimit(), anonymousRequestBodyLimit, controller.D2SDeviceAuthorize)
			device.POST("/approve", middleware.UserAuth(), middleware.CriticalRateLimit(), controller.D2SDeviceApprove)
			device.POST("/token", middleware.CriticalRateLimit(), anonymousRequestBodyLimit, controller.D2SDeviceToken)
			device.POST("/cancel", middleware.CriticalRateLimit(), anonymousRequestBodyLimit, controller.D2SDeviceCancel)
		}

		v1.GET("/license/keys", controller.D2SLicensePublicKeys)
		v1.POST("/webhooks/:provider", anonymousRequestBodyLimit, controller.D2SPaymentWebhook)
		license := v1.Group("/license")
		license.Use(middleware.UserAuth())
		{
			license.GET("/list", controller.D2SLicenseList)
			license.GET("/status", controller.D2SLicenseStatus)
			license.POST("/activate", middleware.CriticalRateLimit(), controller.D2SLicenseActivate)
			license.POST("/switch", middleware.CriticalRateLimit(), controller.D2SLicenseSwitch)
			license.POST("/renew", middleware.CriticalRateLimit(), controller.D2SLicenseRenew)
			license.POST("/offline/issue", middleware.CriticalRateLimit(), controller.D2SLicenseRenew)
			license.POST("/change-mode", middleware.CriticalRateLimit(), controller.D2SLicenseChangeMode)
			license.POST("/revoke/free", middleware.CriticalRateLimit(), controller.D2SLicenseFreeRevoke)
			license.POST("/revoke/paid", middleware.CriticalRateLimit(), controller.D2SLicensePaidRevoke)
			license.POST("/offline/extend", middleware.CriticalRateLimit(), controller.D2SLicenseOfflineExtend)
			license.POST("/online/heartbeat", controller.D2SLicenseOnlineHeartbeat)
			license.POST("/online/logout", controller.D2SLicenseOnlineLogout)
			license.POST("/permanent/confirm", middleware.CriticalRateLimit(), controller.D2SLicensePermanentConfirm)
			license.POST("/manual-unbind", middleware.CriticalRateLimit(), controller.D2SManualUnbindCreate)
			license.GET("/manual-unbind", controller.D2SManualUnbindList)
		}

		orders := v1.Group("/orders")
		orders.Use(middleware.UserAuth())
		{
			orders.POST("/preview", controller.D2SOrderPreview)
			orders.POST("/create", middleware.CriticalRateLimit(), controller.D2SOrderCreate)
			orders.GET("/:id", controller.D2SOrderGet)
		}
		invite := v1.Group("/invite")
		invite.Use(middleware.UserAuth())
		{
			invite.GET("/info", controller.D2SInviteInfo)
			invite.GET("/records", controller.D2SInviteRecords)
		}
		balance := v1.Group("/balance")
		balance.Use(middleware.UserAuth())
		{
			balance.GET("/info", controller.D2SBalanceInfo)
			balance.GET("/transactions", controller.D2SBalanceTransactions)
		}
		withdrawal := v1.Group("/withdrawal")
		withdrawal.Use(middleware.UserAuth())
		{
			withdrawal.POST("/request", middleware.CriticalRateLimit(), controller.D2SWithdrawalCreate)
			withdrawal.GET("/status", controller.D2SWithdrawalStatus)
		}

		admin := v1.Group("/admin")
		admin.Use(middleware.AdminAuth())
		{
			admin.GET("/licenses", controller.D2SAdminLicenses)
			admin.GET("/withdrawals", controller.D2SAdminWithdrawals)
			admin.PUT("/withdrawals/:id", controller.D2SAdminWithdrawalReview)
			admin.GET("/unbind-requests", controller.D2SAdminUnbindRequests)
			admin.PUT("/unbind-requests/:id", controller.D2SAdminUnbindReview)
			admin.PUT("/users/:id/region", controller.D2SAdminSetRegion)
		}
	}
}
