package main

import (
	"net/http"
	"path/filepath"
	"tuneloop-backend/handlers"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
)

func main() {
	r := gin.Default()
	r.Use(cors.Default())

	api := r.Group("/api/v1")
	{
		api.GET("/instruments", handlers.GetInstruments)
		api.GET("/sites", handlers.GetSites)
		api.POST("/upload", handlers.HandleUpload)
	}

	r.Static("/", "../frontend-mobile/dist")

	r.NoRoute(func(c *gin.Context) {
		c.File("../frontend-mobile/dist/index.html")
	})

	r.Run(":5554")
}
