package main

import (
	"fmt"
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
	inventoryHandler := handlers.NewInventoryHandler()
	authHandler := handlers.NewAuthHandler(database.GetDB())

	api := r.Group("/api")

	api.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok"})
	})

	api.GET("/config", func(c *gin.Context) {
		iamExternalURL := os.Getenv("BEACONIAM_EXTERNAL_URL")
		if iamExternalURL == "" {
			iamExternalURL = "http://localhost:5552"
		}

		c.JSON(http.StatusOK, gin.H{
			"code": 20000,
			"data": gin.H{
				"iamLoginUrl": iamExternalURL + "/login",
				"appName":     "TuneLoop",
				"version":     "1.0.0",
			},
		})
	})

	api.GET("/auth/callback", authHandler.Callback)
	api.POST("/auth/callback", authHandler.Callback)
	api.POST("/auth/login", authHandler.PostLogin)
	api.POST("/auth/refresh", authHandler.Refresh)

	authRequired := api.Group("")
	authRequired.Use(middleware.IAMInterceptor(iamService))
	authRequired.Use(middleware.NoCache())
	{
		authRequired.GET("/categories", handlers.GetCategories)
		authRequired.POST("/categories", handlers.CreateCategory)
		authRequired.GET("/instruments", handlers.GetInstruments)
		authRequired.GET("/instruments/:id", handlers.GetInstruments)

		// Owner 专属路由 - 使用中间件直接包裹
		authRequired.POST("/instruments", middleware.RequireOwner(), handlers.CreateInstrument)
		authRequired.PUT("/instruments/:id/status", handlers.UpdateInstrumentStatus)
		authRequired.GET("/instruments/:id/pricing", handlers.GetInstrumentPricing)
		authRequired.POST("/instruments/import", handlers.ImportInstruments)
		authRequired.GET("/instruments/export", handlers.ExportInstruments)
		authRequired.GET("/instruments/import/template", handlers.DownloadImportTemplate)
		authRequired.POST("/upload", handlers.HandleUpload)
		authRequired.GET("/overdue-leases", handlers.GetOverdueLeases)
		authRequired.POST("/orders/preview", handlers.PreviewOrder)
		authRequired.POST("/orders", handlers.CreateOrder)

		siteRequired := authRequired.Group("")
		{
			siteRequired.GET("/common/sites", siteHandler.ListSites)
			siteRequired.GET("/common/sites/nearby", siteHandler.GetNearbySites)
			siteRequired.GET("/common/sites/:id", siteHandler.GetSiteDetail)
			siteRequired.POST("/merchant/sites", siteHandler.CreateSite)
			siteRequired.PUT("/merchant/sites/:id", siteHandler.UpdateSite)
			siteRequired.DELETE("/merchant/sites/:id", siteHandler.DeleteSite)
		}

		inventoryRequired := authRequired.Group("")
		{
			inventoryRequired.GET("/merchant/inventory", inventoryHandler.ListInventory)
			inventoryRequired.POST("/merchant/inventory/transfer", inventoryHandler.TransferInventory)
			inventoryRequired.GET("/merchant/inventory/transfers", inventoryHandler.ListTransfers)
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
			merchantMaint.POST("/merchant/maintenance/:id/quote", maintHandler.SendQuote)

			techMaint := authRequired.Group("")
			{
				techMaint.GET("/technician/tickets", maintHandler.ListTechnicianTickets)
				techMaint.PUT("/technician/tickets/:id/accept", maintHandler.AcceptTicket)
				techMaint.POST("/technician/tickets/:id/complete", maintHandler.CompleteTicket)
			}

			permHandler := handlers.NewPermissionHandler(database.GetDB())

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

	iamService := services.NewIAMService()

	wxPort := getEnv("TUNELOOP_WX_PORT", "5556")
	wwwPort := getEnv("TUNELOOP_WWW_PORT", "5557")

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
	setupAPIRoutes(pcRouter, iamService)

	pcRouter.NoRoute(func(c *gin.Context) {
		if strings.HasPrefix(c.Request.URL.Path, "/api/") {
			c.JSON(404, gin.H{
				"code":    40400,
				"message": "endpoint not found: " + c.Request.URL.Path,
			})
			return
		}
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

	mobileRouter.NoRoute(func(c *gin.Context) {
		if strings.HasPrefix(c.Request.URL.Path, "/api/") {
			c.JSON(404, gin.H{
				"code":    40400,
				"message": "endpoint not found: " + c.Request.URL.Path,
			})
			return
		}
		c.File(filepath.Join(mobileDistPath, "index.html"))
	})

	go mobileRouter.Run(":" + wxPort)

	leaseAccumulator := tasks.NewLeaseAccumulator()
	leaseAccumulator.Start()

	_ = wwwURL
	_ = wxURL

	pcRouter.Run(":" + wwwPort)
}
