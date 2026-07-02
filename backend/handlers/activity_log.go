package handlers

import (
	"net/http"
	"sort"
	"time"
	"tuneloop-backend/database"
	"tuneloop-backend/middleware"
	"tuneloop-backend/models"
	"tuneloop-backend/services"

	"github.com/gin-gonic/gin"
)

type activityEvent struct {
	Event    string `json:"event"`
	Time     string `json:"time"`
	Operator string `json:"operator"`
	Media    []struct {
		URL       string `json:"url"`
		BatchType string `json:"batch_type"`
	} `json:"media"`
}

type activitySession struct {
	OrderID   string           `json:"order_id"`
	Status    string           `json:"status"`
	StartDate string           `json:"start_date"`
	EndDate   string           `json:"end_date"`
	Events    []activityEvent  `json:"events"`
}

// statusToBatchType maps order status transitions to their relevant batch_type.
var statusToBatchType = map[string]string{
	" → pending_shipment": "",
	" → paid":             "",
	" → shipped":          "shipping",
	" → in_lease":         "",
	" → returning":        "",
	" → returned":         "receiving",
	" → completed":        "",
	" → cancelled":        "",
	" → assessed":         "receiving",
	" → maintenance":      "repaired",
}

// GetInstrumentActivityLog returns the full activity log for an instrument
func GetInstrumentActivityLog(c *gin.Context) {
	instrumentID := c.Param("id")
	if instrumentID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40001, "message": "instrument id is required"})
		return
	}

	ctx := c.Request.Context()
	tenantID := middleware.GetTenantID(ctx)
	db := database.GetDB().WithContext(ctx)

	// Find all orders for this instrument
	var orders []models.Order
	if err := db.Where("instrument_id = ? AND tenant_id = ?", instrumentID, tenantID).
		Order("created_at ASC").Find(&orders).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50000, "message": "failed to query orders"})
		return
	}

	// Get all status histories for these orders
	var allHistory []models.OrderStatusHistory
	var orderIDs []string
	for _, o := range orders {
		orderIDs = append(orderIDs, o.ID)
	}
	if len(orderIDs) > 0 {
		db.Where("order_id IN ? AND tenant_id = ?", orderIDs, tenantID).
			Order("changed_at ASC").Find(&allHistory)
	}

	// Group history by order
	historyByOrder := make(map[string][]models.OrderStatusHistory)
	for _, h := range allHistory {
		historyByOrder[h.OrderID] = append(historyByOrder[h.OrderID], h)
	}

	// Get all media for this instrument
	storage := services.MediaStorageFromContext(c)
	var mediaList []models.InstrumentMedia
	db.Where("instrument_id = ? AND tenant_id = ?", instrumentID, tenantID).
		Find(&mediaList)

	// Build media lookup by batch_type
	mediaByBatchType := make(map[string][]string)
	for _, m := range mediaList {
		url, _ := storage.GetURL(ctx, m.StorageKey)
		if url == "" {
			url = "/uploads/media/" + m.StorageKey
		}
		mediaByBatchType[m.BatchType] = append(mediaByBatchType[m.BatchType], url)
	}

	// Build sessions
	var sessions []activitySession
	for _, o := range orders {
		histories := historyByOrder[o.ID]

		var events []activityEvent
		for _, h := range histories {
			eventName := h.Notes
			if eventName == "" {
				eventName = h.StatusFrom + " → " + h.StatusTo
			}

			// Determine which batch_type to show for this event
			eventSuffix := " → " + h.StatusTo
			batchType := statusToBatchType[eventSuffix]

			var mediaItems []struct {
				URL       string `json:"url"`
				BatchType string `json:"batch_type"`
			}
			if batchType != "" {
				if urls, ok := mediaByBatchType[batchType]; ok {
					for _, u := range urls {
						mediaItems = append(mediaItems, struct {
							URL       string `json:"url"`
							BatchType string `json:"batch_type"`
						}{URL: u, BatchType: batchType})
					}
				}
			}

			operator := ""
			if h.ChangedBy != nil {
				operator = *h.ChangedBy
			}

			events = append(events, activityEvent{
				Event:    eventName,
				Time:     h.ChangedAt.Format(time.RFC3339),
				Operator: operator,
				Media:    mediaItems,
			})
		}

		startDate := ""
		if o.StartDate != nil {
			startDate = *o.StartDate
		}
		endDate := ""
		if o.EndDate != nil {
			endDate = *o.EndDate
		}

		sessions = append(sessions, activitySession{
			OrderID:   o.ID,
			Status:    o.Status,
			StartDate: startDate,
			EndDate:   endDate,
			Events:    events,
		})
	}

	// Sort sessions by start date descending (newest first)
	sort.Slice(sessions, func(i, j int) bool {
		return sessions[i].StartDate > sessions[j].StartDate
	})

	c.JSON(http.StatusOK, gin.H{
		"code": 20000,
		"data": gin.H{
			"sessions": sessions,
		},
	})
}
