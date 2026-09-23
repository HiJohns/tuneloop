package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"tuneloop-backend/database"
	"tuneloop-backend/middleware"
	"tuneloop-backend/models"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// #2041: 待提交订单缓存 三端点 + 归属隔离。
func TestPendingOrders_CRUDAndIsolation(t *testing.T) {
	cleanup := setupMockIAMAndDB(t)
	defer cleanup()
	db := database.GetDB()
	require.NoError(t, db.AutoMigrate(&models.PendingOrder{}))

	tenantID := uuid.New().String()
	mkUser := func() (string, string) {
		id := uuid.New().String()
		u := models.User{ID: id, TenantID: tenantID, OrgID: tenantID, IAMSub: id, Name: "U", Status: "active"}
		require.NoError(t, db.Create(&u).Error)
		return id, id // (localID, iamSub)
	}
	_, subA := mkUser()
	_, subB := mkUser()

	router := gin.New()
	router.Use(func(c *gin.Context) {
		ctx := context.WithValue(c.Request.Context(), middleware.ContextKeyUserID, c.GetHeader("X-Sub"))
		ctx = context.WithValue(ctx, middleware.ContextKeyTenantID, tenantID)
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})
	router.POST("/api/user/pending-orders", CreatePendingOrder)
	router.GET("/api/user/pending-orders", ListPendingOrders)
	router.DELETE("/api/user/pending-orders/:id", DeletePendingOrder)

	do := func(method, path, sub, body string) *httptest.ResponseRecorder {
		var r *http.Request
		if body != "" {
			r = httptest.NewRequest(method, path, bytes.NewBufferString(body))
			r.Header.Set("Content-Type", "application/json")
		} else {
			r = httptest.NewRequest(method, path, nil)
		}
		r.Header.Set("X-Sub", sub)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, r)
		return w
	}

	// create
	w := do("POST", "/api/user/pending-orders", subA, `{"payload":{"instrument_id":"abc","days":3}}`)
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	var created map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &created))
	id := created["data"].(map[string]interface{})["id"].(string)

	// list (A) → 1 with payload object
	w = do("GET", "/api/user/pending-orders", subA, "")
	require.Equal(t, http.StatusOK, w.Code)
	var listed map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &listed))
	list := listed["data"].(map[string]interface{})["list"].([]interface{})
	require.Len(t, list, 1)
	assert.Equal(t, "abc", list[0].(map[string]interface{})["payload"].(map[string]interface{})["instrument_id"])

	// isolation: B cannot delete A's pending order
	w = do("DELETE", "/api/user/pending-orders/"+id, subB, "")
	assert.Equal(t, http.StatusNotFound, w.Code, "他人待提交订单不可操作")

	// A deletes → abandoned → list empty
	require.Equal(t, http.StatusOK, do("DELETE", "/api/user/pending-orders/"+id, subA, "").Code)
	w = do("GET", "/api/user/pending-orders", subA, "")
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &listed))
	assert.Len(t, listed["data"].(map[string]interface{})["list"].([]interface{}), 0)
}
