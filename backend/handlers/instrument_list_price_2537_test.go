package handlers

// #2537 两页列口径区分：乐器列表新增 `overdue_daily_fee`（分），必须与
// pricing-v2 同一条计算链（overrides → pricing JSONB 回落），不得前端解析 JSONB。
//
// 证据行：R1（列表字段 ↔ pricing-v2 同源 + 空值边界）。

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"tuneloop-backend/handlers/testfixtures"
	"tuneloop-backend/models"
	"tuneloop-backend/testutil"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// pricingV2Overdue2537 calls GET /instruments/:id/pricing-v2 and returns
// overdue_daily_fee (cents) plus whether the field was present.
func pricingV2Overdue2537(t *testing.T, f instrument2528Fixture, instrumentID string) (int64, bool) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	router := setupTestRouter(t, f.tenantID, f.userID)
	router.GET("/instruments/:id/pricing-v2", GetInstrumentPricingV2)

	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/instruments/"+instrumentID+"/pricing-v2", nil))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var resp struct {
		Code int                    `json:"code"`
		Data map[string]interface{} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, 20000, resp.Code)
	return numOrNil2528(resp.Data["overdue_daily_fee"])
}

// R1：列表 overdue_daily_fee（分）与 pricing-v2 同源；无定价 → null（不伪造 0）。
func TestInstrumentListOverdue2537_R1(t *testing.T) {
	db := testfixtures.SetupTestDB(t)
	testutil.SetDB(t, db)

	cents := func(v int64) *models.Cents { c := models.Cents(v); return &c }
	f := seedInstrument2528(t, db, []string{"A", "B", "C"}, map[string]func(*models.Instrument){
		// A: pricing JSONB 逾期费 36 元/天 → 列表与 pricing-v2 均 3600 分。
		"A": func(i *models.Instrument) {
			i.BaseDailyRate = cents(10000)
			i.Pricing = `{"overdue_daily_fee":36}`
		},
		// B: pricing_overrides 覆盖为 15 元/天 → 1500 分（override 优先）。
		"B": func(i *models.Instrument) {
			i.BaseDailyRate = cents(10000)
			i.PricingOverrides = `{"overdue_daily_fee":15}`
		},
		// C: 无基础日租 → 列表字段不可解析 → null。
		"C": func(i *models.Instrument) {
			i.BaseDailyRate = nil
			i.Pricing = `{}`
		},
	})

	rows := listInstruments2528(t, f)
	require.Len(t, rows, 3)

	a := rows["SN2528-A"]
	require.NotNil(t, a)
	listA, okA := numOrNil2528(a["overdue_daily_fee"])
	require.True(t, okA, "list must expose overdue_daily_fee for a priced instrument")
	v2A, okV2A := pricingV2Overdue2537(t, f, f.ids["A"])
	require.True(t, okV2A, "pricing-v2 must expose overdue_daily_fee")

	b := rows["SN2528-B"]
	require.NotNil(t, b)
	listB, okB := numOrNil2528(b["overdue_daily_fee"])
	require.True(t, okB, "list must expose overdue_daily_fee for an overridden instrument")

	c := rows["SN2528-C"]
	require.NotNil(t, c)

	fmt.Printf("LIST_OVERDUE_A=%d\n", listA)
	fmt.Printf("PRICING_V2_OVERDUE_A=%d\n", v2A)
	fmt.Printf("LIST_OVERDUE_B=%d\n", listB)
	if c["overdue_daily_fee"] == nil {
		fmt.Printf("LIST_OVERDUE_C=null\n")
	} else {
		cVal, _ := numOrNil2528(c["overdue_daily_fee"])
		fmt.Printf("LIST_OVERDUE_C=%d\n", cVal)
	}

	require.EqualValues(t, 3600, listA, "pricing.overdue_daily_fee=36 yuan → 3600 cents")
	require.EqualValues(t, listA, v2A, "list value must equal pricing-v2 value (single chain)")
	require.EqualValues(t, 1500, listB, "pricing_overrides.overdue_daily_fee=15 yuan → 1500 cents")
	require.Nil(t, c["overdue_daily_fee"], "unresolvable pricing must stay null, never a fake 0")
}
