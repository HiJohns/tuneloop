package main

import (
	"tuneloop-backend/handlers"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
)

func main() {
	r := gin.Default()
	r.Use(cors.Default())

	r.NoRoute(func(c *gin.Context) {
		c.File("../frontend-mobile/dist/index.html")
	})

	r.Static("/assets", "../frontend-mobile/dist/assets")

	api := r.Group("/api/v1")
	{
		api.GET("/instruments", handlers.GetInstruments)
		api.GET("/sites", handlers.GetSites)
		api.POST("/upload", handlers.HandleUpload)
	}

	r.GET("/", func(c *gin.Context) {
		c.File("../frontend-mobile/dist/index.html")
	})

	r.Run(":5554")
}
