package main

import (
	"os"
	"path/filepath"
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
	pcPort := getEnv("PC_PORT", "5554")
	mobilePort := getEnv("MOBILE_PORT", "5553")

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
