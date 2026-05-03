package handlers

import (
	"net/http"
	"strings"
	"tuneloop-backend/middleware"
	"tuneloop-backend/services"

	"github.com/gin-gonic/gin"
)

// BulkImportHandler handles batch import requests for organizations and accounts.
type BulkImportHandler struct {
	iamClient  *services.IAMClient
	permReg    *services.PermissionRegistry
}

// NewBulkImportHandler creates a new bulk import handler.
func NewBulkImportHandler(iamClient *services.IAMClient, permReg *services.PermissionRegistry) *BulkImportHandler {
	return &BulkImportHandler{
		iamClient: iamClient,
		permReg:   permReg,
	}
}

// ImportOrganizations handles CSV upload for bulk organization/site import.
// POST /api/admin/bulk-import/organizations
func (h *BulkImportHandler) ImportOrganizations(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID := middleware.GetTenantID(ctx)
	if tenantID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40001, "message": "Tenant ID is required"})
		return
	}

	file, header, err := c.Request.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "File upload failed: " + err.Error()})
		return
	}
	defer file.Close()

	if !strings.HasSuffix(strings.ToLower(header.Filename), ".csv") {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40003, "message": "Only CSV files (.csv) are supported"})
		return
	}

	dryRun := c.Query("dry_run") == "true"

	result, err := services.ImportOrganizationsCSV(ctx, file, tenantID, h.iamClient, dryRun)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40004, "message": "Import failed: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": 20000, "data": result})
}

// ImportAccounts handles CSV upload for bulk account/user import.
// POST /api/admin/bulk-import/accounts
func (h *BulkImportHandler) ImportAccounts(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID := middleware.GetTenantID(ctx)
	if tenantID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40001, "message": "Tenant ID is required"})
		return
	}

	file, header, err := c.Request.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40002, "message": "File upload failed: " + err.Error()})
		return
	}
	defer file.Close()

	if !strings.HasSuffix(strings.ToLower(header.Filename), ".csv") {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40003, "message": "Only CSV files (.csv) are supported"})
		return
	}

	dryRun := c.Query("dry_run") == "true"

	result, err := services.ImportAccountsCSV(ctx, file, tenantID, h.iamClient, h.permReg, dryRun)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40004, "message": "Import failed: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": 20000, "data": result})
}

// DownloadOrganizationTemplate serves the organization CSV template.
// GET /api/admin/bulk-import/template/organizations
func (h *BulkImportHandler) DownloadOrganizationTemplate(c *gin.Context) {
	c.Header("Content-Type", "text/csv; charset=utf-8")
	c.Header("Content-Disposition", "attachment; filename=\"bulk_sites_template.csv\"")

	headers := []string{"organization_code", "name", "parent_code", "type"}
	sampleRows := [][]string{
		{"TUNELOOP", "Tuneloop 商户", "", "merchant"},
		{"TUNELOOP_HD", "海淀网点", "TUNELOOP", "site"},
	}

	if err := services.WriteCSVTemplate(c.Writer, headers, sampleRows); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50001, "message": "Failed to generate template: " + err.Error()})
	}
}

// DownloadAccountTemplate serves the account CSV template.
// GET /api/admin/bulk-import/template/accounts
func (h *BulkImportHandler) DownloadAccountTemplate(c *gin.Context) {
	c.Header("Content-Type", "text/csv; charset=utf-8")
	c.Header("Content-Disposition", "attachment; filename=\"bulk_accounts_template.csv\"")

	headers := []string{"email", "name", "role_template", "organization_code", "phone", "tags"}
	sampleRows := [][]string{
		{"admin@tuneloop.com", "系统管理员", "namespace_admin", "", "", ""},
		{"admin_debug@tuneloop.com", "调试管理员", "merchant_admin", "TUNELOOP", "", ""},
		{"tech_zhang@tuneloop.com", "张工", "site_member", "TUNELOOP", "", "IT"},
		{"haidian_admin@tuneloop.com", "海淀管理员", "site_admin", "TUNELOOP_HD", "", ""},
		{"haidian_staff@tuneloop.com", "海淀员工", "site_member", "TUNELOOP_HD", "", ""},
		{"haidian_engineer@tuneloop.com", "海淀维修师傅", "site_member", "TUNELOOP_HD", "", "engineer"},
		{"customer_lee@tuneloop.com", "顾客小李", "customer", "", "", ""},
	}

	if err := services.WriteCSVTemplate(c.Writer, headers, sampleRows); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50001, "message": "Failed to generate template: " + err.Error()})
	}
}
