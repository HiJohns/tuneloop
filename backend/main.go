package main

import (
	"tuneloop-backend/handlers"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
)

func main() {
	r := gin.Default()
	r.Use(cors.Default())

	r.GET("/", func(c *gin.Context) {
		c.File("../frontend-pc/dist/index.html")
	})
	r.Static("/assets", "../frontend-pc/dist/assets")

	r.GET("/wx", func(c *gin.Context) {
		c.File("../frontend-mobile/dist/index.html")
	})
	r.Static("/wx/assets", "../frontend-mobile/dist/assets")

	r.NoRoute(func(c *gin.Context) {
		c.File("../frontend-pc/dist/index.html")
	})

	api := r.Group("/api/v1")
	{
		api.GET("/instruments", handlers.GetInstruments)
		api.GET("/sites", handlers.GetSites)
		api.POST("/upload", handlers.HandleUpload)
	}

	r.Run(":5554")
}
