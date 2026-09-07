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

// TestMembershipLevelBenefits covers #1830: per-level benefit rows
// (admin bulk replace + customer read).
func TestMembershipLevelBenefits(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := testfixtures.SetupTestDB(t)

	require.NoError(t, db.Create(&models.MembershipLevel{ID: 1, Name: "初级", MinAmount: 0}).Error)
	require.NoError(t, db.Create(&models.MembershipLevel{ID: 2, Name: "中级", MinAmount: 5000}).Error)

	router := gin.New()
	router.GET("/membership/benefits", GetMembershipBenefits)
	router.PUT("/admin/membership-levels/:id/benefits", UpdateLevelBenefits)
	router.GET("/admin/membership-levels/:id/benefits", ListLevelBenefits)

	t.Run("customer read requires level_id", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/membership/benefits", nil)
		router.ServeHTTP(w, req)
		require.Equal(t, http.StatusBadRequest, w.Code)
		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		require.Equal(t, float64(40002), resp["code"])
	})

	t.Run("customer read empty level returns empty list", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/membership/benefits?level_id=1", nil)
		router.ServeHTTP(w, req)
		require.Equal(t, http.StatusOK, w.Code)
		var resp struct {
			Code float64 `json:"code"`
			Data struct {
				LevelID int `json:"level_id"`
				List    []struct {
					Title string `json:"title"`
				} `json:"list"`
			} `json:"data"`
		}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		require.Equal(t, float64(20000), resp.Code)
		require.Equal(t, 1, resp.Data.LevelID)
		require.Empty(t, resp.Data.List)
	})

	t.Run("admin bulk replace then list preserves order", func(t *testing.T) {
		payload := `{"items":[
			{"title":"租金返现","description":"0.5%"},
			{"title":"积分抵用","description":"30%"}
		]}`
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPut, "/admin/membership-levels/1/benefits", bytes.NewBufferString(payload))
		req.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(w, req)
		require.Equal(t, http.StatusOK, w.Code)

		w2 := httptest.NewRecorder()
		req2 := httptest.NewRequest(http.MethodGet, "/admin/membership-levels/1/benefits", nil)
		router.ServeHTTP(w2, req2)
		require.Equal(t, http.StatusOK, w2.Code)
		var resp struct {
			Code float64 `json:"code"`
			Data struct {
				List []models.MembershipLevelBenefit `json:"list"`
			} `json:"data"`
		}
		require.NoError(t, json.Unmarshal(w2.Body.Bytes(), &resp))
		require.Len(t, resp.Data.List, 2)
		require.Equal(t, "租金返现", resp.Data.List[0].Title)
		require.Equal(t, "积分抵用", resp.Data.List[1].Title)
		require.Equal(t, 1, resp.Data.List[0].SortOrder)
		require.Equal(t, 2, resp.Data.List[1].SortOrder)
	})

	t.Run("replace clears previous rows", func(t *testing.T) {
		payload := `{"items":[{"title":"仅一条","description":""}]}`
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPut, "/admin/membership-levels/1/benefits", bytes.NewBufferString(payload))
		req.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(w, req)
		require.Equal(t, http.StatusOK, w.Code)

		w2 := httptest.NewRecorder()
		req2 := httptest.NewRequest(http.MethodGet, "/admin/membership-levels/1/benefits", nil)
		router.ServeHTTP(w2, req2)
		var resp struct {
			Code float64 `json:"code"`
			Data struct {
				List []models.MembershipLevelBenefit `json:"list"`
			} `json:"data"`
		}
		require.NoError(t, json.Unmarshal(w2.Body.Bytes(), &resp))
		require.Len(t, resp.Data.List, 1)
		require.Equal(t, "仅一条", resp.Data.List[0].Title)
	})

	t.Run("unknown level returns 404", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPut, "/admin/membership-levels/99/benefits", bytes.NewBufferString(`{"items":[]}`))
		req.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(w, req)
		require.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("level isolation between tiers", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/membership/benefits?level_id=2", nil)
		router.ServeHTTP(w, req)
		require.Equal(t, http.StatusOK, w.Code)
		var resp struct {
			Code float64 `json:"code"`
			Data struct {
				List []models.MembershipLevelBenefit `json:"list"`
			} `json:"data"`
		}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		require.Empty(t, resp.Data.List) // level 2 untouched by level 1 replaces
	})
}
