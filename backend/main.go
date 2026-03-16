package main

import (
	"os"
	"path/filepath"
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

func getAbsPath(relativePath string) string {
	execDir, _ := os.Getwd()
	return filepath.Join(execDir, relativePath)
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

	// Start Mobile server (blocking)
	mobileRouter.Run(":" + mobilePort)
}
