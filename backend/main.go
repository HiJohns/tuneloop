package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"tuneloop-backend/database"
	"tuneloop-backend/handlers"
	"tuneloop-backend/internal/tasks"
	"tuneloop-backend/middleware"
	"tuneloop-backend/models"
	"tuneloop-backend/services"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
)

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getAbsPath(relativePath string) string {
	execDir, _ := os.Getwd()
	return filepath.Join(execDir, relativePath)
}

func getWWWPath() string {
	// Check for WWW_PATH env var first (absolute path)
	if wwwPath := os.Getenv("WWW_PATH"); wwwPath != "" {
		return wwwPath
	}
	// Default: relative to service location
	return "../www"
}

func extractPort(urlStr string) string {
	u, err := url.Parse(urlStr)
	if err != nil {
		return "5554"
	}

	if u.Port() != "" {
		return u.Port()
	}

	switch u.Scheme {
	case "https":
		return "443"
	case "http":
		return "80"
	default:
		return "5554"
	}
}

func setupAPIRoutes(r *gin.Engine, iamService *services.IAMService, permRegistry *services.PermissionRegistry) {
	auditWriter := services.NewAuditWriter()
	siteHandler := handlers.NewSiteHandler()
	// New handlers for Issue #345 (Merchant Management + Setup)
	merchantHandler := handlers.NewMerchantHandler()
	setupHandler := handlers.NewSetupHandler()
	siteMemberHandler := handlers.NewSiteMemberHandler()
	iamProxyHandler := handlers.NewIAMProxyHandler()
	staffHandler := &handlers.UserStaffHandler{}
	propertyHandler := handlers.NewPropertyHandler()
	inventoryHandler := handlers.NewInventoryHandler()
	authHandler := handlers.NewAuthHandler(database.GetDB())

	// New handlers for Issue #299 (Maintenance, Appeal, Warehouse, User Rental)
	maintenanceWorkerHandler := handlers.NewMaintenanceWorkerHandler()
	maintenanceSessionHandler := handlers.NewMaintenanceSessionHandler()
	appealHandler := handlers.NewAppealHandler()
	iamClient := services.NewIAMClient()
	permManageHandler := handlers.NewPermissionManageHandler(database.GetDB(), iamClient, permRegistry)
	roleManageHandler := handlers.NewRoleManageHandler(database.GetDB(), iamClient, permRegistry)
	warehouseHandler := handlers.NewWarehouseHandler()
	userRentalHandler := handlers.NewUserRentalHandler()
	userAddressHandler := handlers.NewUserAddressHandler()
	bannerHandler := handlers.NewBannerHandler()

	// Bulk import handler (Issue #423)
	bulkImportHandler := handlers.NewBulkImportHandler(iamClient, permRegistry)

	api := r.Group("/api")
	api.Use(middleware.CultureMiddleware())

	api.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok"})
	})

	api.GET("/config", func(c *gin.Context) {
		iamExternalURL := os.Getenv("BEACONIAM_EXTERNAL_URL")
		if iamExternalURL == "" {
			iamExternalURL = "http://localhost:5552"
		}

		// PC Web configuration — derived from IAM_NAMESPACE + "_web"
		iamNamespace := os.Getenv("IAM_NAMESPACE")
		iamPCClientID := ""
		if iamNamespace != "" {
			iamPCClientID = iamNamespace + "_web"
		}
		if iamPCClientID == "" {
			iamPCClientID = os.Getenv("IAM_PC_CLIENT_ID")
		}
		if iamPCClientID == "" {
			iamPCClientID = "tuneloop-pc"
		}
		iamPCRedirectURI := os.Getenv("EXTERNAL_WEB_URL")
		if iamPCRedirectURI == "" {
			iamPCRedirectURI = "http://localhost:5554"
		}
		iamPCRedirectURI += "/callback"

		// WeChat Mini Program configuration — derived from IAM_NAMESPACE + "_wechat"
		iamWXClientID := ""
		if iamNamespace != "" {
			iamWXClientID = iamNamespace + "_wechat"
		}
		if iamWXClientID == "" {
			iamWXClientID = os.Getenv("IAM_WX_CLIENT_ID")
		}
		if iamWXClientID == "" {
			iamWXClientID = "tuneloop-wx"
		}
		iamWXRedirectURI := os.Getenv("EXTERNAL_MOBILE_URL")
		if iamWXRedirectURI == "" {
			iamWXRedirectURI = "http://localhost:5553"
		}
		iamWXRedirectURI += "/callback"

		c.JSON(http.StatusOK, gin.H{
			"code": 20000,
			"data": gin.H{
				"pc": gin.H{
					"iamExternalUrl": iamExternalURL,
					"iamClientId":    iamPCClientID,
					"iamRedirectUri": iamPCRedirectURI,
				},
				"wx": gin.H{
					"iamExternalUrl": iamExternalURL,
					"iamClientId":    iamWXClientID,
					"iamRedirectUri": iamWXRedirectURI,
				},
				"appName": "TuneLoop",
				"version": "1.0.0",
			},
		})
	})

	api.GET("/config/permissions", func(c *gin.Context) {
		cusPermMapping := gin.H{}
		for code, bit := range permRegistry.GetCusPermMapping() {
			cusPermMapping[code] = bit
		}
		c.JSON(http.StatusOK, gin.H{
			"code": 20000,
			"data": gin.H{
				"sys_perm_mapping": middleware.SysPermBitNames,
				"cus_perm_mapping": cusPermMapping,
			},
		})
	})

	api.GET("/auth/callback", authHandler.Callback)
	api.POST("/auth/callback", authHandler.Callback)
	api.GET("/auth/oidc/authorization-url", authHandler.GetOIDCAuthorizationURL)
	api.POST("/auth/login", authHandler.PostLogin)
	api.POST("/auth/refresh", authHandler.Refresh)
	api.POST("/wx/login", authHandler.WxLogin)
	api.POST("/wx/phone", authHandler.WxPhone)
	// Setup routes (public, no auth required)
	api.GET("/setup/status", setupHandler.GetSetupStatus)
	api.POST("/setup/init", setupHandler.InitializeSystem)
	api.POST("/iam/users/:user_id/invite", iamProxyHandler.InviteUserToMerchant)
	// Public browsing routes (no auth required)
	api.GET("/public/instruments", handlers.GetPublicInstruments)
	api.GET("/public/instruments/:id", handlers.GetPublicInstrumentByID)
	api.GET("/public/instruments/:id/pricing-v2", handlers.GetPublicInstrumentPricingV2)
	api.GET("/public/instruments/:id/media", handlers.GetPublicInstrumentMedia)
	api.GET("/public/instruments/:id/display-media", handlers.GetPublicInstrumentDisplayMedia)
	api.GET("/public/categories", handlers.GetPublicCategories)
	api.GET("/public/sites", handlers.GetPublicSites)
	api.GET("/public/banners", bannerHandler.GetPublicBanners)
	authRequired := api.Group("")
	authRequired.Use(middleware.IAMInterceptor(iamService, iamClient))
	authRequired.Use(middleware.NoCache())
	authRequired.Use(middleware.AuditLogger(auditWriter))
	authRequired.Use(middleware.RequirePasswordNotForceChange())
	{
		// IAM Proxy routes
		iamProxyHandler := handlers.NewIAMProxyHandler()
		authRequired.GET("/iam/users/lookup", iamProxyHandler.LookupUser)
		authRequired.GET("/iam/users/search", iamProxyHandler.SearchUsers)
		authRequired.POST("/iam/users", iamProxyHandler.CreateUser)
		authRequired.GET("/iam/organizations", iamProxyHandler.ListOrganizations)
		authRequired.POST("/iam/organizations/sync", middleware.RequireSysPerm(middleware.SysPermOrganizationCreate), iamProxyHandler.SyncOrganizations)
		authRequired.POST("/iam/users/sync", middleware.RequireSysPerm(middleware.SysPermUserCreate), iamProxyHandler.SyncUsers)
		authRequired.PUT("/iam/users/:id", iamProxyHandler.UpdateIAMUser)

		// Confirmation Session routes
		confirmationHandler := handlers.NewConfirmationSessionHandler()
		authRequired.POST("/confirmation-sessions", confirmationHandler.Create)
		authRequired.GET("/confirmation-sessions/:id", confirmationHandler.Get)
		authRequired.POST("/confirmation-sessions/:id/confirm", confirmationHandler.Confirm)
		authRequired.POST("/confirmation-sessions/:id/reject", confirmationHandler.Reject)

		// SMS callback (no auth required)
		api.GET("/confirmation/callback/sms", confirmationHandler.SMSCallback)

		// IAM confirmation callback (no auth required, IAM redirects here)
		api.GET("/iam/confirmation-callback", confirmationHandler.IAMConfirmationCallback)

		// Category reads — any authenticated user
		authRequired.GET("/categories", handlers.GetCategories)
		authRequired.GET("/categories/:id", handlers.GetCategoryByID)
		authRequired.GET("/categories/:id/children", handlers.GetCategoryChildren)

		// Category writes — namespace_admin only (category:manage cus_perm)
		categoryAdmin := authRequired.Group("")
		categoryAdmin.Use(middleware.RequireCusPerm("category:manage"))
		categoryAdmin.POST("/categories", handlers.CreateCategory)
		categoryAdmin.PUT("/categories/:id", handlers.UpdateCategory)
		categoryAdmin.DELETE("/categories/:id", handlers.DeleteCategory)
		categoryAdmin.PUT("/categories/sort", handlers.UpdateCategorySort)

		// Banner writes — namespace_admin only (banner:manage cus_perm)
		bannerAdmin := authRequired.Group("")
		bannerAdmin.Use(middleware.RequireCusPerm("banner:manage"))
		bannerAdmin.GET("/admin/banners", bannerHandler.ListBanners)
		bannerAdmin.POST("/admin/banners", bannerHandler.CreateBanner)
		bannerAdmin.PUT("/admin/banners/:id", bannerHandler.UpdateBanner)
		bannerAdmin.DELETE("/admin/banners/:id", bannerHandler.DeleteBanner)

		authRequired.GET("/instruments", middleware.RequireCusPerm("instrument:read"), handlers.GetInstruments)
		authRequired.GET("/instruments/levels", handlers.GetInstrumentLevels)
		authRequired.GET("/instruments/filter-options", handlers.GetInstrumentFilterOptions)
		authRequired.GET("/instruments/check", handlers.CheckInstrumentSN)
		authRequired.GET("/instruments/:id", middleware.RequireCusPerm("instrument:read"), handlers.GetInstrumentByID)
		authRequired.PUT("/instruments/:id", middleware.RequireCusPerm("instrument:update"), handlers.UpdateInstrument)
		authRequired.GET("/reports/assessment/:order_id", handlers.HandleAssessmentReport(database.GetDB()))

		// Instrument CRUD
		authRequired.POST("/instruments", middleware.RequireCusPerm("instrument:create"), handlers.CreateInstrument)
		authRequired.DELETE("/instruments/:id", middleware.RequireCusPerm("instrument:delete"), handlers.DeleteInstrument)
		authRequired.PUT("/instruments/:id/status", middleware.RequireCusPerm("instrument:update"), handlers.UpdateInstrumentStatus)
		authRequired.POST("/instruments/:id/photos/upload", handlers.UploadInstrumentPhotos)
		authRequired.GET("/instruments/:id/photos/latest", handlers.GetLatestInstrumentPhotos)
		authRequired.POST("/instruments/:id/media", middleware.RequireCusPerm("instrument:media_upload"), handlers.CreateInstrumentMedia)
		authRequired.PUT("/instruments/:id/media/display", middleware.RequireCusPerm("instrument:media_display"), handlers.SetMediaDisplay)
		authRequired.DELETE("/instruments/:id/media/:batch_id", middleware.RequireCusPerm("instrument:media_delete"), handlers.DeleteMediaBatch)
		authRequired.GET("/instruments/:id/media", handlers.GetInstrumentMedia)
		authRequired.POST("/instruments/:id/display-image", middleware.RequireCusPerm("instrument:media_upload"), handlers.UploadDisplayImage)
		authRequired.GET("/instruments/:id/activity-log", handlers.GetInstrumentActivityLog)
		authRequired.GET("/instruments/:id/pricing", handlers.GetInstrumentPricing)
		authRequired.POST("/instruments/import", handlers.ImportInstruments)
		authRequired.GET("/instruments/export", handlers.ExportInstruments)
		authRequired.GET("/instruments/import/template", handlers.DownloadCSVTemplate)
		authRequired.POST("/instruments/batch-import", handlers.ExecuteBatchImport)
		authRequired.POST("/instruments/batch-import/preview", handlers.PreviewBatchImport)
		authRequired.POST("/instruments/batch-import/media", handlers.UploadBatchMedia)
		authRequired.GET("/overdue-leases", handlers.GetOverdueLeases)
		authRequired.GET("/orders/by-instrument-sn", middleware.RequireCusPerm("order:read"), handlers.GetOrderByInstrumentSN)
		authRequired.POST("/orders/:id/pickup", middleware.RequireCusPerm("order:update"), handlers.PickupOrder)
		authRequired.POST("/orders/:id/cancel", middleware.RequireCusPerm("order:cancel"), handlers.CancelOrder)

		// Forwarding session routes
		authRequired.GET("/forwarding/sessions", handlers.ListForwardingSessions)
		authRequired.PUT("/forwarding/sessions/:id/ship", handlers.ShipForwardingSession)
		authRequired.PUT("/forwarding/sessions/:id/receive", handlers.ReceiveForwardingSession)
		authRequired.PUT("/forwarding/sessions/:id/ready", handlers.ReadyForwardingSession)
		authRequired.PUT("/forwarding/sessions/:id/last-mile", handlers.LastMileForwardingSession)
		authRequired.PUT("/forwarding/sessions/:id/complete", handlers.CompleteForwardingSession)
		authRequired.PUT("/forwarding/sessions/:id/lost", handlers.LostForwardingSession)
		authRequired.PUT("/forwarding/sessions/:id/recover", handlers.RecoverForwardingSession)

		// Scrap instrument
		authRequired.POST("/instruments/:id/scrap", middleware.RequireCusPerm("instrument:update"), handlers.ScrapInstrument)

		// Merchant management routes (require tenant sys_perm + project_admin role)
		authRequired.GET("/merchants", middleware.RequireSysPerm(middleware.SysPermTenantList), merchantHandler.ListMerchants)
		authRequired.GET("/merchants/:id", middleware.RequireSysPerm(middleware.SysPermTenantView), merchantHandler.GetMerchant)
		authRequired.POST("/merchants", middleware.RequireSysPerm(middleware.SysPermTenantCreate), merchantHandler.CreateMerchant)
		authRequired.PUT("/merchants/:id", middleware.RequireSysPerm(middleware.SysPermTenantUpdate), merchantHandler.UpdateMerchant)
		authRequired.DELETE("/merchants/:id", middleware.RequireSysPerm(middleware.SysPermTenantDelete), merchantHandler.DeleteMerchant)

		siteRequired := authRequired.Group("")
		{
			siteRequired.GET("/common/sites", siteHandler.ListSites)
			siteRequired.GET("/common/sites/nearby", siteHandler.GetNearbySites)
			siteRequired.GET("/common/sites/:id", siteHandler.GetSiteDetail)
		siteRequired.POST("/merchant/sites", middleware.RequireSysPerm(middleware.SysPermOrganizationCreate), siteHandler.CreateSite)
		siteRequired.PUT("/merchant/sites/:id", middleware.RequireSysPerm(middleware.SysPermOrganizationUpdate), siteHandler.UpdateSite)
		siteRequired.DELETE("/merchant/sites/:id", middleware.RequireSysPerm(middleware.SysPermOrganizationDelete), siteHandler.DeleteSite)
			siteRequired.GET("/sites/tree", siteHandler.GetSiteTree)
			siteRequired.GET("/sites/:id/members", siteMemberHandler.ListMembers)
			siteRequired.POST("/sites/:id/members", siteMemberHandler.AddMember)
			siteRequired.PUT("/sites/:id/members/:uid", siteMemberHandler.UpdateMemberRole)
			siteRequired.DELETE("/sites/:id/members/:uid", siteMemberHandler.RemoveMember)

			// Staff/User management routes (Issue #333)
			authRequired.GET("/staff", middleware.RequireRole("ADMIN", "OWNER"), staffHandler.ListStaff)
			authRequired.PUT("/users/me", staffHandler.UpdateCurrentUser)
			authRequired.POST("/users/me/resend-email-confirmation", staffHandler.ResendEmailConfirmation)
			authRequired.POST("/user/reset-password", handlers.ResetPasswordSelf)
			authRequired.POST("/user/change-password", handlers.ChangePasswordSelf)
			authRequired.POST("/users", middleware.RequireSysPerm(middleware.SysPermUserCreate), staffHandler.CreateUser)
			authRequired.PUT("/users/:id", middleware.RequireSysPerm(middleware.SysPermUserUpdate), staffHandler.UpdateUser)
			authRequired.DELETE("/users/batch", middleware.RequireSysPerm(middleware.SysPermUserDelete), staffHandler.BatchDeleteUsers)
			authRequired.POST("/users/reset-password", middleware.RequireSysPerm(middleware.SysPermUserUpdate), staffHandler.ResetPassword)
			authRequired.GET("/users/check", staffHandler.CheckUserExists)
			authRequired.POST("/users/:id/activate", staffHandler.ActivateUser)

			// Notification routes
			authRequired.GET("/notifications", handlers.GetNotifications)
			authRequired.GET("/notifications/unread-count", handlers.GetUnreadCount)
			authRequired.POST("/notifications/mark-all-read", handlers.MarkAllNotificationsRead)
			authRequired.GET("/notifications/:id", handlers.GetNotificationDetail)
			authRequired.POST("/notifications/:id/read", handlers.MarkNotificationRead)
			authRequired.GET("/instrument-photo-specs/:category_id", handlers.GetInstrumentPhotoSpecs)
		}

		propertyRequired := authRequired.Group("")
		{
			propertyRequired.GET("/properties", propertyHandler.ListProperties)
			propertyRequired.GET("/properties/:id/options/search", propertyHandler.SearchPropertyOptions)

			propertyRequiredWithAdmin := propertyRequired.Group("")
			propertyRequiredWithAdmin.Use(middleware.RequireCusPerm("attribute:manage"))
			propertyRequiredWithAdmin.POST("/property", propertyHandler.CreateProperty)
			propertyRequiredWithAdmin.PUT("/property/:id", propertyHandler.UpdateProperty)
			propertyRequiredWithAdmin.POST("/property/option", propertyHandler.CreatePropertyOption)
			propertyRequiredWithAdmin.PUT("/property/confirm", propertyHandler.ConfirmPropertyValue)
			propertyRequiredWithAdmin.PUT("/property/merge", propertyHandler.MergePropertyValues)
		}

		inventoryRequired := authRequired.Group("")
		{
			inventoryRequired.GET("/merchant/inventory", inventoryHandler.ListInventory)
			inventoryRequired.POST("/merchant/inventory/transfer", inventoryHandler.TransferInventory)
			inventoryRequired.GET("/merchant/inventory/transfers", inventoryHandler.ListTransfers)
			inventoryRequired.GET("/inventory/rent-setting", inventoryHandler.GetRentSetting)
			inventoryRequired.PUT("/inventory/rent-setting/batch", inventoryHandler.BatchUpdateRent)
		}

		// Pricing system routes (Issue #689)
		authRequired.GET("/pricing/templates", handlers.ListPricingTemplates)
		authRequired.GET("/pricing/merchant-config", handlers.GetMerchantPricingConfig)
		authRequired.PUT("/pricing/merchant-config", middleware.RequireRole("OWNER"), middleware.RequireCusPerm("instrument:price_config"), handlers.UpdateMerchantPricingConfig)
		authRequired.GET("/instruments/:id/pricing-v2", handlers.GetInstrumentPricingV2)
		authRequired.PUT("/instruments/batch-pricing", middleware.RequireRole("ADMIN", "OWNER"), handlers.BatchSetInstrumentPricing)

		userRequired := authRequired.Group("")
		{
			userRequired.GET("/user/ownership/:id", handlers.GetOwnershipInfo)
			userRequired.GET("/user/ownership/:id/download", handlers.DownloadOwnershipCertificate)
			userRequired.POST("/orders/:id/transfer-ownership", handlers.TriggerOwnershipTransfer)
			userRequired.PUT("/orders/:id/terminate", handlers.TerminateOrder)
		}

		maintHandler := handlers.NewMaintenanceHandler()
		maintRequired := authRequired.Group("")
		maintRequired.Use(middleware.RequireCusPerm("instrument:maintain"))
		{
			maintRequired.POST("/maintenance", maintHandler.SubmitRepair)
			maintRequired.POST("/maintenance/report", maintHandler.ReportRepair)
			maintRequired.GET("/maintenance/:id", maintHandler.GetMaintenanceDetail)
			maintRequired.PUT("/maintenance/:id/cancel", maintHandler.CancelMaintenance)
			maintRequired.PUT("/maintenance/tickets/:id/status", maintHandler.UpdateTicketStatus)
		}

		merchantMaint := authRequired.Group("")
		merchantMaint.Use(middleware.RequireCusPerm("instrument:maintain"))
		{
			merchantMaint.GET("/merchant/maintenance", maintHandler.ListMerchantMaintenance)
			merchantMaint.PUT("/merchant/maintenance/:id/accept", maintHandler.AcceptMaintenance)
			merchantMaint.PUT("/merchant/maintenance/:id/assign", maintHandler.AssignTechnician)
			merchantMaint.PUT("/merchant/maintenance/:id/update", maintHandler.UpdateProgress)

			// Outbound confirmation routes for mini-program (must be before /orders/:id)
			outboundHandler := handlers.NewOutboundHandler(database.GetDB())
			authRequired.GET("/orders/:id/outbound-photos", outboundHandler.GetOutboundPhotos)
			authRequired.POST("/orders/:id/outbound-confirm", outboundHandler.ConfirmOutbound)

			// Assessment routes for damage comparison (must be before /orders/:id)
			assessmentHandler := handlers.NewAssessmentHandler(database.GetDB())
			authRequired.GET("/orders/:id/assessment", assessmentHandler.GetAssessmentData)
			authRequired.POST("/orders/:id/assessment", assessmentHandler.SubmitAssessment)

			// Label management routes for tag normalization
			labelHandler := handlers.NewLabelHandler(database.GetDB())
			authRequired.GET("/labels", labelHandler.GetLabels)
			authRequired.POST("/labels", labelHandler.CreateLabel)
			authRequired.PUT("/labels/:id/approve", labelHandler.ApproveLabel)
			authRequired.PUT("/labels/:id/reject", labelHandler.RejectLabel)
			authRequired.POST("/labels/merge", labelHandler.MergeLabels)
			merchantMaint.POST("/merchant/maintenance/:id/quote", maintHandler.SendQuote)

			techMaint := authRequired.Group("")
			{
				techMaint.GET("/technician/tickets", maintHandler.ListTechnicianTickets)
				techMaint.PUT("/technician/tickets/:id/accept", maintHandler.AcceptTicket)
				techMaint.POST("/technician/tickets/:id/complete", maintHandler.CompleteTicket)
			}

		// Issue #303: Maintenance Worker Management Routes
			maintRequired.GET("/maintenance/workers", maintenanceWorkerHandler.ListWorkers)
			maintRequired.POST("/maintenance/workers", maintenanceWorkerHandler.CreateWorker)
			maintRequired.GET("/maintenance/workers/:id", maintenanceWorkerHandler.GetWorker)
			maintRequired.DELETE("/maintenance/workers/:id", maintenanceWorkerHandler.DeleteWorker)

			// Issue #304: Maintenance Session Routes
			maintRequired.GET("/maintenance/sessions", maintenanceSessionHandler.ListSessions)
			maintRequired.GET("/maintenance/sessions/:id", maintenanceSessionHandler.GetSession)
			maintRequired.POST("/maintenance/:id/start", maintenanceSessionHandler.StartWork)
			maintRequired.PUT("/maintenance/:id/status", maintenanceSessionHandler.UpdateStatus)
			maintRequired.POST("/maintenance/:id/record", maintenanceSessionHandler.SubmitRecord)
			maintRequired.POST("/maintenance/:id/inspect", maintenanceSessionHandler.Inspect)

			// Issue #305: Appeal Processing Routes
			authRequired.GET("/appeals", appealHandler.ListAppeals)
			authRequired.GET("/appeals/:id", appealHandler.GetAppeal)
			authRequired.PUT("/appeals/:id/resolve", appealHandler.ResolveAppeal)
			authRequired.POST("/appeals", appealHandler.SubmitAppeal)
			authRequired.POST("/appeals/:id/agree", appealHandler.AgreeDamage)
			authRequired.GET("/user/appeals", appealHandler.ListAppeals)

			// Issue #306: Warehouse Routes
			authRequired.GET("/warehouse/orders", warehouseHandler.ListOrders)
			authRequired.PUT("/warehouse/orders/:id/shipping", warehouseHandler.UpdateShipping)
			authRequired.PUT("/warehouse/orders/:id/return-inspect", warehouseHandler.InspectReturn)
			authRequired.PUT("/warehouse/orders/:id/damage", warehouseHandler.AssessDamage)

			// Issue #307: User Rental Routes
			// /user/instruments require tenant context (org binding required)
			authRequired.GET("/user/instruments", userRentalHandler.ListInstruments)
			authRequired.GET("/user/instruments/:id", userRentalHandler.GetInstrument)

			// User-friendly routes: auth required but org binding optional (guest support)
			userOptionalAuth := api.Group("")
			userOptionalAuth.Use(middleware.OptionalIAMInterceptor(iamService, iamClient))
			userOptionalAuth.Use(middleware.NoCache())
			userOptionalAuth.Use(middleware.AuditLogger(auditWriter))
			{
				userOptionalAuth.POST("/user/orders", userRentalHandler.CreateOrder)
				userOptionalAuth.POST("/user/orders/batch", userRentalHandler.BatchCreateOrder)
				userOptionalAuth.GET("/user/rentals", userRentalHandler.ListRentals)
				userOptionalAuth.POST("/user/rentals/:id/return", userRentalHandler.ReturnRental)
				userOptionalAuth.GET("/user/contracts", userRentalHandler.ListContracts)
				userOptionalAuth.GET("/user/contracts/:id", userRentalHandler.GetContract)
				userOptionalAuth.GET("/orders", middleware.RequireCusPerm("order:read"), handlers.GetOrders)
			userOptionalAuth.GET("/orders/:id", middleware.RequireCusPerm("order:read"), handlers.GetOrder)
				userOptionalAuth.POST("/orders/:id/return", middleware.RequireCusPerm("order:update"), handlers.ReturnOrder)
				userOptionalAuth.POST("/orders/:id/pay", handlers.PayOrder)
				userOptionalAuth.GET("/users/me", staffHandler.GetCurrentUser)
				userOptionalAuth.POST("/upload", handlers.HandleUpload)
				userOptionalAuth.PUT("/warehouse/orders/:id/delivery", warehouseHandler.ConfirmDelivery)
				userOptionalAuth.GET("/user/addresses", userAddressHandler.ListAddresses)
				userOptionalAuth.POST("/user/addresses", userAddressHandler.CreateAddress)
				userOptionalAuth.PUT("/user/addresses/:id", userAddressHandler.UpdateAddress)
				userOptionalAuth.PUT("/user/addresses/:id/default", userAddressHandler.SetDefaultAddress)
				userOptionalAuth.DELETE("/user/addresses/:id", userAddressHandler.DeleteAddress)
			}

			// Permission Management (merchant admin only, sys_perm bit 26)
			permRequired := authRequired.Group("")
			permRequired.Use(middleware.RequireSysPerm(middleware.SysPermPermissionCreate))
			{
				permRequired.GET("/admin/users", permManageHandler.ListUsers)
				permRequired.PUT("/admin/users/:id/permissions", permManageHandler.SetUserPermissions)
				permRequired.PUT("/admin/users/:id/roles", permManageHandler.SetUserRole)
				permRequired.GET("/admin/roles", roleManageHandler.ListRoles)
				permRequired.POST("/admin/roles", roleManageHandler.CreateRole)
				permRequired.PUT("/admin/roles/:id", roleManageHandler.UpdateRole)
				permRequired.DELETE("/admin/roles/:id", roleManageHandler.DeleteRole)
			}

			systemHandler := handlers.NewSystemHandler()
			authRequired.GET("/system/clients", middleware.RequireSysPerm(middleware.SysPermNamespaceView), systemHandler.GetClients)
			authRequired.GET("/system/tenants", middleware.RequireSysPerm(middleware.SysPermTenantList), systemHandler.GetTenants)
			authRequired.GET("/settings/:key", handlers.GetSetting)
			authRequired.PUT("/settings/:key", middleware.RequireSysPerm(middleware.SysPermTenantUpdate), handlers.UpsertSetting)

			dashboardHandler := handlers.NewDashboardHandler(database.GetDB())
			authRequired.GET("/admin/dashboard/stats", dashboardHandler.GetDashboardStats)
			authRequired.GET("/admin/dashboard/near-transfers", dashboardHandler.GetNearTransfers)

			// Audit Log routes (Issue #598)
			authRequired.GET("/admin/audit-logs", handlers.ListAuditLogs)
			authRequired.GET("/admin/audit-logs/:id", handlers.GetAuditLog)
			authRequired.POST("/admin/audit-logs/export", handlers.ExportAuditLogs)

			// Issue #423: Bulk Import Routes
			authRequired.POST("/admin/bulk-import/organizations", middleware.RequireSysPerm(middleware.SysPermOrganizationCreate), bulkImportHandler.ImportOrganizations)
			authRequired.POST("/admin/bulk-import/accounts", middleware.RequireSysPerm(middleware.SysPermUserCreate), bulkImportHandler.ImportAccounts)
			authRequired.GET("/admin/bulk-import/template/organizations", middleware.RequireSysPerm(middleware.SysPermOrganizationCreate), bulkImportHandler.DownloadOrganizationTemplate)
			authRequired.GET("/admin/bulk-import/template/accounts", middleware.RequireSysPerm(middleware.SysPermUserCreate), bulkImportHandler.DownloadAccountTemplate)

			leaseHandler := handlers.NewLeaseHandler(database.GetDB())
			authRequired.GET("/merchant/leases", leaseHandler.ListLeases)
			authRequired.GET("/merchant/leases/:id", leaseHandler.GetLease)
			authRequired.POST("/merchant/leases", leaseHandler.CreateLease)
			authRequired.PUT("/merchant/leases/:id", leaseHandler.UpdateLease)
			authRequired.DELETE("/merchant/leases/:id", leaseHandler.TerminateLease)

			depositHandler := handlers.NewDepositHandler(database.GetDB())
			authRequired.GET("/merchant/deposits", depositHandler.ListDeposits)
			authRequired.POST("/merchant/deposits", depositHandler.CreateDeposit)
			authRequired.PUT("/merchant/deposits/:id", depositHandler.UpdateDeposit)
		}
	}
}

func main() {
	// Parse command line flags
	var envFile string
	flag.StringVar(&envFile, "env", "", "Path to .env file to load")
	flag.Parse()

	// Load environment file if specified
	if envFile != "" {
		if err := godotenv.Load(envFile); err != nil {
			log.Printf("[WARN] Failed to load env file from %s: %v", envFile, err)
		} else {
			log.Printf("[INFO] Loaded configuration from: %s", envFile)
		}
	} else {
		// Try to load .env from current directory
		if err := godotenv.Load(); err == nil {
			log.Printf("[INFO] Loaded configuration from default .env file")
		}
	}

	cfg := database.LoadConfig()
	db, err := database.InitDB(cfg)
	if err != nil {
		panic("failed to initialize database: " + err.Error())
	}
	database.SetDB(db)

	if err := database.BootstrapDatabase(db); err != nil {
		fmt.Printf("FATAL: Database bootstrap failed: %v\n", err)
		fmt.Println("Please check your database connection and migration files.")
		os.Exit(1)
	}

	// Load persisted UUID IAM client credentials (from activation on previous restarts)
	services.LoadIAMClientCredentials(db)

	// Init permission registry BEFORE BootstrapIAM — it accesses GlobalPermissionRegistry
	namespaceID := os.Getenv("IAM_NAMESPACE")
	if namespaceID == "" {
		namespaceID = "tuneloop"
	}
	permRegistry := services.NewPermissionRegistry()
	services.GlobalPermissionRegistry = permRegistry
	if err := permRegistry.RegisterAndSync(namespaceID); err != nil {
		fmt.Printf("Warning: Customer permission bootstrap failed: %v\n", err)
		log.Printf("[INFO] Continuing with mock permission registry")
	} else {
		log.Printf("[INFO] Customer permissions registered and cached")
		middleware.PermissionRegistry = permRegistry
	}

	if err := services.BootstrapIAM(db); err != nil {
		fmt.Printf("Warning: IAM bootstrap failed: %v\n", err)
	}

	// Sync sys_perm to IAM role templates for each business role.
	// This is independent of cus_perm template management (beaconiam #293
	// only removes customer-permissions APIs, not sys_perm or role-templates).
	{
		iamClient := services.NewIAMClient()
		nsID, nsErr := iamClient.GetNamespaceID()
		if nsErr == nil && nsID != "" {
			log.Printf("[Bootstrap] Syncing role template sys_perm and cus_perm to IAM...")
			for code, template := range services.AllRoleTemplates {
				// Ensure role template exists in IAM before syncing
				templates, _ := iamClient.ListRoleTemplates(nsID)
				exists := false
				for _, t := range templates {
					if t.Code == code {
						exists = true
						break
					}
				}
				if !exists {
					cusPerm, cusPermExt := services.ComputeCusPermBitmapExt(template.CusPermCodes, permRegistry.GetCusPermBit)
					_, err := iamClient.CreateRoleTemplate(nsID, code, template.Name, cusPerm, cusPermExt)
					if err != nil {
						log.Printf("[Bootstrap] Warning: failed to create role template %s: %v", code, err)
						continue
					}
					log.Printf("[Bootstrap] Created role template %s", code)
				}
				if len(template.SysPermBits) > 0 {
					if err := iamClient.SyncRoleTemplateSysPerm(nsID, code, template.SysPermBits); err != nil {
						log.Printf("[Bootstrap] Warning: failed to sync sys_perm for role %s: %v", code, err)
					} else {
						log.Printf("[Bootstrap] Synced sys_perm for role %s: bits=%v", code, template.SysPermBits)
					}
				}
			if len(template.CusPermCodes) > 0 {
				cusPerm, cusPermExt := services.ComputeCusPermBitmapExt(template.CusPermCodes, permRegistry.GetCusPermBit)
				log.Printf("[Bootstrap] Computing cus_perm for %s: codes=%v → value=%d", code, template.CusPermCodes, cusPerm)
				if err := iamClient.SyncRoleTemplateCusPerm(nsID, code, cusPerm, cusPermExt); err != nil {
					log.Printf("[Bootstrap] Warning: failed to sync cus_perm for role %s: %v", code, err)
				} else {
					log.Printf("[Bootstrap] Synced cus_perm for role %s: value=%d, codes=%v", code, cusPerm, template.CusPermCodes)
					// Sync cus_perm for existing users of this default role
					var roleUsers []models.User
					if err := db.Where("role = ? AND status = ?", code, "active").Find(&roleUsers).Error; err != nil {
						log.Printf("[Bootstrap] Warning: failed to query users for role %s: %v", code, err)
					} else {
						for _, u := range roleUsers {
							nilUUID := "00000000-0000-0000-0000-000000000000"
							if u.OrgID == nilUUID || u.OrgID == "" {
								continue
							}
							if err := iamClient.SetUserCustomerPermissions(u.OrgID, u.IAMSub, cusPerm, cusPermExt); err != nil {
								log.Printf("[Bootstrap] Warning: failed to sync cus_perm for user %s (role %s): %v", u.IAMSub, code, err)
							} else {
								log.Printf("[Bootstrap] Synced cus_perm for user %s (role %s): value=%d", u.IAMSub, code, cusPerm)
							}
						}
					}
				}
			}
			}
		} else {
			log.Printf("[Bootstrap] Skipping sys_perm sync: namespace not resolvable (nsErr=%v)", nsErr)
		}
	}

	// Log current configuration
	log.Printf("[INFO] IAM External URL: %s", os.Getenv("BEACONIAM_EXTERNAL_URL"))
	log.Printf("[INFO] IAM PC Client ID: %s", os.Getenv("IAM_PC_CLIENT_ID"))
	log.Printf("[INFO] IAM WX Client ID: %s", os.Getenv("IAM_WX_CLIENT_ID"))
	log.Printf("[INFO] EXTERNAL_WEB_URL: %s", os.Getenv("EXTERNAL_WEB_URL"))
	log.Printf("[INFO] EXTERNAL_MOBILE_URL: %s", os.Getenv("EXTERNAL_MOBILE_URL"))
	log.Printf("[INFO] Working Directory: %s", func() string { dir, _ := os.Getwd(); return dir }())

	iamService := services.NewIAMService()

	wxPort := getEnv("TUNELOOP_WX_PORT", "5556")
	wwwPort := getEnv("TUNELOOP_WWW_PORT", "5557")

	// Log server ports
	log.Printf("[INFO] Mobile server listening on port: %s", wxPort)
	log.Printf("[INFO] PC server listening on port: %s", wwwPort)

	wwwURL := fmt.Sprintf("http://localhost:%s", wwwPort)
	wxURL := fmt.Sprintf("http://localhost:%s", wxPort)

	pcRouter := gin.Default()
	pcRouter.Use(cors.Default())
	setupAPIRoutes(pcRouter, iamService, permRegistry)

	// Serve uploads (user uploaded files)
	pcRouter.Static("/uploads", getAbsPath("./uploads"))

	pcRouter.NoRoute(func(c *gin.Context) {
		path := c.Request.URL.Path

		if strings.HasPrefix(path, "/api/") {
			c.JSON(404, gin.H{
				"code":    40400,
				"message": "endpoint not found: " + path,
			})
			return
		}

		// Return 404 for non-API routes (frontend served by nginx)
		c.Status(404)
	})

	mobileRouter := gin.Default()
	mobileRouter.Use(cors.Default())

	// Mobile frontend from configurable path
	mobileDistPath := getAbsPath(getWWWPath() + "/mobile")
	mobileRouter.GET("/", func(c *gin.Context) {
		c.File(filepath.Join(mobileDistPath, "index.html"))
	})
	mobileRouter.Static("/assets", filepath.Join(mobileDistPath, "assets"))
	mobileRouter.Static("/instruments", filepath.Join(mobileDistPath, "instruments"))
	setupAPIRoutes(mobileRouter, iamService, permRegistry)
	mobileRouter.Static("/uploads", getAbsPath("./uploads"))

	mobileRouter.NoRoute(func(c *gin.Context) {
		path := c.Request.URL.Path

		if strings.HasPrefix(path, "/api/") {
			c.JSON(404, gin.H{
				"code":    40400,
				"message": "endpoint not found: " + path,
			})
			return
		}

		// Check for missing static files (uploads, assets, instruments)
		if strings.HasPrefix(path, "/uploads/") || strings.HasPrefix(path, "/assets/") || strings.HasPrefix(path, "/instruments/") {
			c.Status(404)
			return
		}

		// Serve index.html for SPA routing
		c.File(filepath.Join(mobileDistPath, "index.html"))
	})

	go mobileRouter.Run(":" + wxPort)

	leaseAccumulator := tasks.NewLeaseAccumulator()
	leaseAccumulator.Start()

	auditLogCleaner := services.NewAuditLogCleaner()
	auditLogCleaner.Start()

	autoConfirmSvc := handlers.NewAutoConfirmService()
	autoConfirmSvc.Start()
	defer autoConfirmSvc.Stop()

	depositRefundScheduler := services.NewDepositRefundScheduler()
	depositRefundScheduler.Start()
	defer depositRefundScheduler.Stop()

	logisticsMonitor := handlers.NewLogisticsMonitor()
	logisticsMonitor.Start()
	defer logisticsMonitor.Stop()

	mediaCleanup := services.NewMediaCleanupService()
	mediaCleanup.Start()
	defer mediaCleanup.Stop()

	_ = wwwURL
	_ = wxURL

	pcRouter.Run(":" + wwwPort)
}
