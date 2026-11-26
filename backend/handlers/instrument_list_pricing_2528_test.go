package handlers

// #2528 库存监控列表显示缺陷：后端为列表新增 resolved_daily_rate / deposit /
// deposit_reduced（分），且必须与 pricing-v2 同源（显式押金优先、NULL 回落定价）。
//
// 证据行：R1（字段与空值边界）、R2（列表 ↔ pricing-v2 同源）。

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"tuneloop-backend/handlers/testfixtures"
	"tuneloop-backend/models"
	"tuneloop-backend/testutil"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

type instrument2528Fixture struct {
	tenantID   string
	categoryID string
	userID     string
	ids        map[string]string // key → instrument id
}

// seedInstrument2528 creates the category/user scaffolding plus one instrument
// per requested key, applying mutate for per-fixture pricing.
func seedInstrument2528(t *testing.T, db *gorm.DB, keys []string, mutate map[string]func(*models.Instrument)) instrument2528Fixture {
	t.Helper()
	f := instrument2528Fixture{
		tenantID:   uuid.New().String(),
		categoryID: uuid.New().String(),
		userID:     uuid.New().String(),
		ids:        map[string]string{},
	}
	now := time.Now()
	require.NoError(t, db.Exec(
		`INSERT INTO categories (id, name, tenant_id, level, visible, sort, created_at)
		 VALUES (?, 'Piano2528', ?, 1, true, 1, ?)`,
		f.categoryID, f.tenantID, now).Error)

	for _, key := range keys {
		id := uuid.New().String()
		inst := models.Instrument{
			ID:             id,
			TenantID:       f.tenantID,
			OrgID:          &f.tenantID,
			CategoryID:     &f.categoryID,
			SN:             "SN2528-" + key,
			StockStatus:    models.StockStatusAvailable,
			Images:         "[]",
			Specifications: "{}",
			Pricing:        "{}",
		}
		if fn, ok := mutate[key]; ok && fn != nil {
			fn(&inst)
		}
		require.NoError(t, db.Create(&inst).Error, "create instrument %s", key)
		f.ids[key] = id
	}
	return f
}

// listInstruments2528 calls GET /instruments and returns rows keyed by SN.
func listInstruments2528(t *testing.T, f instrument2528Fixture) map[string]map[string]interface{} {
	t.Helper()
	gin.SetMode(gin.TestMode)
	router := setupTestRouter(t, f.tenantID, f.userID)
	router.GET("/instruments", GetInstruments)

	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/instruments?pageSize=100", nil))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var resp struct {
		Code int `json:"code"`
		Data struct {
			List  []map[string]interface{} `json:"list"`
			Total int                      `json:"total"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, 20000, resp.Code)
	rows := map[string]map[string]interface{}{}
	for _, row := range resp.Data.List {
		if sn, ok := row["sn"].(string); ok {
			rows[sn] = row
		}
	}
	return rows
}

func numOrNil2528(v interface{}) (int64, bool) {
	switch val := v.(type) {
	case float64:
		return int64(val), true
	case int64:
		return val, true
	case nil:
		return 0, false
	default:
		return 0, false
	}
}

// R1：新增字段与空值边界（显式押金 / 显式零押金 / 无基础日租）。
func TestInstrumentListPricing2528_R1(t *testing.T) {
	db := testfixtures.SetupTestDB(t)
	testutil.SetDB(t, db)

	cents := func(v int64) *models.Cents { c := models.Cents(v); return &c }
	f := seedInstrument2528(t, db, []string{"F1", "F3", "F4"}, map[string]func(*models.Instrument){
		"F1": func(i *models.Instrument) {
			i.BaseDailyRate = cents(1000)
			i.Deposit = cents(50000)
		},
		"F3": func(i *models.Instrument) {
			i.BaseDailyRate = cents(1000)
			i.Deposit = cents(0)
			i.Pricing = `{"deposit":0}`
		},
		"F4": func(i *models.Instrument) {
			i.BaseDailyRate = nil
			i.Pricing = `{}`
			i.Deposit = nil
		},
	})

	rows := listInstruments2528(t, f)
	fmt.Printf("TOTAL=%d\n", len(rows))
	require.Len(t, rows, 3)

	f1 := rows["SN2528-F1"]
	require.NotNil(t, f1)
	f1Resolved, ok := numOrNil2528(f1["resolved_daily_rate"])
	require.True(t, ok, "resolved_daily_rate must be present")
	f1Deposit, ok := numOrNil2528(f1["deposit"])
	require.True(t, ok, "deposit must be present for explicit deposit")
	fmt.Printf("F1_RESOLVED=%d\n", f1Resolved)
	fmt.Printf("F1_DEPOSIT=%d\n", f1Deposit)
	require.EqualValues(t, 1000, f1Resolved, "resolved_daily_rate must be cents")
	require.EqualValues(t, 50000, f1Deposit, "explicit deposit must be passed through in cents")

	f3 := rows["SN2528-F3"]
	require.NotNil(t, f3)
	f3Deposit, ok := numOrNil2528(f3["deposit"])
	require.True(t, ok, "explicit zero deposit must be present (not null)")
	fmt.Printf("F3_DEPOSIT=%d\n", f3Deposit)
	fmt.Printf("F3_DEPOSIT_PRESENT=%d\n", boolToInt2528(f3["deposit"] != nil))
	require.EqualValues(t, 0, f3Deposit, "explicit zero deposit must stay 0")
	require.NotNil(t, f3["deposit"], "explicit zero deposit must not be dropped to null")

	f4 := rows["SN2528-F4"]
	require.NotNil(t, f4)
	f4Resolved, _ := numOrNil2528(f4["resolved_daily_rate"])
	fmt.Printf("F4_RESOLVED=%d\n", f4Resolved)
	require.EqualValues(t, 0, f4Resolved, "no base rate → resolved_daily_rate=0")
	require.Nil(t, f4["deposit"], "no base rate → deposit must be null (not fake 0)")
	fmt.Printf("F4_DEPOSIT=null\n")
}

func boolToInt2528(v bool) int {
	if v {
		return 1
	}
	return 0
}

// R2：列表字段 ↔ pricing-v2 同源（含 pricing_overrides 覆盖）。
func TestInstrumentListPricing2528_R2(t *testing.T) {
	db := testfixtures.SetupTestDB(t)
	testutil.SetDB(t, db)

	cents := func(v int64) *models.Cents { c := models.Cents(v); return &c }
	f := seedInstrument2528(t, db, []string{"F1", "F2", "F3", "F5"}, map[string]func(*models.Instrument){
		"F1": func(i *models.Instrument) {
			i.BaseDailyRate = cents(1000)
			i.Deposit = cents(50000)
		},
		"F2": func(i *models.Instrument) {
			i.BaseDailyRate = nil
			i.Pricing = `{"daily_rent":10,"deposit":30}`
			i.Deposit = nil
		},
		"F3": func(i *models.Instrument) {
			i.BaseDailyRate = cents(1000)
			i.Deposit = cents(0)
			i.Pricing = `{"deposit":0}`
		},
		"F5": func(i *models.Instrument) {
			i.BaseDailyRate = cents(1000)
			i.PricingOverrides = `{"daily_rent":20}`
		},
	})

	gin.SetMode(gin.TestMode)
	router := setupTestRouter(t, f.tenantID, f.userID)
	router.GET("/instruments", GetInstruments)
	router.GET("/instruments/:id/pricing-v2", GetInstrumentPricingV2)

	listRows := listInstruments2528(t, f)

	parityOK, mismatch := 0, 0
	for _, key := range []string{"F1", "F2", "F3", "F5"} {
		sn := "SN2528-" + key
		listRow := listRows[sn]
		require.NotNil(t, listRow, "list row %s missing", sn)

		w := httptest.NewRecorder()
		router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/instruments/"+f.ids[key]+"/pricing-v2", nil))
		require.Equal(t, http.StatusOK, w.Code, "pricing-v2 %s: %s", key, w.Body.String())

		var v2 struct {
			Data map[string]interface{} `json:"data"`
		}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &v2))

		listResolved, _ := numOrNil2528(listRow["resolved_daily_rate"])
		v2Resolved, _ := numOrNil2528(v2.Data["base_daily_rate"])
		listDeposit, listHasDeposit := numOrNil2528(listRow["deposit"])
		v2Deposit, v2HasDeposit := numOrNil2528(v2.Data["deposit"])
		listReduced, listHasReduced := numOrNil2528(listRow["deposit_reduced"])
		v2Reduced, v2HasReduced := numOrNil2528(v2.Data["deposit_reduced"])

		ok := listResolved == v2Resolved &&
			listHasDeposit == v2HasDeposit && listDeposit == v2Deposit &&
			listHasReduced == v2HasReduced && listReduced == v2Reduced

		if key == "F2" {
			fmt.Printf("F2_RESOLVED=%d\n", listResolved)
			fmt.Printf("F2_DEPOSIT=%d\n", listDeposit)
			require.EqualValues(t, 1000, listResolved, "F2 resolved from pricing.daily_rent (yuan→cents)")
			require.EqualValues(t, 3000, listDeposit, "F2 deposit from pricing.deposit (yuan→cents)")
		}
		if key == "F5" {
			fmt.Printf("F5_RESOLVED=%d\n", listResolved)
			require.EqualValues(t, 2000, listResolved, "F5 resolved honours pricing_overrides.daily_rent")
		}

		if ok {
			parityOK++
		} else {
			mismatch++
			t.Logf("PARITY_MISMATCH[%s]: list(resolved=%d deposit=%d/%v reduced=%d/%v) v2(resolved=%d deposit=%d/%v reduced=%d/%v)",
				key, listResolved, listDeposit, listHasDeposit, listReduced, listHasReduced,
				v2Resolved, v2Deposit, v2HasDeposit, v2Reduced, v2HasReduced)
		}
	}
	fmt.Printf("PARITY_OK=%d\n", parityOK)
	fmt.Printf("PARITY_MISMATCH=%d\n", mismatch)
	require.Equal(t, 4, parityOK)
	require.Zero(t, mismatch)
}
