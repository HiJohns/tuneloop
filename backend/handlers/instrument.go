package handlers

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"tuneloop-backend/middleware"
)

type InstrumentHandler struct {
	db *gorm.DB
}

func NewInstrumentHandler(db *gorm.DB) *InstrumentHandler {
	return &InstrumentHandler{db: db}
}

// GetInstruments - GET /api/instruments
func (h *InstrumentHandler) GetInstruments(c *gin.Context) {
	tenantID := middleware.GetTenantID(c.Request.Context())
	if tenantID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    40002,
			"message": "tenant_id is required",
		})
		return
	}

	// Parse query parameters
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	perPage, _ := strconv.Atoi(c.DefaultQuery("per_page", "20"))
	categoryID := c.Query("category_id")
	brand := c.Query("brand")
	status := c.Query("status") // "active", "inactive", "maintenance"
	sort := c.DefaultQuery("sort", "created_at")
	_ = c.DefaultQuery("order", "desc") // Order parameter (not used in mock)

	if page < 1 {
		page = 1
	}
	if perPage < 1 || perPage > 100 {
		perPage = 20
	}

	// Mock instruments data
	instruments := []gin.H{
		{
			"id":            "instrument_001",
			"name":          "雅马哈钢琴 U1",
			"brand":         "雅马哈",
			"category_id":   "category_001",
			"category_name": "钢琴",
			"model":         "U1",
			"level":         "standard",
			"description":   "专业级立式钢琴，适合初学者到高级演奏者",
			"images":        []string{"/images/piano1.jpg", "/images/piano2.jpg"},
			"video":         "/videos/piano-demo.mp4",
			"status":        "active",
			"stock": gin.H{
				"total":       10,
				"available":   6,
				"rented":      3,
				"maintenance": 1,
			},
			"pricing": gin.H{
				"daily":   150,
				"weekly":  900,
				"monthly": 3750,
				"deposit": 3000,
			},
			"specs": []gin.H{
				{
					"id":           "spec_001",
					"name":         "标准版 121cm",
					"daily_rent":   150,
					"weekly_rent":  900,
					"monthly_rent": 3750,
					"deposit":      3000,
					"stock":        5,
				},
				{
					"id":           "spec_002",
					"name":         "专业版 131cm",
					"daily_rent":   180,
					"weekly_rent":  1080,
					"monthly_rent": 4500,
					"deposit":      3500,
					"stock":        5,
				},
			},
			"rating":       4.8,
			"review_count": 128,
			"created_at":   time.Now().AddDate(0, 0, -30).Format(time.RFC3339),
			"updated_at":   time.Now().AddDate(0, 0, -1).Format(time.RFC3339),
		},
		{
			"id":            "instrument_002",
			"name":          "卡马吉他 D1C",
			"brand":         "卡马",
			"category_id":   "category_002",
			"category_name": "吉他",
			"model":         "D1C",
			"level":         "beginner",
			"description":   "入门级民谣吉他，性价比高，适合初学者",
			"images":        []string{"/images/guitar1.jpg"},
			"video":         "",
			"status":        "active",
			"stock": gin.H{
				"total":       25,
				"available":   20,
				"rented":      4,
				"maintenance": 1,
			},
			"pricing": gin.H{
				"daily":   50,
				"weekly":  300,
				"monthly": 1250,
				"deposit": 1000,
			},
			"specs": []gin.H{
				{
					"id":           "spec_003",
					"name":         "标准 41寸",
					"daily_rent":   50,
					"weekly_rent":  300,
					"monthly_rent": 1250,
					"deposit":      1000,
					"stock":        25,
				},
			},
			"rating":       4.6,
			"review_count": 89,
			"created_at":   time.Now().AddDate(0, -1, -15).Format(time.RFC3339),
			"updated_at":   time.Now().AddDate(0, 0, -2).Format(time.RFC3339),
		},
		{
			"id":            "instrument_003",
			"name":          "敦煌古筝 696D",
			"brand":         "敦煌",
			"category_id":   "category_003",
			"category_name": "古筝",
			"model":         "696D",
			"level":         "intermediate",
			"description":   "中级演奏古筝，音色优美，适合进阶学习者",
			"images":        []string{"/images/guzheng1.jpg", "/images/guzheng2.jpg"},
			"video":         "/videos/guzheng-demo.mp4",
			"status":        "maintenance",
			"stock": gin.H{
				"total":       5,
				"available":   0,
				"rented":      1,
				"maintenance": 4,
			},
			"pricing": gin.H{
				"daily":   80,
				"weekly":  480,
				"monthly": 2000,
				"deposit": 2000,
			},
			"specs": []gin.H{
				{
					"id":           "spec_004",
					"name":         "标准 21弦",
					"daily_rent":   80,
					"weekly_rent":  480,
					"monthly_rent": 2000,
					"deposit":      2000,
					"stock":        5,
				},
			},
			"rating":       4.9,
			"review_count": 45,
			"created_at":   time.Now().AddDate(0, -2, -20).Format(time.RFC3339),
			"updated_at":   time.Now().AddDate(0, 0, -5).Format(time.RFC3339),
		},
	}

	// Apply filters
	var filtered []gin.H
	for _, instrument := range instruments {
		match := true

		if categoryID != "" && instrument["category_id"] != categoryID {
			match = false
		}
		if brand != "" && instrument["brand"] != brand {
			match = false
		}
		if status != "" && instrument["status"] != status {
			match = false
		}

		if match {
			filtered = append(filtered, instrument)
		}
	}

	// Apply sorting
	switch sort {
	case "price":
		// Mock sorting by price
	case "rating":
		// Mock sorting by rating
	case "created_at":
		// Default: sort by created_at
	}

	// Pagination
	total := len(filtered)
	start := (page - 1) * perPage
	end := start + perPage
	if start > total {
		start = total
	}
	if end > total {
		end = total
	}

	paginatedInstruments := filtered[start:end]

	c.JSON(http.StatusOK, gin.H{
		"code": 20000,
		"data": gin.H{
			"instruments": paginatedInstruments,
			"pagination": gin.H{
				"page":        page,
				"per_page":    perPage,
				"total":       total,
				"total_pages": (total + perPage - 1) / perPage,
			},
			"filters": gin.H{
				"category_id": categoryID,
				"brand":       brand,
				"status":      status,
				"sort":        sort,
			},
		},
	})
}

// GetInstrument - GET /api/instruments/:id
func (h *InstrumentHandler) GetInstrument(c *gin.Context) {
	id := c.Param("id")
	tenantID := middleware.GetTenantID(c.Request.Context())

	if id == "" || tenantID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    40002,
			"message": "instrument_id and tenant_id are required",
		})
		return
	}

	// Mock instrument detail
	instrument := gin.H{
		"id":            id,
		"name":          "雅马哈钢琴 U1",
		"brand":         "雅马哈",
		"category_id":   "category_001",
		"category_name": "钢琴",
		"model":         "U1",
		"level":         "standard",
		"description":   "专业级立式钢琴，适合初学者到高级演奏者",
		"material":      "实木",
		"size":          "121cm",
		"suitable":      "初学者到高级演奏者",
		"images":        []string{"/images/piano1.jpg", "/images/piano2.jpg", "/images/piano3.jpg"},
		"video":         "/videos/piano-demo.mp4",
		"status":        "active",
		"stock": gin.H{
			"total":       10,
			"available":   6,
			"rented":      3,
			"maintenance": 1,
		},
		"pricing": gin.H{
			"daily":   150,
			"weekly":  900,
			"monthly": 3750,
			"deposit": 3000,
		},
		"specs": []gin.H{
			{
				"id":           "spec_001",
				"name":         "标准版 121cm",
				"daily_rent":   150,
				"weekly_rent":  900,
				"monthly_rent": 3750,
				"deposit":      3000,
				"stock":        5,
				"attributes": gin.H{
					"height": "121cm",
					"weight": "250kg",
					"color":  "black",
				},
			},
			{
				"id":           "spec_002",
				"name":         "专业版 131cm",
				"daily_rent":   180,
				"weekly_rent":  1080,
				"monthly_rent": 4500,
				"deposit":      3500,
				"stock":        5,
				"attributes": gin.H{
					"height": "131cm",
					"weight": "280kg",
					"color":  "black",
				},
			},
		},
		"delivery_options": []gin.H{
			{"type": "pickup", "name": "门店自提", "fee": 0},
			{"type": "delivery", "name": "送货上门", "fee": 100},
		},
		"rating":       4.8,
		"review_count": 128,
		"sold_count":   45,
		"created_at":   time.Now().AddDate(0, 0, -30).Format(time.RFC3339),
		"updated_at":   time.Now().AddDate(0, 0, -1).Format(time.RFC3339),
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 20000,
		"data": instrument,
	})
}

// CreateInstrument - POST /api/instruments
func (h *InstrumentHandler) CreateInstrument(c *gin.Context) {
	tenantID := middleware.GetTenantID(c.Request.Context())
	if tenantID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    40002,
			"message": "tenant_id is required",
		})
		return
	}

	var req struct {
		Name        string   `json:"name" binding:"required"`
		Brand       string   `json:"brand" binding:"required"`
		CategoryID  string   `json:"category_id" binding:"required"`
		Model       string   `json:"model"`
		Level       string   `json:"level" binding:"required"`
		Description string   `json:"description"`
		Material    string   `json:"material"`
		Size        string   `json:"size"`
		Suitable    string   `json:"suitable"`
		Images      []string `json:"images"`
		Video       string   `json:"video"`
		Specs       []gin.H  `json:"specs"`
		Pricing     gin.H    `json:"pricing"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    40002,
			"message": "Invalid parameters: " + err.Error(),
		})
		return
	}

	// Mock created instrument
	instrument := gin.H{
		"id":          "instrument_new_" + time.Now().Format("20060102150405"),
		"name":        req.Name,
		"brand":       req.Brand,
		"category_id": req.CategoryID,
		"model":       req.Model,
		"level":       req.Level,
		"description": req.Description,
		"material":    req.Material,
		"size":        req.Size,
		"suitable":    req.Suitable,
		"images":      req.Images,
		"video":       req.Video,
		"status":      "active",
		"stock": gin.H{
			"total":       0,
			"available":   0,
			"rented":      0,
			"maintenance": 0,
		},
		"specs":      req.Specs,
		"pricing":    req.Pricing,
		"created_at": time.Now().Format(time.RFC3339),
		"updated_at": time.Now().Format(time.RFC3339),
		"created_by": c.GetString("user_id"),
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 20000,
		"data": instrument,
	})
}

// UpdateInstrument - PUT /api/instruments/:id
func (h *InstrumentHandler) UpdateInstrument(c *gin.Context) {
	id := c.Param("id")
	tenantID := middleware.GetTenantID(c.Request.Context())

	if id == "" || tenantID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    40002,
			"message": "instrument_id and tenant_id are required",
		})
		return
	}

	var req struct {
		Name        string   `json:"name"`
		Brand       string   `json:"brand"`
		CategoryID  string   `json:"category_id"`
		Model       string   `json:"model"`
		Level       string   `json:"level"`
		Description string   `json:"description"`
		Material    string   `json:"material"`
		Size        string   `json:"size"`
		Suitable    string   `json:"suitable"`
		Images      []string `json:"images"`
		Video       string   `json:"video"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    40002,
			"message": "Invalid parameters: " + err.Error(),
		})
		return
	}

	// Mock updated instrument
	instrument := gin.H{
		"id":          id,
		"name":        req.Name,
		"brand":       req.Brand,
		"category_id": req.CategoryID,
		"model":       req.Model,
		"level":       req.Level,
		"description": req.Description,
		"material":    req.Material,
		"size":        req.Size,
		"suitable":    req.Suitable,
		"images":      req.Images,
		"video":       req.Video,
		"updated_at":  time.Now().Format(time.RFC3339),
		"updated_by":  c.GetString("user_id"),
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 20000,
		"data": instrument,
	})
}

// UpdateShelfStatus - PUT /api/instruments/:id/shelf-status
func (h *InstrumentHandler) UpdateShelfStatus(c *gin.Context) {
	id := c.Param("id")
	tenantID := middleware.GetTenantID(c.Request.Context())

	if id == "" || tenantID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    40002,
			"message": "instrument_id and tenant_id are required",
		})
		return
	}

	var req struct {
		Status string `json:"status" binding:"required"` // "active", "inactive", "maintenance"
		Reason string `json:"reason"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    40002,
			"message": "Invalid parameters: " + err.Error(),
		})
		return
	}

	validStatuses := []string{"active", "inactive", "maintenance"}
	isValid := false
	for _, s := range validStatuses {
		if req.Status == s {
			isValid = true
			break
		}
	}

	if !isValid {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    40002,
			"message": "Status must be one of: active, inactive, maintenance",
		})
		return
	}

	result := gin.H{
		"id":         id,
		"status":     req.Status,
		"reason":     req.Reason,
		"updated_at": time.Now().Format(time.RFC3339),
		"updated_by": c.GetString("user_id"),
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 20000,
		"data": result,
	})
}
