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
			auth.POST("/register", middleware.D2SUserActionLog("REGISTER"), middleware.CriticalRateLimit(), anonymousRequestBodyLimit, controller.Register)
			auth.POST("/login", middleware.D2SLoginActionLog(), middleware.CriticalRateLimit(), middleware.DisableCache(), anonymousRequestBodyLimit, controller.Login)
			auth.POST("/logout", middleware.D2SUserActionLog("LOGOUT"), middleware.SessionCookieOriginGuard(), middleware.CriticalRateLimit(), middleware.DisableCache(), controller.AuthLogout)
			auth.POST("/refresh", middleware.SessionCookieOriginGuard(), middleware.CriticalRateLimit(), middleware.DisableCache(), controller.RefreshAuth)
			auth.POST("/password-reset/confirm", middleware.D2SUserActionLog("PASSWORD_RESET"), middleware.CriticalRateLimit(), anonymousRequestBodyLimit, controller.ResetPassword)
		}

		device := v1.Group("/device")
		{
			device.POST("/authorize", middleware.CriticalRateLimit(), anonymousRequestBodyLimit, controller.D2SDeviceAuthorize)
			device.POST("/approve", middleware.D2SLoginActionLog(), middleware.UserAuth(), middleware.CriticalRateLimit(), controller.D2SDeviceApprove)
			device.POST("/token", middleware.D2SLoginActionLog(), middleware.CriticalRateLimit(), anonymousRequestBodyLimit, controller.D2SDeviceToken)
			device.POST("/cancel", middleware.CriticalRateLimit(), anonymousRequestBodyLimit, controller.D2SDeviceCancel)
		}

		v1.GET("/license/keys", controller.D2SLicensePublicKeys)
		v1.GET("/support/config", controller.D2SSupportConfig)
		v1.POST("/support/tickets", middleware.UserAuth(), middleware.CriticalRateLimit(), anonymousRequestBodyLimit, controller.D2SSubmitSupportTicket)
		v1.GET("/support/my-tickets", middleware.UserAuth(), controller.D2SUserSupportTickets)
		downloads := v1.Group("/downloads")
		downloads.Use(middleware.D2SUserActionLog("DOWNLOAD"), middleware.UserAuth())
		{
			downloads.POST("/:target/track", middleware.CriticalRateLimit(), controller.D2STrackDownload)
		}
		v1.POST("/webhooks/:provider", middleware.CriticalRateLimit(), anonymousRequestBodyLimit, controller.D2SPaymentWebhook)
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
			license.POST("/online/heartbeat", controller.D2SLicenseOnlineHeartbeat)
			license.POST("/online/logout", controller.D2SLicenseOnlineLogout)
			license.POST("/core/grant", middleware.CriticalRateLimit(), controller.D2SLicenseCoreGrant)
			license.POST("/permanent/confirm", middleware.CriticalRateLimit(), controller.D2SLicensePermanentConfirm)
			license.POST("/manual-unbind", middleware.CriticalRateLimit(), controller.D2SManualUnbindCreate)
			license.GET("/manual-unbind", controller.D2SManualUnbindList)
		}
		licensePurchase := v1.Group("/license")
		licensePurchase.Use(middleware.D2SUserActionLog("PURCHASE"), middleware.UserAuth())
		{
			licensePurchase.POST("/revoke/paid", middleware.CriticalRateLimit(), controller.D2SLicensePaidRevoke)
			licensePurchase.POST("/offline/extend", middleware.CriticalRateLimit(), controller.D2SLicenseOfflineExtend)
		}

		orders := v1.Group("/orders")
		{
			orders.POST("/preview", middleware.D2SUserActionLog("PURCHASE"), middleware.UserAuth(), controller.D2SOrderPreview)
			orders.POST("/create", middleware.D2SUserActionLog("PURCHASE"), middleware.UserAuth(), middleware.CriticalRateLimit(), controller.D2SOrderCreate)
			orders.GET("/providers", middleware.UserAuth(), controller.D2SOrderProviders)
			orders.GET("", middleware.UserAuth(), controller.D2SOrderList)
			orders.GET("/:id", middleware.UserAuth(), controller.D2SOrderGet)
			orders.POST("/:id/checkout", middleware.D2SUserActionLog("PURCHASE"), middleware.UserAuth(), middleware.CriticalRateLimit(), controller.D2SOrderCheckout)
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

		v1.POST("/admin/auth/login", middleware.CriticalRateLimit(), middleware.DisableCache(), anonymousRequestBodyLimit, controller.D2SAdminLogin)
		adminAuth := v1.Group("/admin")
		adminAuth.Use(middleware.D2SAdminAuth())
		{
			adminAuth.GET("/auth/session", controller.D2SAdminSession)
			adminAuth.POST("/auth/logout", middleware.SessionCookieOriginGuard(), controller.D2SAdminLogout)
		}
		accountAdmin := v1.Group("/admin")
		accountAdmin.Use(middleware.D2SAdminAuth(), middleware.D2SAdminRequireRole("account_operator"))
		{
			accountAdmin.GET("/support/tickets", controller.D2SAdminSupportTickets)
			accountAdmin.PUT("/support/tickets/:id", controller.D2SAdminUpdateSupportTicket)
			accountAdmin.PUT("/users/:id/emergency", controller.D2SAdminSetEmergencyControl)
			accountAdmin.GET("/purchases", controller.D2SAdminPurchaseRecords)
			accountAdmin.POST("/users/batch/preview", controller.D2SAdminBatchEmergencyPreview)
			accountAdmin.POST("/users/batch/apply", controller.D2SAdminBatchEmergencyApply)
		}
		auditAdmin := v1.Group("/admin")
		auditAdmin.Use(middleware.D2SAdminAuth(), middleware.D2SAdminRequireRole("audit_reader"))
		{
			auditAdmin.GET("/audit/user-events", controller.D2SAdminUserActionLogs)
			auditAdmin.GET("/audit/registrations", controller.D2SAdminRegistrationRecords)
			auditAdmin.POST("/audit/export-approvals", middleware.SessionCookieOriginGuard(), controller.D2SAdminRequestExportApproval)
			auditAdmin.GET("/audit/export-approvals/:id", controller.D2SAdminGetExportApproval)
			auditAdmin.GET("/audit/user-events/export.csv", controller.D2SAdminExportUserActionLogs)
			auditAdmin.POST("/audit/user-events/:id/verify-hash", controller.D2SAdminVerifyUserActionLog)
			auditAdmin.GET("/audit/registrations/export.csv", controller.D2SAdminExportRegistrationRecords)
		}
		admin := v1.Group("/admin")
		admin.Use(middleware.D2SAdminAuth(), middleware.D2SAdminRequireRole("super_admin"))
		{
			admin.GET("/audit/export-approvals", controller.D2SAdminListExportApprovals)
			admin.PUT("/audit/export-approvals/:id", middleware.SessionCookieOriginGuard(), controller.D2SAdminReviewExportApproval)
			admin.POST("/audit/retention/execute", controller.D2SAdminExecuteAuditRetention)
			admin.GET("/licenses", controller.D2SAdminLicenses)
			admin.GET("/orders", controller.D2SAdminOrders)
			admin.GET("/payment-events", controller.D2SAdminPaymentEvents)
			admin.GET("/balances", controller.D2SAdminBalances)
			admin.GET("/signing-keys", controller.D2SAdminSigningKeys)
			admin.PUT("/signing-keys/:id", controller.D2SAdminSigningKeyReview)
			admin.GET("/withdrawals", controller.D2SAdminWithdrawals)
			admin.PUT("/withdrawals/:id", controller.D2SAdminWithdrawalReview)
			admin.GET("/unbind-requests", controller.D2SAdminUnbindRequests)
			admin.PUT("/unbind-requests/:id", controller.D2SAdminUnbindReview)
			admin.PUT("/users/:id/region", controller.D2SAdminSetRegion)
		}

		reconciliationAdmin := v1.Group("/admin")
		reconciliationAdmin.Use(middleware.D2SAdminReconciliationAuth(), middleware.D2SAdminRequireRole("super_admin"))
		reconciliationAdmin.GET("/reconciliation", controller.D2SAdminReconciliation)
	}
}
