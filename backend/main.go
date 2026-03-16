package main

import (
	"os"
	"tuneloop-backend/handlers"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
)

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func setupAPIRoutes(r *gin.Engine) {
	api := r.Group("/api/v1")
	{
		api.GET("/instruments", handlers.GetInstruments)
		api.GET("/sites", handlers.GetSites)
		api.POST("/upload", handlers.HandleUpload)
		api.GET("/overdue-leases", handlers.GetOverdueLeases)
	}
}

func main() {
	pcPort := getEnv("PC_PORT", "5554")
	mobilePort := getEnv("MOBILE_PORT", "5553")

	// PC Service (Port 5554)
	pcRouter := gin.Default()
	pcRouter.Use(cors.Default())

	pcRouter.GET("/", func(c *gin.Context) {
		c.File("../frontend-pc/dist/index.html")
	})
	pcRouter.Static("/assets", "../frontend-pc/dist/assets")
	pcRouter.StaticFile("/favicon.ico", "../frontend-pc/dist/favicon.ico")
	pcRouter.StaticFile("/favicon.svg", "../frontend-pc/dist/favicon.svg")
	setupAPIRoutes(pcRouter)

	// SPA support: return index.html for non-static routes
	pcRouter.NoRoute(func(c *gin.Context) {
		c.File("../frontend-pc/dist/index.html")
	})

	// Mobile Service (Port 5553)
	mobileRouter := gin.Default()
	mobileRouter.Use(cors.Default())

	mobileRouter.GET("/", func(c *gin.Context) {
		c.File("../frontend-mobile/dist/index.html")
	})
	mobileRouter.Static("/assets", "../frontend-mobile/dist/assets")
	mobileRouter.Static("/instruments", "../frontend-mobile/public/instruments")
	setupAPIRoutes(mobileRouter)

	// SPA support: return index.html for non-static routes
	mobileRouter.NoRoute(func(c *gin.Context) {
		c.File("../frontend-mobile/dist/index.html")
	})

	// Start PC server in a goroutine
	go pcRouter.Run(":" + pcPort)

	// Start Mobile server (blocking)
	mobileRouter.Run(":" + mobilePort)
}
