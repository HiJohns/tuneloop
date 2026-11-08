package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"tuneloop-backend/handlers/testfixtures"
	"tuneloop-backend/models"
	"tuneloop-backend/testutil"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func seedTierInstrument(t *testing.T, db *gorm.DB, tenantID, orgID string, fullYuan float64, reducedYuan *float64) string {
	t.Helper()
	instID := uuid.New().String()
	full := models.FromYuan(fullYuan)
	inst := models.Instrument{
		ID: instID, TenantID: tenantID, OrgID: &orgID,
		SN: "2321-INST-" + instID[:8], StockStatus: "available",
		Deposit: &full,
		Pricing: `{"daily_rent":10,"deposit":` + "0" + `}`,
	}
	if reducedYuan != nil {
		reduced := models.FromYuan(*reducedYuan)
		inst.DepositReduced = &reduced
	}
	require.NoError(t, db.Create(&inst).Error)
	return instID
}

func seedEligibleGuarantors(t *testing.T, db *gorm.DB, userID string) []string {
	t.Helper()
	g1 := uuid.New().String()
	g2 := uuid.New().String()
	require.NoError(t, db.Create(&models.Guarantor{ID: g1, UserID: userID, Name: "G1", Phone: "13800000001"}).Error)
	require.NoError(t, db.Create(&models.Guarantor{ID: g2, UserID: userID, Name: "G2", Phone: "13800000002"}).Error)
	return []string{g1, g2}
}

func createTierOrder(t *testing.T, router *gin.Engine, body map[string]interface{}) map[string]interface{} {
	t.Helper()
	httpCode, bizCode, data := post1867JSON(t, router, "/api/user/orders", body)
	require.Equal(t, http.StatusCreated, httpCode)
	require.Equal(t, 20000, bizCode)
	return data
}

func TestDepositTiers_2321_Single(t *testing.T) {
	db, handler := setup1867Fixture(t)
	tenantID, orgID, _ := testfixtures.NewTenantIDs("2321a1b2c3d4")
	sub := uuid.New().String()
	userID := seed1867User(t, db, tenantID, orgID, sub, true, strPtr1867("student"), true, 700)
	guarantors := seedEligibleGuarantors(t, db, userID)
	router := new1867Router(handler, tenantID, orgID, sub)

	t.Run("full by default", func(t *testing.T) {
		instID := seedTierInstrument(t, db, tenantID, orgID, 500, ptrFloat(200))
		data := createTierOrder(t, router, map[string]interface{}{
			"instrument_id": instID, "start_date": "2026-09-15", "end_date": "2026-09-20",
		})
		var order models.Order
		require.NoError(t, db.Where("id = ?", data["order_id"]).First(&order).Error)
		assert.Equal(t, models.DepositTierFull, order.DepositTier)
		assert.Equal(t, models.FromYuan(500), order.Deposit)
		require.NotNil(t, order.DepositFull)
		assert.Equal(t, models.FromYuan(500), *order.DepositFull)
		assert.False(t, order.DepositWaived)
	})

	t.Run("reduced with eligible user and guarantors", func(t *testing.T) {
		instID := seedTierInstrument(t, db, tenantID, orgID, 500, ptrFloat(200))
		data := createTierOrder(t, router, map[string]interface{}{
			"instrument_id": instID, "start_date": "2026-09-15", "end_date": "2026-09-20",
			"deposit_tier": "reduced", "guarantor_ids": guarantors, "recommendation_letter": "/uploads/l.jpg",
		})
		var order models.Order
		require.NoError(t, db.Where("id = ?", data["order_id"]).First(&order).Error)
		assert.Equal(t, models.DepositTierReduced, order.DepositTier)
		assert.Equal(t, models.FromYuan(200), order.Deposit)
		require.NotNil(t, order.DepositReduced)
		assert.Equal(t, models.FromYuan(200), *order.DepositReduced)
		assert.Equal(t, "/uploads/l.jpg", order.RecommendationLetter)
		assert.False(t, order.DepositWaived)
		var links int64
		db.Model(&models.OrderGuarantor{}).Where("order_id = ?", order.ID).Count(&links)
		assert.EqualValues(t, 2, links)
	})

	t.Run("reduced amount zero keeps reduced requirements", func(t *testing.T) {
		instID := seedTierInstrument(t, db, tenantID, orgID, 500, ptrFloat(0))
		data := createTierOrder(t, router, map[string]interface{}{
			"instrument_id": instID, "start_date": "2026-09-15", "end_date": "2026-09-20",
			"deposit_tier": "reduced", "guarantor_ids": guarantors, "recommendation_letter": "/uploads/l0.jpg",
		})
		var order models.Order
		require.NoError(t, db.Where("id = ?", data["order_id"]).First(&order).Error)
		assert.Equal(t, models.DepositTierReduced, order.DepositTier)
		assert.Equal(t, models.Cents(0), order.Deposit)
		var links int64
		db.Model(&models.OrderGuarantor{}).Where("order_id = ?", order.ID).Count(&links)
		assert.EqualValues(t, 2, links)
	})

	t.Run("zero deposit instrument ignores requirements", func(t *testing.T) {
		instID := seedTierInstrument(t, db, tenantID, orgID, 0, nil)
		data := createTierOrder(t, router, map[string]interface{}{
			"instrument_id": instID, "start_date": "2026-09-15", "end_date": "2026-09-20",
			"deposit_tier": "reduced",
		})
		var order models.Order
		require.NoError(t, db.Where("id = ?", data["order_id"]).First(&order).Error)
		assert.Equal(t, models.DepositTierZero, order.DepositTier)
		assert.Equal(t, models.Cents(0), order.Deposit)
		assert.Empty(t, order.RecommendationLetter)
	})

	t.Run("no reduced config falls back to full", func(t *testing.T) {
		instID := seedTierInstrument(t, db, tenantID, orgID, 500, nil)
		data := createTierOrder(t, router, map[string]interface{}{
			"instrument_id": instID, "start_date": "2026-09-15", "end_date": "2026-09-20",
			"deposit_tier": "reduced", "guarantor_ids": guarantors, "recommendation_letter": "/uploads/l.jpg",
		})
		var order models.Order
		require.NoError(t, db.Where("id = ?", data["order_id"]).First(&order).Error)
		assert.Equal(t, models.DepositTierFull, order.DepositTier)
		assert.Equal(t, models.FromYuan(500), order.Deposit)
	})

	t.Run("legacy deposit_waived maps to reduced not zero", func(t *testing.T) {
		instID := seedTierInstrument(t, db, tenantID, orgID, 500, ptrFloat(200))
		data := createTierOrder(t, router, map[string]interface{}{
			"instrument_id": instID, "start_date": "2026-09-15", "end_date": "2026-09-20",
			"deposit_waived": true, "guarantor_ids": guarantors, "recommendation_letter": "/uploads/legacy.jpg",
		})
		var order models.Order
		require.NoError(t, db.Where("id = ?", data["order_id"]).First(&order).Error)
		assert.Equal(t, models.DepositTierReduced, order.DepositTier)
		assert.Equal(t, models.FromYuan(200), order.Deposit)
		assert.False(t, order.DepositWaived)
	})
}

func TestDepositTiers_2321_BatchMixed(t *testing.T) {
	db, handler := setup1867Fixture(t)
	tenantID, orgID, _ := testfixtures.NewTenantIDs("2321b2c3d4e5")
	sub := uuid.New().String()
	userID := seed1867User(t, db, tenantID, orgID, sub, true, strPtr1867("student"), true, 700)
	guarantors := seedEligibleGuarantors(t, db, userID)
	router := new1867Router(handler, tenantID, orgID, sub)

	reducedInst := seedTierInstrument(t, db, tenantID, orgID, 500, ptrFloat(200))
	zeroInst := seedTierInstrument(t, db, tenantID, orgID, 0, nil)
	body := map[string]interface{}{
		"items": []map[string]interface{}{
			{"instrument_id": reducedInst, "start_date": "2026-09-15", "end_date": "2026-09-20"},
			{"instrument_id": zeroInst, "start_date": "2026-09-15", "end_date": "2026-09-20"},
		},
		"deposit_tier": "reduced", "guarantor_ids": guarantors, "recommendation_letter": "/uploads/batch.jpg",
	}
	httpCode, bizCode, _ := post1867JSON(t, router, "/api/user/orders/batch", body)
	require.Equal(t, http.StatusCreated, httpCode)
	require.Equal(t, 20000, bizCode)

	var reducedOrder, zeroOrder models.Order
	require.NoError(t, db.Where("user_id = ? AND instrument_id = ?", userID, reducedInst).First(&reducedOrder).Error)
	require.NoError(t, db.Where("user_id = ? AND instrument_id = ?", userID, zeroInst).First(&zeroOrder).Error)
	assert.Equal(t, models.DepositTierReduced, reducedOrder.DepositTier)
	assert.Equal(t, models.FromYuan(200), reducedOrder.Deposit)
	assert.Equal(t, models.DepositTierZero, zeroOrder.DepositTier)
	assert.Equal(t, models.Cents(0), zeroOrder.Deposit)
	assert.Equal(t, "/uploads/batch.jpg", reducedOrder.RecommendationLetter)
	assert.Empty(t, zeroOrder.RecommendationLetter)
}

func TestDepositTiers_2321_RejectsReducedAboveFull(t *testing.T) {
	db, _ := setup1867Fixture(t)
	tenantID, orgID, _ := testfixtures.NewTenantIDs("2321c3d4e5f6")
	instID := seedTierInstrument(t, db, tenantID, orgID, 100, nil)

	gin.SetMode(gin.TestMode)
	r := gin.New()
	actor := testutil.MakeSiteAdmin(tenantID, orgID, uuid.New().String())
	r.Use(func(c *gin.Context) {
		c.Request = c.Request.WithContext(actor.InjectContext(c.Request.Context()))
		c.Next()
	})
	r.PUT("/instruments/:id", UpdateInstrument)

	body := `{"deposit":100,"deposit_reduced_enabled":true,"deposit_reduced":200}`
	req := httptest.NewRequest(http.MethodPut, "/instruments/"+instID, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "cannot exceed")
}

func ptrFloat(v float64) *float64 { return &v }

func TestDepositTiers_2321_OrderDetailExposesReducedFields(t *testing.T) {
	db, handler := setup1867Fixture(t)
	tenantID, orgID, _ := testfixtures.NewTenantIDs("2321d4e5f6a7")
	sub := uuid.New().String()
	userID := seed1867User(t, db, tenantID, orgID, sub, true, strPtr1867("student"), true, 700)
	guarantors := seedEligibleGuarantors(t, db, userID)
	instID := seedTierInstrument(t, db, tenantID, orgID, 500, ptrFloat(200))
	createRouter := new1867Router(handler, tenantID, orgID, sub)
	data := createTierOrder(t, createRouter, map[string]interface{}{
		"instrument_id": instID, "start_date": "2026-09-15", "end_date": "2026-09-20",
		"deposit_tier": "reduced", "guarantor_ids": guarantors, "recommendation_letter": "/uploads/detail.jpg",
	})

	gin.SetMode(gin.TestMode)
	r := gin.New()
	actor := testutil.MakeCustomer(tenantID, userID)
	r.Use(func(c *gin.Context) {
		c.Request = c.Request.WithContext(actor.InjectContext(c.Request.Context()))
		c.Next()
	})
	r.GET("/orders/:id", GetOrder)
	req := httptest.NewRequest(http.MethodGet, "/orders/"+data["order_id"].(string), nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	var resp struct {
		Code int                    `json:"code"`
		Data map[string]interface{} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, 20000, resp.Code)
	assert.Equal(t, models.DepositTierReduced, resp.Data["deposit_tier"])
	assert.Equal(t, float64(20000), resp.Data["deposit_reduced"]) // cents
	assert.Equal(t, "/uploads/detail.jpg", resp.Data["recommendation_letter"])
	guarantorsData, ok := resp.Data["guarantors"].([]interface{})
	require.True(t, ok)
	assert.Len(t, guarantorsData, 2)
}
