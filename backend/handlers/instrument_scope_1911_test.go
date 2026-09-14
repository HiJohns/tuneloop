package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"tuneloop-backend/handlers/testfixtures"
	"tuneloop-backend/models"
	"tuneloop-backend/testutil"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// #1911: a technician belonging to multiple sites sees instruments from all
// of them; removing a membership hides that site again.
func TestGetInstruments_MultiSiteTechnician(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := testfixtures.SetupTestDB(t)
	tenantID, orgA, _ := testfixtures.NewTenantIDs("1911a1b2c3d4")
	orgB := uuid.New().String()
	siteA := uuid.New().String()
	siteB := uuid.New().String()
	require.NoError(t, db.Create(&models.Site{ID: siteA, TenantID: tenantID, OrgID: orgA, Name: "站点A"}).Error)
	require.NoError(t, db.Create(&models.Site{ID: siteB, TenantID: tenantID, OrgID: orgB, Name: "站点B"}).Error)

	mkInstrument := func(sn, org, site string) {
		siteUUID := uuid.MustParse(site)
		require.NoError(t, db.Create(&models.Instrument{
			ID: uuid.New().String(), TenantID: tenantID, OrgID: &org,
			SiteID: &siteUUID, SN: sn, StockStatus: "available",
		}).Error)
	}
	mkInstrument("1911-A", orgA, siteA)
	mkInstrument("1911-B", orgB, siteB)

	techID := uuid.New().String()
	require.NoError(t, db.Create(&models.User{
		ID: techID, IAMSub: techID, TenantID: tenantID, OrgID: orgA,
		Username: "tech1911", Name: "技师", Role: "repair_technician", Status: "active",
	}).Error)
	mkMembership := func(site string) {
		require.NoError(t, db.Create(&models.SiteMember{
			TenantID: tenantID, SiteID: site, UserID: techID,
			Role: "repair_technician", Status: "active",
		}).Error)
	}
	mkMembership(siteA)
	mkMembership(siteB)

	router := gin.New()
	router.Use(func(c *gin.Context) {
		actor := testutil.TestActor{
			TenantID: tenantID, OrgID: orgA, UserID: techID,
			Role: "repair_technician", SysPerm: -1, CusPerm: -1,
		}
		c.Request = c.Request.WithContext(actor.InjectContext(c.Request.Context()))
		c.Next()
	})
	router.GET("/instruments", GetInstruments)

	listSNs := func() []string {
		req := httptest.NewRequest(http.MethodGet, "/instruments?page=1&pageSize=50", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		require.Equal(t, http.StatusOK, w.Code)
		var resp struct {
			Code int `json:"code"`
			Data struct {
				List []struct {
					SN string `json:"sn"`
				} `json:"list"`
			} `json:"data"`
		}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		require.Equal(t, 20000, resp.Code)
		sns := []string{}
		for _, it := range resp.Data.List {
			sns = append(sns, it.SN)
		}
		return sns
	}

	assert.ElementsMatch(t, []string{"1911-A", "1911-B"}, listSNs(), "multi-site technician sees both sites")

	require.NoError(t, db.Where("site_id = ? AND user_id = ?", siteB, techID).Delete(&models.SiteMember{}).Error)
	assert.ElementsMatch(t, []string{"1911-A"}, listSNs(), "removed membership is no longer visible")
}
