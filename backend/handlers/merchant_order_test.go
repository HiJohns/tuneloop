package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"tuneloop-backend/handlers/testfixtures"
	"tuneloop-backend/middleware"
	"tuneloop-backend/models"
)

func TestListMerchantOrders_StartDateFilter(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := testfixtures.SetupTestDB(t)

	tenantID := uuid.New().String()
	orgID := uuid.New().String()
	userID := uuid.New().String()

	// Order created today
	now := time.Now()
	todayOrder := models.Order{
		ID: uuid.New().String(), TenantID: tenantID, OrgID: orgID,
		UserID: userID, InstrumentID: uuid.New().String(),
		Status: "paid", Level: "standard", MonthlyRent: 10000,
	}
	testfixtures.SeedUserFK(t, db, todayOrder.UserID, todayOrder.TenantID, todayOrder.OrgID)
	testfixtures.SeedInstrumentFK(t, db, todayOrder.InstrumentID, todayOrder.TenantID, todayOrder.OrgID)
	testfixtures.SeedOrderParents(t, db, &todayOrder) // #2443: strict schema order FKs
	db.Create(&todayOrder)

	// Order created 3 days ago
	past := now.AddDate(0, 0, -3)
	pastOrder := models.Order{
		ID: uuid.New().String(), TenantID: tenantID, OrgID: orgID,
		UserID: userID, InstrumentID: uuid.New().String(),
		Status: "paid", Level: "standard", MonthlyRent: 10000,
	}
	testfixtures.SeedUserFK(t, db, pastOrder.UserID, pastOrder.TenantID, pastOrder.OrgID)
	testfixtures.SeedInstrumentFK(t, db, pastOrder.InstrumentID, pastOrder.TenantID, pastOrder.OrgID)
	testfixtures.SeedOrderParents(t, db, &pastOrder) // #2443: strict schema order FKs
	db.Create(&pastOrder)
	// Backdate created_at for the past order
	db.Model(&models.Order{}).Where("id = ?", pastOrder.ID).
		Update("created_at", past)

	router := gin.New()
	router.Use(func(c *gin.Context) {
		ctx := c.Request.Context()
		ctx = context.WithValue(ctx, middleware.ContextKeyTenantID, tenantID)
		ctx = context.WithValue(ctx, middleware.ContextKeyOrgID, orgID)
		ctx = context.WithValue(ctx, middleware.ContextKeyRole, "ADMIN")
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})
	router.GET("/api/merchant/orders", ListMerchantOrders)

	todayStr := now.Format("2006-01-02")

	req := httptest.NewRequest("GET", fmt.Sprintf("/api/merchant/orders?start_date=%s", todayStr), nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var response struct {
		Code int `json:"code"`
		Data struct {
			List  []map[string]interface{} `json:"list"`
			Total int64                    `json:"total"`
		} `json:"data"`
	}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	assert.Equal(t, 20000, response.Code)
	// Only today's order should be returned (past order excluded)
	assert.Equal(t, int64(1), response.Data.Total)
}

func TestListMerchantOrders_EndDateIncludesFullDay(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := testfixtures.SetupTestDB(t)

	tenantID := uuid.New().String()
	orgID := uuid.New().String()
	userID := uuid.New().String()
	now := time.Now()

	// Order created at 23:59 today
	lateToday := time.Date(now.Year(), now.Month(), now.Day(), 23, 59, 0, 0, now.Location())
	lateOrder := models.Order{
		ID: uuid.New().String(), TenantID: tenantID, OrgID: orgID,
		UserID: userID, InstrumentID: uuid.New().String(),
		Status: "paid", Level: "standard", MonthlyRent: 10000,
	}
	testfixtures.SeedUserFK(t, db, lateOrder.UserID, lateOrder.TenantID, lateOrder.OrgID)
	testfixtures.SeedInstrumentFK(t, db, lateOrder.InstrumentID, lateOrder.TenantID, lateOrder.OrgID)
	testfixtures.SeedOrderParents(t, db, &lateOrder) // #2443: strict schema order FKs
	db.Create(&lateOrder)
	db.Model(&models.Order{}).Where("id = ?", lateOrder.ID).
		Update("created_at", lateToday)

	// Order created tomorrow
	tomorrow := now.AddDate(0, 0, 1)
	tomorrowOrder := models.Order{
		ID: uuid.New().String(), TenantID: tenantID, OrgID: orgID,
		UserID: userID, InstrumentID: uuid.New().String(),
		Status: "paid", Level: "standard", MonthlyRent: 10000,
	}
	testfixtures.SeedUserFK(t, db, tomorrowOrder.UserID, tomorrowOrder.TenantID, tomorrowOrder.OrgID)
	testfixtures.SeedInstrumentFK(t, db, tomorrowOrder.InstrumentID, tomorrowOrder.TenantID, tomorrowOrder.OrgID)
	testfixtures.SeedOrderParents(t, db, &tomorrowOrder) // #2443: strict schema order FKs
	db.Create(&tomorrowOrder)
	db.Model(&models.Order{}).Where("id = ?", tomorrowOrder.ID).
		Update("created_at", tomorrow)

	router := gin.New()
	router.Use(func(c *gin.Context) {
		ctx := c.Request.Context()
		ctx = context.WithValue(ctx, middleware.ContextKeyTenantID, tenantID)
		ctx = context.WithValue(ctx, middleware.ContextKeyOrgID, orgID)
		ctx = context.WithValue(ctx, middleware.ContextKeyRole, "ADMIN")
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})
	router.GET("/api/merchant/orders", ListMerchantOrders)

	todayStr := now.Format("2006-01-02")

	req := httptest.NewRequest("GET", fmt.Sprintf("/api/merchant/orders?start_date=%s&end_date=%s", todayStr, todayStr), nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var response struct {
		Code int `json:"code"`
		Data struct {
			Total int64 `json:"total"`
		} `json:"data"`
	}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	assert.Equal(t, 20000, response.Code)
	// Only today's late order should be returned (tomorrow order excluded)
	assert.Equal(t, int64(1), response.Data.Total)
}

func TestListMerchantOrders_NoDateReturnsAll(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := testfixtures.SetupTestDB(t)

	tenantID := uuid.New().String()
	orgID := uuid.New().String()
	userID := uuid.New().String()

	testfixtures.SeedUserFK(t, db, userID, tenantID, orgID)
	for i := 0; i < 5; i++ {
		o := models.Order{
			ID: uuid.New().String(), TenantID: tenantID, OrgID: orgID,
			UserID: userID, InstrumentID: uuid.New().String(),
			Status: "paid", Level: "standard", MonthlyRent: 10000,
		}
		testfixtures.SeedInstrumentFK(t, db, o.InstrumentID, tenantID, orgID)
		testfixtures.SeedOrderParents(t, db, &o) // #2443: strict schema order FKs
		db.Create(&o)
	}

	router := gin.New()
	router.Use(func(c *gin.Context) {
		ctx := c.Request.Context()
		ctx = context.WithValue(ctx, middleware.ContextKeyTenantID, tenantID)
		ctx = context.WithValue(ctx, middleware.ContextKeyOrgID, orgID)
		ctx = context.WithValue(ctx, middleware.ContextKeyRole, "ADMIN")
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})
	router.GET("/api/merchant/orders", ListMerchantOrders)

	req := httptest.NewRequest("GET", "/api/merchant/orders", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var response struct {
		Code int `json:"code"`
		Data struct {
			Total int64 `json:"total"`
		} `json:"data"`
	}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	assert.Equal(t, 20000, response.Code)
	assert.Equal(t, int64(5), response.Data.Total)
}
