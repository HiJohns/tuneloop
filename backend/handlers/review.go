package handlers

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
)

// GetInstrumentReviews - GET /api/instruments/:id/reviews
func GetInstrumentReviews(c *gin.Context) {
	instrumentID := c.Param("id")
	if instrumentID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    40002,
			"message": "instrument_id is required",
		})
		return
	}

	// Parse query parameters
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	perPage, _ := strconv.Atoi(c.DefaultQuery("per_page", "10"))
	if page < 1 {
		page = 1
	}
	if perPage < 1 || perPage > 50 {
		perPage = 10
	}

	// Mock reviews data
	reviews := []gin.H{
		{
			"id":         "review_001",
			"user_id":    "user_001",
			"user_name":  "张先生",
			"rating":     5,
			"comment":    "钢琴音色很好，孩子很喜欢，租赁流程也很方便。",
			"images":     []string{"/reviews/review_001_1.jpg"},
			"created_at": time.Now().AddDate(0, 0, -5).Format(time.RFC3339),
			"helpful":    12,
		},
		{
			"id":         "review_002",
			"user_id":    "user_002",
			"user_name":  "李女士",
			"rating":     4,
			"comment":    "整体不错，就是送货时间有点长。客服态度很好。",
			"images":     []string{},
			"created_at": time.Now().AddDate(0, 0, -12).Format(time.RFC3339),
			"helpful":    8,
		},
		{
			"id":         "review_003",
			"user_id":    "user_003",
			"user_name":  "王老师",
			"rating":     5,
			"comment":    "作为音乐老师，我觉得这架钢琴的音质和手感都很不错，推荐给初学者。",
			"images":     []string{"/reviews/review_003_1.jpg", "/reviews/review_003_2.jpg"},
			"created_at": time.Now().AddDate(0, 0, -20).Format(time.RFC3339),
			"helpful":    24,
		},
	}

	// Calculate pagination
	total := len(reviews)
	start := (page - 1) * perPage
	end := start + perPage
	if start > total {
		start = total
	}
	if end > total {
		end = total
	}

	paginatedReviews := reviews[start:end]

	// Calculate summary stats
	var totalRating int
	for _, review := range reviews {
		totalRating += int(review["rating"].(int))
	}
	averageRating := float64(totalRating) / float64(len(reviews))

	c.JSON(http.StatusOK, gin.H{
		"code": 20000,
		"data": gin.H{
			"reviews":      paginatedReviews,
			"pagination": gin.H{
				"page":       page,
				"per_page":   perPage,
				"total":      total,
				"total_pages": (total + perPage - 1) / perPage,
			},
			"summary": gin.H{
				"average_rating": averageRating,
				"total_reviews":  total,
				"five_star":      2,
				"four_star":      1,
				"three_star":     0,
				"two_star":       0,
				"one_star":       0,
			},
		},
	})
}
