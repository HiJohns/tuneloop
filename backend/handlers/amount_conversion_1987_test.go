package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"tuneloop-backend/handlers/testfixtures"
	"tuneloop-backend/models"
)

// #1987: Update handler must convert yuan → cents like the Create handler.
func TestUpdateMembershipLevel_MinAmountYuanToCents(t *testing.T) {
	db := testfixtures.SetupTestDB(t)
	require.NoError(t, db.Create(&models.MembershipLevel{ID: 77, Name: "测试级", MinAmount: 0}).Error)

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.PUT("/api/admin/membership-levels/:id", UpdateMembershipLevel)

	body, _ := json.Marshal(map[string]interface{}{"min_amount": 100})
	req := httptest.NewRequest("PUT", "/api/admin/membership-levels/77", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var lv models.MembershipLevel
	require.NoError(t, db.Where("id = ?", 77).First(&lv).Error)
	require.Equal(t, models.Cents(10000), lv.MinAmount, "100 元 → 10000 分（#1987）")
}
