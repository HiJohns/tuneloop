package handlers

import (
	"github.com/gin-gonic/gin"
	"net/http"
)

func GetInstruments(c *gin.Context) {
	c.File("data/instruments.json")
}

func GetSites(c *gin.Context) {
	c.File("data/sites.json")
}

func HandleUpload(c *gin.Context) {
	file, err := c.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "No file uploaded",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success":  true,
		"fileName": file.Filename,
		"url":      "https://dummy.tuneloop.com/uploads/mock-image.jpg",
		"size":     file.Size,
	})
}
