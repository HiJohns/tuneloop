package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"tuneloop-backend/database"
	"tuneloop-backend/handlers"
	"tuneloop-backend/internal/tasks"

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

func extractPort(url string) string {
	if strings.HasPrefix(url, "http://") {
		url = strings.TrimPrefix(url, "http://")
		parts := strings.Split(url, ":")
		if len(parts) > 1 {
			return parts[1]
		}
	}
	if strings.HasPrefix(url, "https://") {
		url = strings.TrimPrefix(url, "https://")
		parts := strings.Split(url, ":")
		if len(parts) > 1 {
			return parts[1]
		}
	}
	return "5554"
}

func setupAPIRoutes(r *gin.Engine) {
	siteHandler := handlers.NewSiteHandler()
	inventoryHandler := handlers.NewInventoryHandler()

	api := r.Group("/api")
	{
		api.GET("/instruments", handlers.GetInstruments)
		api.GET("/instruments/:id", handlers.GetInstruments)
		api.GET("/instruments/:id/pricing", handlers.GetInstrumentPricing)
		api.GET("/sites", handlers.GetSites)
		api.POST("/upload", handlers.HandleUpload)
		api.GET("/overdue-leases", handlers.GetOverdueLeases)
		api.POST("/orders/preview", handlers.PreviewOrder)
		api.POST("/orders", handlers.CreateOrder)

		// Site Management
		api.GET("/common/sites", siteHandler.ListSites)
		api.GET("/common/sites/nearby", siteHandler.GetNearbySites)
		api.GET("/common/sites/:id", siteHandler.GetSiteDetail)
		api.POST("/merchant/sites", siteHandler.CreateSite)
		api.PUT("/merchant/sites/:id", siteHandler.UpdateSite)
		api.DELETE("/merchant/sites/:id", siteHandler.DeleteSite)

		// Inventory Management
		api.GET("/merchant/inventory", inventoryHandler.ListInventory)
		api.POST("/merchant/inventory/transfer", inventoryHandler.TransferInventory)
		api.GET("/merchant/inventory/transfers", inventoryHandler.ListTransfers)

		// Ownership Management
		maintHandler := handlers.NewMaintenanceHandler()
		api.GET("/user/ownership/:id", handlers.GetOwnershipInfo)
		api.GET("/user/ownership/:id/download", handlers.DownloadOwnershipCertificate)
		api.POST("/orders/:id/transfer-ownership", handlers.TriggerOwnershipTransfer)
		api.PUT("/orders/:id/terminate", handlers.TerminateOrder)

		// Maintenance APIs
		api.POST("/maintenance", maintHandler.SubmitRepair)
		api.GET("/maintenance/:id", maintHandler.GetMaintenanceDetail)
		api.PUT("/maintenance/:id/cancel", maintHandler.CancelMaintenance)

		// Merchant Maintenance
		api.GET("/merchant/maintenance", maintHandler.ListMerchantMaintenance)
		api.PUT("/merchant/maintenance/:id/accept", maintHandler.AcceptMaintenance)
		api.PUT("/merchant/maintenance/:id/assign", maintHandler.AssignTechnician)
		api.PUT("/merchant/maintenance/:id/update", maintHandler.UpdateProgress)
		api.POST("/merchant/maintenance/:id/quote", maintHandler.SendQuote)
	}
}

func main() {
	// 初始化数据库
	cfg := database.LoadConfig()
	db, err := database.InitDB(cfg)
	if err != nil {
		panic("failed to initialize database: " + err.Error())
	}
	database.SetDB(db)

	// 运行迁移
	if err := database.RunMigrations(db); err != nil {
		fmt.Printf("Warning: migration failed: %v\n", err)
	}

	wwwURL := getEnv("TUNELOOP_WWW_URL", "http://localhost:5554")
	wxURL := getEnv("TUNELOOP_WX_URL", "http://localhost:5553")
	pcPort := extractPort(wwwURL)
	mobilePort := extractPort(wxURL)

	// PC Service (Port 5554)
	pcRouter := gin.Default()
	pcRouter.Use(cors.Default())

	pcDistPath := getAbsPath("../frontend-pc/dist")
	pcRouter.GET("/", func(c *gin.Context) {
		c.File(filepath.Join(pcDistPath, "index.html"))
	})
	pcRouter.Static("/assets", filepath.Join(pcDistPath, "assets"))
	pcRouter.StaticFile("/favicon.ico", filepath.Join(pcDistPath, "favicon.ico"))
	pcRouter.StaticFile("/favicon.svg", filepath.Join(pcDistPath, "favicon.svg"))
	setupAPIRoutes(pcRouter)

	// SPA support: return index.html for non-static routes
	pcRouter.NoRoute(func(c *gin.Context) {
		c.File(filepath.Join(pcDistPath, "index.html"))
	})

	// Mobile Service (Port 5553)
	mobileRouter := gin.Default()
	mobileRouter.Use(cors.Default())

	mobileDistPath := getAbsPath("../frontend-mobile/dist")
	mobileRouter.GET("/", func(c *gin.Context) {
		c.File(filepath.Join(mobileDistPath, "index.html"))
	})
	mobileRouter.Static("/assets", filepath.Join(mobileDistPath, "assets"))
	mobileRouter.Static("/instruments", "../frontend-mobile/public/instruments")
	setupAPIRoutes(mobileRouter)

	// SPA support: return index.html for non-static routes
	mobileRouter.NoRoute(func(c *gin.Context) {
		c.File(filepath.Join(mobileDistPath, "index.html"))
	})

	// Start PC server in a goroutine
	go pcRouter.Run(":" + pcPort)

	// Start lease accumulator task
	leaseAccumulator := tasks.NewLeaseAccumulator()
	leaseAccumulator.Start()

	// Start Mobile server (blocking)
	mobileRouter.Run(":" + mobilePort)
}
