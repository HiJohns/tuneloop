package handlers

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"tuneloop-backend/middleware"
)

type StockHandler struct {
	db *gorm.DB
}

func NewStockHandler(db *gorm.DB) *StockHandler {
	return &StockHandler{db: db}
}

// GetInstrumentStock - GET /api/instruments/:id/stock
func (h *StockHandler) GetInstrumentStock(c *gin.Context) {
	instrumentID := c.Param("id")
	tenantID := middleware.GetTenantID(c.Request.Context())

	if instrumentID == "" || tenantID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    40002,
			"message": "instrument_id and tenant_id are required",
		})
		return
	}

	// Mock stock data for demonstration
	// In production, this would query the inventory table
	stockData := gin.H{
		"instrument_id": instrumentID,
		"total_stock":   15,
		"available":     8,
		"rented":        5,
		"maintenance":   2,
		"last_updated":  time.Now().Format(time.RFC3339),
		"alert_level":   3, // Alert when stock falls below this level
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 20000,
		"data": stockData,
	})
}

// UpdateInstrumentStock - PUT /api/instruments/:id/stock
func (h *StockHandler) UpdateInstrumentStock(c *gin.Context) {
	instrumentID := c.Param("id")
	tenantID := middleware.GetTenantID(c.Request.Context())

	if instrumentID == "" || tenantID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    40002,
			"message": "instrument_id and tenant_id are required",
		})
		return
	}

	var req struct {
		Quantity    int    `json:"quantity" binding:"required"`
		Operation   string `json:"operation" binding:"required"` // "increase" or "decrease"
		Reason      string `json:"reason" binding:"required"`
		ReferenceID string `json:"reference_id"` // Optional: link to order/transfer
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    40002,
			"message": "Invalid parameters: " + err.Error(),
		})
		return
	}

	if req.Operation != "increase" && req.Operation != "decrease" {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    40002,
			"message": "Operation must be 'increase' or 'decrease'",
		})
		return
	}

	// In production, this would:
	// 1. Validate the operation (check if enough stock for decrease)
	// 2. Update the inventory table
	// 3. Create a transaction log
	// 4. Return the updated stock

	// Mock response
	result := gin.H{
		"instrument_id": instrumentID,
		"operation":     req.Operation,
		"quantity":      req.Quantity,
		"reason":        req.Reason,
		"reference_id":  req.ReferenceID,
		"new_stock":     10, // Mock new stock level
		"updated_at":    time.Now().Format(time.RFC3339),
		"operator":      c.GetString("user_id"), // From IAM context
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 20000,
		"data": result,
	})
}

// GetStockTransactionLog - GET /api/stock/transaction-log
func (h *StockHandler) GetStockTransactionLog(c *gin.Context) {
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
	instrumentID := c.Query("instrument_id")
	operation := c.Query("operation") // "increase" or "decrease"

	if page < 1 {
		page = 1
	}
	if perPage < 1 || perPage > 100 {
		perPage = 20
	}

	// Mock transaction log
	// In production, this would query a stock_transaction_logs table
	transactions := []gin.H{
		{
			"id":            "txn_001",
			"instrument_id": "instrument_123",
			"instrument_name": "雅马哈钢琴 U1",
			"operation":     "decrease",
			"quantity":      1,
			"old_stock":     9,
			"new_stock":     8,
			"reason":        "Rental order #ORD-20240324001",
			"reference_id":  "ORD-20240324001",
			"operator":      "user_001",
			"operator_name": "管理员",
			"created_at":    time.Now().AddDate(0, 0, -1).Format(time.RFC3339),
		},
		{
			"id":            "txn_002",
			"instrument_id": "instrument_123",
			"instrument_name": "雅马哈钢琴 U1",
			"operation":     "increase",
			"quantity":      1,
			"old_stock":     8,
			"new_stock":     9,
			"reason":        "Instrument returned from rental",
			"reference_id":  "RET-20240324002",
			"operator":      "user_001",
			"operator_name": "管理员",
			"created_at":    time.Now().AddDate(0, 0, -2).Format(time.RFC3339),
		},
		{
			"id":            "txn_003",
			"instrument_id": "instrument_456",
			"instrument_name": "卡马吉他 D1C",
			"operation":     "increase",
			"quantity":      5,
			"old_stock":     10,
			"new_stock":     15,
			"reason":        "New inventory purchase",
			"reference_id":  "PUR-20240324003",
			"operator":      "user_002",
			"operator_name": "采购员",
			"created_at":    time.Now().AddDate(0, 0, -3).Format(time.RFC3339),
		},
	}

	// Filter by instrument_id if provided
	if instrumentID != "" {
		var filtered []gin.H
		for _, txn := range transactions {
			if txn["instrument_id"] == instrumentID {
				filtered = append(filtered, txn)
			}
		}
		transactions = filtered
	}

	// Filter by operation if provided
	if operation != "" {
		var filtered []gin.H
		for _, txn := range transactions {
			if txn["operation"] == operation {
				filtered = append(filtered, txn)
			}
		}
		transactions = filtered
	}

	// Pagination
	total := len(transactions)
	start := (page - 1) * perPage
	end := start + perPage
	if start > total {
		start = total
	}
	if end > total {
		end = total
	}

	paginatedTransactions := transactions[start:end]

	c.JSON(http.StatusOK, gin.H{
		"code": 20000,
		"data": gin.H{
			"transactions": paginatedTransactions,
			"pagination": gin.H{
				"page":       page,
				"per_page":   perPage,
				"total":      total,
				"total_pages": (total + perPage - 1) / perPage,
			},
			"summary": gin.H{
				"total_operations": total,
				"today_increase":   2,
				"today_decrease":   1,
			},
		},
	})
}
