package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"tuneloop-backend/handlers/testfixtures"
	"tuneloop-backend/models"
	"tuneloop-backend/testutil"
)

// TestBatchDeleteInstruments covers #1798 batch delete semantics:
// mixed success/failure, empty ids rejection, and single-delete error code contract.
func TestBatchDeleteInstruments(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := testfixtures.SetupTestDB(t)

	tenantID, orgID, userID := testfixtures.NewTenantIDs("00000000b3c4")

	siteUUID := uuid.New()
	require.NoError(t, db.Create(&models.Site{
		ID:       siteUUID.String(),
		TenantID: tenantID,
		OrgID:    orgID,
		Name:     "测试网点",
	}).Error)

	// Instrument A: no orders → deletable
	instA := models.Instrument{
		ID:           uuid.New().String(),
		TenantID:     tenantID,
		OrgID:        &orgID,
		SiteID:       &siteUUID,
		CategoryName: "钢琴",
		SN:           "SN-BATCH-001",
		StockStatus:  "available",
	}
	// Instrument B: has linked order → blocked
	instB := models.Instrument{
		ID:           uuid.New().String(),
		TenantID:     tenantID,
		OrgID:        &orgID,
		SiteID:       &siteUUID,
		CategoryName: "钢琴",
		SN:           "SN-BATCH-002",
		StockStatus:  "available",
	}
	// Instrument C: rented → blocked
	instC := models.Instrument{
		ID:           uuid.New().String(),
		TenantID:     tenantID,
		OrgID:        &orgID,
		SiteID:       &siteUUID,
		CategoryName: "钢琴",
		SN:           "SN-BATCH-003",
		StockStatus:  "rented",
	}
	require.NoError(t, db.Create(&instA).Error)
	require.NoError(t, db.Create(&instB).Error)
	require.NoError(t, db.Create(&instC).Error)

	// Order linked to instrument B
	startStr := time.Now().AddDate(0, 0, -10).Format("2006-01-02")
	endStr := time.Now().Format("2006-01-02")
	order := models.Order{
		ID:           uuid.New().String(),
		TenantID:     tenantID,
		OrgID:        orgID,
		UserID:       uuid.New().String(),
		InstrumentID: instB.ID,
		Status:       "completed",
		Deposit:      models.FromYuan(100),
		StartDate:    &startStr,
		EndDate:      &endStr,
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}
	require.NoError(t, db.Create(&order).Error)

	admin := testutil.MakeSiteAdmin(tenantID, orgID, userID)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		ctx := admin.InjectContext(c.Request.Context())
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})
	router.DELETE("/instruments/batch", BatchDeleteInstruments)
	router.DELETE("/instruments/:id", DeleteInstrument)

	// --- Case 1: mixed scenario (A deletable, B linked-orders, C in use, D not found) ---
	body, _ := json.Marshal(map[string]interface{}{
		"ids": []string{instA.ID, instB.ID, instC.ID, uuid.New().String()},
	})
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("DELETE", "/instruments/batch", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	var resp struct {
		Code    int      `json:"code"`
		Deleted []string `json:"deleted"`
		Failed  []struct {
			ID     string `json:"id"`
			Reason string `json:"reason"`
		} `json:"failed"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, 20000, resp.Code)
	require.ElementsMatch(t, []string{instA.ID}, resp.Deleted)
	require.Len(t, resp.Failed, 3)

	reasonByID := map[string]string{}
	for _, f := range resp.Failed {
		reasonByID[f.ID] = f.Reason
	}
	require.Equal(t, "instrument has linked orders", reasonByID[instB.ID])
	require.Equal(t, "instrument in use", reasonByID[instC.ID])
	require.Equal(t, "instrument not found", reasonByID[reasonForID(resp.Failed, instB.ID, instC.ID)])

	// --- Case 2: empty ids → 40002 ---
	body2, _ := json.Marshal(map[string]interface{}{"ids": []string{}})
	w2 := httptest.NewRecorder()
	req2, _ := http.NewRequest("DELETE", "/instruments/batch", bytes.NewReader(body2))
	req2.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w2, req2)
	require.Equal(t, http.StatusBadRequest, w2.Code)
	var resp2 struct {
		Code int `json:"code"`
	}
	require.NoError(t, json.Unmarshal(w2.Body.Bytes(), &resp2))
	require.Equal(t, 40002, resp2.Code)

	// --- Case 3: single delete preserves error code contract (#1798 M1) ---
	w3 := httptest.NewRecorder()
	req3, _ := http.NewRequest("DELETE", "/instruments/"+instB.ID, nil)
	router.ServeHTTP(w3, req3)
	require.Equal(t, http.StatusConflict, w3.Code)
	var resp3 struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	}
	require.NoError(t, json.Unmarshal(w3.Body.Bytes(), &resp3))
	require.Equal(t, 40901, resp3.Code)
	require.Equal(t, "instrument has linked orders", resp3.Message)
}

// reasonForID returns the ID of the failed item that is neither instB nor instC
// (i.e. the not-found id), to make the ElementsMatch-style assertion readable.
func reasonForID(failed []struct {
	ID     string `json:"id"`
	Reason string `json:"reason"`
}, bID, cID string) string {
	for _, f := range failed {
		if f.ID != bID && f.ID != cID {
			return f.ID
		}
	}
	return ""
}
