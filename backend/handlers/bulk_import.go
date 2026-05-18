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

	allowMerchant := middleware.GetCusPerm(ctx) == 0

	result, err := services.ImportOrganizationsCSV(ctx, file, tenantID, h.iamClient, dryRun, allowMerchant)
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

	headers := []string{"name", "type", "parent_name", "address", "phone"}
	sampleRows := [][]string{
		{"海淀网点", "直营店", "-", "北京市海淀区中关村大街1号", "010-12345678"},
		{"海淀分店A", "加盟店", "海淀网点", "北京市海淀区学院路5号", "010-87654321"},
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

	headers := []string{"username", "name", "email", "phone", "site"}
	sampleRows := [][]string{
		{"admin_debug", "调试管理员", "admin_debug@tuneloop.com", "13800000001", "TUNELOOP"},
		{"staff_zhang", "张工", "zhang@tuneloop.com", "13800000002", "TUNELOOP_HD"},
	}

	if err := services.WriteCSVTemplate(c.Writer, headers, sampleRows); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50001, "message": "Failed to generate template: " + err.Error()})
	}
}
