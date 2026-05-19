package middleware

import (
	"bytes"
	"io"
	"log"
	"net/http"

	"tuneloop-backend/services"

	"github.com/gin-gonic/gin"
)

type auditRouteInfo struct {
	Action       string
	ResourceType string
	Priority     string
}

var auditRouteMap = map[string]auditRouteInfo{
	"POST /api/users":                          {"CREATE", "user", "CRITICAL"},
	"PUT /api/users/:id":                       {"UPDATE", "user", "CRITICAL"},
	"DELETE /api/users/batch":                  {"DELETE", "user", "CRITICAL"},
	"POST /api/iam/users":                      {"CREATE", "iam_user", "CRITICAL"},
	"PUT /api/iam/users/:id":                   {"UPDATE", "iam_user", "CRITICAL"},
	"POST /api/iam/users/:user_id/invite":      {"INVITE", "user", "CRITICAL"},
	"POST /api/merchants":                      {"CREATE", "merchant", "CRITICAL"},
	"PUT /api/merchants/:id":                   {"UPDATE", "merchant", "CRITICAL"},
	"DELETE /api/merchants/:id":                {"DELETE", "merchant", "CRITICAL"},
	"POST /api/merchant/sites":                 {"CREATE", "site", "CRITICAL"},
	"PUT /api/merchant/sites/:id":              {"UPDATE", "site", "CRITICAL"},
	"DELETE /api/merchant/sites/:id":           {"DELETE", "site", "CRITICAL"},
	"POST /api/sites/:id/members":              {"CREATE", "site_member", "CRITICAL"},
	"PUT /api/sites/:id/members/:uid":          {"UPDATE", "site_member", "CRITICAL"},
	"DELETE /api/sites/:id/members/:uid":       {"DELETE", "site_member", "CRITICAL"},
	"PUT /api/admin/roles/:id/permissions":     {"UPDATE", "role_permission", "CRITICAL"},
	"POST /api/admin/roles":                    {"CREATE", "role", "CRITICAL"},
	"DELETE /api/admin/roles/:id":              {"DELETE", "role", "CRITICAL"},
	"POST /api/orders":                         {"CREATE", "order", "CRITICAL"},
	"POST /api/orders/:id/pay":                 {"PAY", "order", "CRITICAL"},
	"POST /api/orders/:id/pickup":              {"PICKUP", "order", "CRITICAL"},
	"POST /api/orders/:id/return":              {"RETURN", "order", "CRITICAL"},
	"POST /api/orders/:id/cancel":              {"CANCEL", "order", "CRITICAL"},
	"POST /api/orders/:id/transfer-ownership":  {"TRANSFER_OWNERSHIP", "order", "CRITICAL"},
	"PUT /api/orders/:id/terminate":            {"TERMINATE", "order", "CRITICAL"},
	"PUT /api/warehouse/orders/:id/delivery":   {"DELIVERY", "order", "CRITICAL"},
	"PUT /api/warehouse/orders/:id/return-inspect": {"RETURN_INSPECT", "order", "CRITICAL"},
	"PUT /api/warehouse/orders/:id/damage":     {"DAMAGE", "order", "CRITICAL"},
	"PUT /api/warehouse/orders/:id/shipping":   {"SHIPPING", "order", "CRITICAL"},
	"POST /api/merchant/leases":                {"CREATE", "lease", "CRITICAL"},
	"PUT /api/merchant/leases/:id":             {"UPDATE", "lease", "CRITICAL"},
	"DELETE /api/merchant/leases/:id":          {"TERMINATE", "lease", "CRITICAL"},
	"POST /api/merchant/deposits":              {"CREATE", "deposit", "CRITICAL"},
	"PUT /api/merchant/deposits/:id":           {"UPDATE", "deposit", "CRITICAL"},
	"POST /api/merchant/inventory/transfer":    {"TRANSFER", "inventory", "CRITICAL"},
	"PUT /api/inventory/rent-setting/batch":    {"BATCH_UPDATE", "rent_setting", "CRITICAL"},
	"PUT /api/appeals/:id/resolve":             {"RESOLVE", "appeal", "CRITICAL"},
	"POST /api/setup/init":                     {"INIT", "system", "CRITICAL"},
	"POST /api/confirmation-sessions/:id/confirm": {"CONFIRM", "confirmation", "CRITICAL"},
	"POST /api/admin/bulk-import/organizations": {"IMPORT", "organization", "CRITICAL"},
	"POST /api/admin/bulk-import/accounts":     {"IMPORT", "account", "CRITICAL"},
	"POST /api/iam/organizations/sync":         {"SYNC", "organization", "CRITICAL"},
	"POST /api/iam/users/sync":                 {"SYNC", "user", "CRITICAL"},
	"POST /api/instruments":                    {"CREATE", "instrument", "HIGH"},
	"PUT /api/instruments/:id":                 {"UPDATE", "instrument", "HIGH"},
	"DELETE /api/instruments/:id":              {"DELETE", "instrument", "HIGH"},
	"PUT /api/instruments/:id/status":          {"UPDATE_STATUS", "instrument", "HIGH"},
	"POST /api/instruments/import":             {"IMPORT", "instrument", "HIGH"},
	"POST /api/instruments/batch-import":       {"BATCH_IMPORT", "instrument", "HIGH"},
	"POST /api/maintenance":                    {"CREATE", "maintenance_ticket", "HIGH"},
	"PUT /api/maintenance/:id/cancel":          {"CANCEL", "maintenance_ticket", "HIGH"},
	"PUT /api/merchant/maintenance/:id/assign": {"ASSIGN", "maintenance_ticket", "HIGH"},
	"POST /api/merchant/maintenance/:id/quote": {"QUOTE", "maintenance_ticket", "HIGH"},
	"POST /api/maintenance/:id/start":          {"START", "maintenance_ticket", "HIGH"},
	"POST /api/maintenance/:id/inspect":        {"INSPECT", "maintenance_ticket", "HIGH"},
	"POST /api/orders/:id/assessment":          {"SUBMIT", "assessment", "HIGH"},
	"POST /api/orders/:id/outbound-confirm":    {"CONFIRM", "outbound", "HIGH"},
	"POST /api/user/orders":                    {"CREATE", "user_order", "HIGH"},
	"POST /api/user/rentals/:id/return":        {"RETURN", "user_rental", "HIGH"},
	"POST /api/maintenance/workers":            {"CREATE", "maintenance_worker", "HIGH"},
	"DELETE /api/maintenance/workers/:id":      {"DELETE", "maintenance_worker", "HIGH"},
	"PUT /api/property/merge":                  {"MERGE", "property", "HIGH"},
	"POST /api/labels/merge":                   {"MERGE", "label", "HIGH"},
	"PUT /api/merchant/merchants/:id/status":   {"UPDATE_STATUS", "merchant", "HIGH"},
}

const maxRequestBodySize = 10 * 1024

func AuditLogger(writer *services.AuditWriter) gin.HandlerFunc {
	return func(c *gin.Context) {
		method := c.Request.Method
		path := c.FullPath()
		key := method + " " + path

		info, ok := auditRouteMap[key]
		if !ok {
			c.Next()
			return
		}

		ctx := c.Request.Context()
		tenantID := GetTenantID(ctx)
		if tenantID == "" {
			c.Next()
			return
		}

		userID := GetUserID(ctx)
		if userID == "" {
			c.Next()
			return
		}

		orgID := GetOrgID(ctx)
		var orgIDPtr *string
		if orgID != "" {
			orgIDPtr = &orgID
		}

		resourceID := c.Param("id")
		if resourceID == "" {
			resourceID = c.Param("user_id")
		}

		var bodyStr string
		if info.Priority == "CRITICAL" && c.Request.Body != nil && c.Request.ContentLength > 0 {
			bodyBytes, err := io.ReadAll(c.Request.Body)
			if err != nil {
				c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{
					"code": 40001, "message": "cannot read request body",
				})
				return
			}
			c.Request.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
			if len(bodyBytes) > 0 {
				if len(bodyBytes) > maxRequestBodySize {
					bodyBytes = bodyBytes[:maxRequestBodySize]
				}
				bodyStr = string(bodyBytes)
			}
		}

		rec := &services.AuditRecord{
			TenantID:     tenantID,
			OrgID:        orgIDPtr,
			UserID:       userID,
			ActorRole:    GetRole(ctx),
			Action:       info.Action,
			ResourceType: info.ResourceType,
			ResourceID:   resourceID,
			Details:      "",
			RequestBody:  bodyStr,
			IPAddress:    c.ClientIP(),
			UserAgent:    c.GetHeader("User-Agent"),
		}

		if err := writer.WriteSync(rec); err != nil {
			log.Printf("[CRITICAL] Audit log save failed, rejecting request: %v", err)
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
				"code":    50000,
				"message": "internal error",
			})
			return
		}

		c.Next()
	}
}
