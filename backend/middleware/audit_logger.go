package middleware

import (
	"bytes"
	"encoding/json"
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
	// ---- Forwarding sessions (7) ----
	"PUT /api/forwarding/sessions/:id/ship":      {"SHIP", "forwarding_session", "HIGH"},
	"PUT /api/forwarding/sessions/:id/receive":   {"RECEIVE", "forwarding_session", "HIGH"},
	"PUT /api/forwarding/sessions/:id/ready":     {"READY", "forwarding_session", "HIGH"},
	"PUT /api/forwarding/sessions/:id/last-mile": {"LAST_MILE", "forwarding_session", "HIGH"},
	"PUT /api/forwarding/sessions/:id/complete":  {"COMPLETE", "forwarding_session", "HIGH"},
	"PUT /api/forwarding/sessions/:id/lost":      {"LOST", "forwarding_session", "HIGH"},
	"PUT /api/forwarding/sessions/:id/recover":   {"RECOVER", "forwarding_session", "HIGH"},
	// ---- Instrument media ----
	"POST /api/instruments/:id/photos/upload":   {"UPLOAD", "instrument_photo", "HIGH"},
	"POST /api/instruments/:id/media":           {"CREATE", "instrument_media", "HIGH"},
	"PUT /api/instruments/:id/media/display":    {"UPDATE", "instrument_media", "HIGH"},
	"DELETE /api/instruments/:id/media/:batch_id": {"DELETE", "instrument_media", "HIGH"},
	"POST /api/instruments/:id/display-image":   {"UPLOAD", "instrument_display", "HIGH"},
	// ---- Instrument other ----
	"POST /api/instruments/:id/scrap":           {"SCRAP", "instrument", "CRITICAL"},
	"POST /api/instruments/batch-import/media":  {"IMPORT_MEDIA", "instrument", "HIGH"},
	"POST /api/instruments/batch-import/preview": {"PREVIEW", "instrument", "HIGH"},
	"PUT /api/instruments/batch-pricing":        {"BATCH_PRICING", "instrument", "HIGH"},
	// ---- Categories ----
	"POST /api/categories":    {"CREATE", "category", "HIGH"},
	"PUT /api/categories/:id": {"UPDATE", "category", "HIGH"},
	"DELETE /api/categories/:id": {"DELETE", "category", "HIGH"},
	"PUT /api/categories/sort": {"SORT", "category", "HIGH"},
	// ---- Properties ----
	"POST /api/property":      {"CREATE", "property", "HIGH"},
	"PUT /api/property/:id":   {"UPDATE", "property", "HIGH"},
	"POST /api/property/option": {"CREATE", "property_option", "HIGH"},
	"PUT /api/property/confirm": {"CONFIRM", "property", "HIGH"},
	// ---- Labels ----
	"POST /api/labels":               {"CREATE", "label", "HIGH"},
	"PUT /api/labels/:id/approve":    {"APPROVE", "label", "HIGH"},
	"PUT /api/labels/:id/reject":     {"REJECT", "label", "HIGH"},
	// ---- Maintenance additional ----
	"POST /api/maintenance/report":           {"REPORT", "maintenance_ticket", "HIGH"},
	"POST /api/maintenance/:id/record":       {"RECORD", "maintenance_ticket", "HIGH"},
	"PUT /api/maintenance/:id/status":        {"UPDATE_STATUS", "maintenance_ticket", "HIGH"},
	"PUT /api/maintenance/tickets/:id/status": {"UPDATE_STATUS", "maintenance_ticket", "HIGH"},
	"PUT /api/merchant/maintenance/:id/accept": {"ACCEPT", "maintenance_ticket", "HIGH"},
	"PUT /api/merchant/maintenance/:id/update": {"UPDATE", "maintenance_ticket", "HIGH"},
	// ---- Technician ----
	"POST /api/technician/tickets/:id/complete": {"COMPLETE", "maintenance_ticket", "HIGH"},
	"PUT /api/technician/tickets/:id/accept":    {"ACCEPT", "maintenance_ticket", "HIGH"},
	// ---- User addresses ----
	"POST /api/user/addresses":            {"CREATE", "address", "HIGH"},
	"PUT /api/user/addresses/:id":         {"UPDATE", "address", "HIGH"},
	"PUT /api/user/addresses/:id/default": {"SET_DEFAULT", "address", "HIGH"},
	"DELETE /api/user/addresses/:id":      {"DELETE", "address", "HIGH"},
	// ---- User self ----
	"PUT /api/users/me":                          {"UPDATE", "user_self", "HIGH"},
	"POST /api/user/change-password":             {"CHANGE_PASSWORD", "user_self", "HIGH"},
	"POST /api/user/reset-password":              {"RESET_PASSWORD", "user_self", "HIGH"},
	"POST /api/users/:id/activate":              {"ACTIVATE", "user", "CRITICAL"},
	"POST /api/users/reset-password":            {"RESET_PASSWORD", "user", "HIGH"},
	"POST /api/users/me/resend-email-confirmation": {"RESEND_EMAIL", "user_self", "LOW"},
	// ---- Appeals additional ----
	"POST /api/appeals":            {"CREATE", "appeal", "HIGH"},
	"POST /api/appeals/:id/agree": {"AGREE", "appeal", "HIGH"},
	// ---- Banner management ----
	"POST /api/admin/banners":    {"CREATE", "banner", "HIGH"},
	"PUT /api/admin/banners/:id": {"UPDATE", "banner", "HIGH"},
	"DELETE /api/admin/banners/:id": {"DELETE", "banner", "HIGH"},
	// ---- Permission management ----
	"PUT /api/admin/users/:id/permissions": {"UPDATE", "user_permission", "CRITICAL"},
	"PUT /api/admin/users/:id/roles":       {"UPDATE", "user_role", "CRITICAL"},
	"PUT /api/admin/roles/:id":             {"UPDATE", "role", "CRITICAL"},
	// ---- Confirmation sessions ----
	"POST /api/confirmation-sessions":          {"CREATE", "confirmation", "HIGH"},
	"POST /api/confirmation-sessions/:id/reject": {"REJECT", "confirmation", "HIGH"},
	// ---- Other ----
	"POST /api/user/orders/batch":        {"BATCH_CREATE", "user_order", "HIGH"},
	"PUT /api/pricing/merchant-config":   {"UPDATE", "pricing_config", "HIGH"},
	"PUT /api/settings/:key":             {"UPDATE", "setting", "HIGH"},
	"POST /api/upload":                   {"UPLOAD", "file", "HIGH"},
	"POST /api/notifications/mark-all-read": {"MARK_READ", "notification", "LOW"},
	"POST /api/notifications/:id/read":    {"MARK_READ", "notification", "LOW"},
}

const maxRequestBodySize = 10 * 1024

type responseBodyWriter struct {
	gin.ResponseWriter
	body *bytes.Buffer
}

func (w *responseBodyWriter) Write(b []byte) (int, error) {
	w.body.Write(b)
	return w.ResponseWriter.Write(b)
}

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

		// wrap response writer to capture body
		resBody := &responseBodyWriter{ResponseWriter: c.Writer, body: &bytes.Buffer{}}
		c.Writer = resBody

		c.Next()

		statusCode := c.Writer.Status()
		status := "success"
		if statusCode >= 400 {
			status = "failure"
		}

		var errMsg string
		if resBody.body.Len() > 0 {
			var respBody struct {
				Message string `json:"message"`
			}
			if err := json.Unmarshal(resBody.body.Bytes(), &respBody); err == nil && respBody.Message != "" {
				errMsg = respBody.Message
			}
		}

		ipAddress := c.GetHeader("X-Forwarded-For")
		if ipAddress == "" {
			ipAddress = c.GetHeader("X-Real-IP")
		}
		if ipAddress == "" {
			ipAddress = c.ClientIP()
		}
		actorName := GetName(ctx)

		rec := &services.AuditRecord{
			TenantID:     tenantID,
			OrgID:        orgIDPtr,
			UserID:       userID,
			ActorRole:    GetRole(ctx),
			Action:       info.Action,
			ResourceType: info.ResourceType,
			ResourceID:   resourceID,
			StatusCode:   statusCode,
			Status:       status,
			ErrorMessage: errMsg,
			Details:      "",
			RequestBody:  bodyStr,
			ActorName:    actorName,
			IPAddress:    ipAddress,
			UserAgent:    c.GetHeader("User-Agent"),
		}

		if err := writer.WriteSync(rec); err != nil {
			log.Printf("[CRITICAL] Audit log save failed: %v", err)
		}
	}
}
