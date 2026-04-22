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

func setupAPIRoutes(r *gin.Engine, iamService *services.IAMService) {
	siteHandler := handlers.NewSiteHandler()
	propertyHandler := handlers.NewPropertyHandler()
	inventoryHandler := handlers.NewInventoryHandler()
	authHandler := handlers.NewAuthHandler(database.GetDB())

	// New handlers for Issue #299 (Maintenance, Appeal, Warehouse, User Rental)
	maintenanceWorkerHandler := handlers.NewMaintenanceWorkerHandler()
	maintenanceSessionHandler := handlers.NewMaintenanceSessionHandler()
	appealHandler := handlers.NewAppealHandler()
	warehouseHandler := handlers.NewWarehouseHandler()
	userRentalHandler := handlers.NewUserRentalHandler()

	api := r.Group("/api")

	api.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok"})
	})

	api.GET("/config", func(c *gin.Context) {
		iamExternalURL := os.Getenv("BEACONIAM_EXTERNAL_URL")
		if iamExternalURL == "" {
			iamExternalURL = "http://localhost:5552"
		}

		// PC Web configuration
		iamPCClientID := os.Getenv("IAM_PC_CLIENT_ID")
		if iamPCClientID == "" {
			iamPCClientID = "tuneloop-pc"
		}
		iamPCRedirectURI := os.Getenv("IAM_PC_REDIRECT_URI")
		if iamPCRedirectURI == "" {
			iamPCRedirectURI = "http://localhost:5554/callback"
		}

		// WeChat Mini Program configuration
		iamWXClientID := os.Getenv("IAM_WX_CLIENT_ID")
		if iamWXClientID == "" {
			iamWXClientID = "tuneloop-wx"
		}
		iamWXRedirectURI := os.Getenv("IAM_WX_REDIRECT_URI")
		if iamWXRedirectURI == "" {
			iamWXRedirectURI = "http://localhost:5556/callback"
		}

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

	api.GET("/auth/callback", authHandler.Callback)
	api.POST("/auth/callback", authHandler.Callback)
	api.GET("/auth/oidc/authorization-url", authHandler.GetOIDCAuthorizationURL)
	api.POST("/auth/login", authHandler.PostLogin)
	api.POST("/auth/refresh", authHandler.Refresh)

	authRequired := api.Group("")
	authRequired.Use(middleware.IAMInterceptor(iamService))
	authRequired.Use(middleware.NoCache())
	{
		// IAM Proxy routes
		iamProxyHandler := handlers.NewIAMProxyHandler()
		authRequired.GET("/iam/users/lookup", iamProxyHandler.LookupUser)
		authRequired.POST("/iam/users", iamProxyHandler.CreateUser)

		authRequired.GET("/categories", handlers.GetCategories)
		authRequired.POST("/categories", handlers.CreateCategory)
		authRequired.GET("/categories/:id", handlers.GetCategoryByID)
		authRequired.PUT("/categories/:id", handlers.UpdateCategory)
		authRequired.DELETE("/categories/:id", handlers.DeleteCategory)
		authRequired.GET("/categories/:id/children", handlers.GetCategoryChildren)
		authRequired.PUT("/categories/sort", handlers.UpdateCategorySort)
		authRequired.GET("/instruments", handlers.GetInstruments)
		authRequired.GET("/instruments/levels", handlers.GetInstrumentLevels)
		authRequired.GET("/instruments/check", handlers.CheckInstrumentSN)
		authRequired.GET("/instruments/:id", handlers.GetInstrumentByID)
		authRequired.PUT("/instruments/:id", handlers.UpdateInstrument)
		authRequired.GET("/reports/assessment/:order_id", handlers.HandleAssessmentReport(database.GetDB()))

		// Owner 专属路由 - 使用中间件直接包裹
		authRequired.POST("/instruments", middleware.RequireOwner(), handlers.CreateInstrument)
		authRequired.PUT("/instruments/:id/status", handlers.UpdateInstrumentStatus)
		authRequired.GET("/instruments/:id/pricing", handlers.GetInstrumentPricing)
		authRequired.POST("/instruments/import", handlers.ImportInstruments)
		authRequired.GET("/instruments/export", handlers.ExportInstruments)
		authRequired.GET("/instruments/import/template", handlers.DownloadImportTemplate)
		authRequired.POST("/instruments/batch-import", handlers.BatchImportInstruments)
		authRequired.POST("/instruments/batch-import/preview", handlers.PreviewBatchImport)
		authRequired.POST("/upload", handlers.HandleUpload)
		authRequired.GET("/overdue-leases", handlers.GetOverdueLeases)
		authRequired.POST("/orders/preview", handlers.PreviewOrder)
		authRequired.POST("/orders", handlers.CreateOrder)
		authRequired.GET("/orders", handlers.GetOrders)
		authRequired.GET("/orders/:id", handlers.GetOrder)
		authRequired.POST("/orders/:id/pay", handlers.PayOrder)
		authRequired.POST("/orders/:id/pickup", handlers.PickupOrder)
		authRequired.POST("/orders/:id/return", handlers.ReturnOrder)
		authRequired.POST("/orders/:id/cancel", handlers.CancelOrder)

		siteRequired := authRequired.Group("")
		{
			siteRequired.GET("/common/sites", siteHandler.ListSites)
			siteRequired.GET("/common/sites/nearby", siteHandler.GetNearbySites)
			siteRequired.GET("/common/sites/:id", siteHandler.GetSiteDetail)
			siteRequired.POST("/merchant/sites", siteHandler.CreateSite)
			siteRequired.PUT("/merchant/sites/:id", siteHandler.UpdateSite)
			siteRequired.DELETE("/merchant/sites/:id", siteHandler.DeleteSite)
			siteRequired.GET("/sites/tree", siteHandler.GetSiteTree)
		}

		propertyRequired := authRequired.Group("")
		{
			propertyRequired.GET("/properties", propertyHandler.ListProperties)
			propertyRequired.POST("/property", propertyHandler.CreateProperty)
			propertyRequired.PUT("/property/:id", propertyHandler.UpdateProperty)
			propertyRequired.POST("/property/option", propertyHandler.CreatePropertyOption)
			propertyRequired.PUT("/property/confirm", propertyHandler.ConfirmPropertyValue)
			propertyRequired.PUT("/property/merge", propertyHandler.MergePropertyValues)
		}

		inventoryRequired := authRequired.Group("")
		{
			inventoryRequired.GET("/merchant/inventory", inventoryHandler.ListInventory)
			inventoryRequired.POST("/merchant/inventory/transfer", inventoryHandler.TransferInventory)
			inventoryRequired.GET("/merchant/inventory/transfers", inventoryHandler.ListTransfers)
			inventoryRequired.GET("/inventory/rent-setting", inventoryHandler.GetRentSetting)
			inventoryRequired.PUT("/inventory/rent-setting/batch", inventoryHandler.BatchUpdateRent)
		}

		userRequired := authRequired.Group("")
		{
			userRequired.GET("/user/ownership/:id", handlers.GetOwnershipInfo)
			userRequired.GET("/user/ownership/:id/download", handlers.DownloadOwnershipCertificate)
			userRequired.POST("/orders/:id/transfer-ownership", handlers.TriggerOwnershipTransfer)
			userRequired.PUT("/orders/:id/terminate", handlers.TerminateOrder)
		}

		maintHandler := handlers.NewMaintenanceHandler()
		authRequired.POST("/maintenance", maintHandler.SubmitRepair)
		authRequired.POST("/maintenance/report", maintHandler.ReportRepair)
		authRequired.GET("/maintenance/:id", maintHandler.GetMaintenanceDetail)
		authRequired.PUT("/maintenance/:id/cancel", maintHandler.CancelMaintenance)
		authRequired.PUT("/maintenance/tickets/:id/status", maintHandler.UpdateTicketStatus)

		merchantMaint := authRequired.Group("")
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

			permHandler := handlers.NewPermissionHandler(database.GetDB())

			// Issue #303: Maintenance Worker Management Routes
			authRequired.GET("/maintenance/workers", maintenanceWorkerHandler.ListWorkers)
			authRequired.POST("/maintenance/workers", maintenanceWorkerHandler.CreateWorker)
			authRequired.GET("/maintenance/workers/:id", maintenanceWorkerHandler.GetWorker)
			authRequired.DELETE("/maintenance/workers/:id", maintenanceWorkerHandler.DeleteWorker)

			// Issue #304: Maintenance Session Routes
			authRequired.GET("/maintenance/sessions", maintenanceSessionHandler.ListSessions)
			authRequired.GET("/maintenance/sessions/:id", maintenanceSessionHandler.GetSession)
			authRequired.POST("/maintenance/:id/start", maintenanceSessionHandler.StartWork)
			authRequired.PUT("/maintenance/:id/status", maintenanceSessionHandler.UpdateStatus)
			authRequired.POST("/maintenance/:id/record", maintenanceSessionHandler.SubmitRecord)
			authRequired.POST("/maintenance/:id/inspect", maintenanceSessionHandler.Inspect)

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
			authRequired.PUT("/warehouse/orders/:id/delivery", warehouseHandler.ConfirmDelivery)
			authRequired.PUT("/warehouse/orders/:id/return-inspect", warehouseHandler.InspectReturn)
			authRequired.PUT("/warehouse/orders/:id/damage", warehouseHandler.AssessDamage)

			// Issue #307: User Rental Routes
			authRequired.GET("/user/instruments", userRentalHandler.ListInstruments)
			authRequired.GET("/user/instruments/:id", userRentalHandler.GetInstrument)
			authRequired.POST("/user/orders", userRentalHandler.CreateOrder)
			authRequired.GET("/user/rentals", userRentalHandler.ListRentals)
			authRequired.POST("/user/rentals/:id/return", userRentalHandler.ReturnRental)
			authRequired.GET("/user/contracts/:id", userRentalHandler.GetContract)

			// Admin/Owner 专属路由组
			adminRequired := authRequired.Group("")
			adminRequired.Use(middleware.RequireRole("ADMIN", "OWNER"))
			{
				adminRequired.GET("/admin/permissions", permHandler.GetPermissions)
				adminRequired.GET("/admin/roles", permHandler.GetRoles)
				adminRequired.GET("/admin/roles/:id/permissions", permHandler.GetRolePermissions)
				adminRequired.PUT("/admin/roles/:id/permissions", permHandler.UpdateRolePermissions)
				adminRequired.POST("/admin/roles", permHandler.CreateRole)
				adminRequired.DELETE("/admin/roles/:id", permHandler.DeleteRole)
			}

			systemHandler := handlers.NewSystemHandler()
			authRequired.GET("/system/clients", systemHandler.GetClients)
			authRequired.GET("/system/tenants", systemHandler.GetTenants)

			dashboardHandler := handlers.NewDashboardHandler(database.GetDB())
			authRequired.GET("/admin/dashboard/stats", dashboardHandler.GetDashboardStats)
			authRequired.GET("/admin/dashboard/near-transfers", dashboardHandler.GetNearTransfers)

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

	if err := services.BootstrapIAM(db); err != nil {
		fmt.Printf("Warning: IAM bootstrap failed: %v\n", err)
	}

	// Log current configuration
	log.Printf("[INFO] IAM External URL: %s", os.Getenv("BEACONIAM_EXTERNAL_URL"))
	log.Printf("[INFO] IAM PC Client ID: %s", os.Getenv("IAM_PC_CLIENT_ID"))
	log.Printf("[INFO] IAM WX Client ID: %s", os.Getenv("IAM_WX_CLIENT_ID"))
	log.Printf("[INFO] IAM PC Redirect URI: %s", os.Getenv("IAM_PC_REDIRECT_URI"))
	log.Printf("[INFO] IAM WX Redirect URI: %s", os.Getenv("IAM_WX_REDIRECT_URI"))
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

	pcDistPath := getAbsPath("../frontend-pc/dist")
	pcRouter.GET("/", func(c *gin.Context) {
		c.File(filepath.Join(pcDistPath, "index.html"))
	})
	pcRouter.Static("/assets", filepath.Join(pcDistPath, "assets"))
	pcRouter.StaticFile("/favicon.ico", filepath.Join(pcDistPath, "favicon.ico"))
	pcRouter.StaticFile("/favicon.svg", filepath.Join(pcDistPath, "favicon.svg"))
	pcRouter.Static("/uploads", getAbsPath("./uploads"))
	setupAPIRoutes(pcRouter, iamService)

	pcRouter.NoRoute(func(c *gin.Context) {
		path := c.Request.URL.Path

		if strings.HasPrefix(path, "/api/") {
			c.JSON(404, gin.H{
				"code":    40400,
				"message": "endpoint not found: " + path,
			})
			return
		}

		// Check for missing static files (uploads, assets)
		if strings.HasPrefix(path, "/uploads/") || strings.HasPrefix(path, "/assets/") {
			c.Status(404)
			return
		}

		// Serve index.html for SPA routing
		c.File(filepath.Join(pcDistPath, "index.html"))
	})

	mobileRouter := gin.Default()
	mobileRouter.Use(cors.Default())

	mobileDistPath := getAbsPath("../frontend-mobile/dist")
	mobileRouter.GET("/", func(c *gin.Context) {
		c.File(filepath.Join(mobileDistPath, "index.html"))
	})
	mobileRouter.Static("/assets", filepath.Join(mobileDistPath, "assets"))
	mobileRouter.Static("/instruments", "../frontend-mobile/public/instruments")
	setupAPIRoutes(mobileRouter, iamService)
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

	_ = wwwURL
	_ = wxURL

	pcRouter.Run(":" + wwwPort)
}
