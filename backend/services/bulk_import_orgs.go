package services

import (
	"context"
	"fmt"
	"io"
	"log"
	"strings"
	"tuneloop-backend/database"
	"tuneloop-backend/models"

	"github.com/google/uuid"
)

// OrgImportRecord represents a parsed organization import row.
type OrgImportRecord struct {
	RowNum     int
	Name       string
	Type       string
	ParentName string
	Address    string
	Phone      string
}

// ImportOrganizationsCSV imports organizations from a CSV file.
func ImportOrganizationsCSV(ctx context.Context, r io.Reader, tenantID string, iamClient *IAMClient, dryRun bool, allowMerchant bool) (*BulkImportResult, error) {
	_, records, err := ParseCSV(r)
	if err != nil {
		return nil, err
	}

	// Parse records
	var orgRecords []OrgImportRecord
	for _, rec := range records {
		orgRecords = append(orgRecords, OrgImportRecord{
			RowNum:     rec.RowNum,
			Name:       strings.TrimSpace(rec.Fields["name"]),
			Type:       strings.TrimSpace(rec.Fields["type"]),
			ParentName: strings.TrimSpace(rec.Fields["parent_name"]),
			Address:    strings.TrimSpace(rec.Fields["address"]),
			Phone:      strings.TrimSpace(rec.Fields["phone"]),
		})
	}

	// Reject merchant type for non-namespace-admin users
	if !allowMerchant {
		for _, org := range orgRecords {
			if org.Type == "merchant" {
				return nil, fmt.Errorf("row %d: type 'merchant' is not allowed for your role (only namespace administrators can import merchants)", org.RowNum)
			}
		}
	}

	// Deduplicate by name (keep last)
	seen := make(map[string]int)
	for i, o := range orgRecords {
		if o.Name != "" {
			seen[o.Name] = i
		}
	}
	var deduped []OrgImportRecord
	for i, o := range orgRecords {
		if o.Name == "" || seen[o.Name] == i {
			deduped = append(deduped, o)
		}
	}
	orgRecords = deduped

	db := database.GetDB().WithContext(ctx)
	result := &BulkImportResult{}
	result.Summary.Total = len(orgRecords)

	// Preload existing sites by name
	var existingSites []models.Site
	db.Where("tenant_id = ? AND deleted_at IS NULL", tenantID).Find(&existingSites)
	existingByName := make(map[string]models.Site)
	for _, s := range existingSites {
		if s.Name != "" {
			existingByName[s.Name] = s
		}
	}

	for _, org := range orgRecords {
		if org.Name == "" {
			result.Summary.Failed++
			result.Details = append(result.Details, BulkImportDetail{
				Row:    org.RowNum,
				Key:    "",
				Action: "failed",
				Reason: "name is required",
			})
			continue
		}

		_, exists := existingByName[org.Name]
		if !exists {
			lower := strings.ToLower(org.Name)
			for _, s := range existingSites {
				if s.Name != "" && strings.ToLower(s.Name) == lower {
					exists = true
					break
				}
			}
		}

		if dryRun {
			if exists {
				result.Summary.Skipped++
				result.Details = append(result.Details, BulkImportDetail{
					Row:    org.RowNum,
					Key:    org.Name,
					Action: "skipped",
					Reason: "name exists, skipped (create-only)",
				})
			} else {
				result.Summary.Created++
				result.Details = append(result.Details, BulkImportDetail{
					Row:    org.RowNum,
					Key:    org.Name,
					Action: "created",
					Reason: "new organization",
				})
			}
			continue
		}

		if exists {
			result.Summary.Skipped++
			result.Details = append(result.Details, BulkImportDetail{
				Row:    org.RowNum,
				Key:    org.Name,
				Action: "skipped",
				Reason: "name already exists, skipped (create-only)",
			})
			continue
		}

		orgID := uuid.New().String()
		newSite := models.Site{
			ID:       uuid.New().String(),
			TenantID: tenantID,
			OrgID:    orgID,
			Name:     org.Name,
			Type:     org.Type,
			Address:  org.Address,
			Phone:    org.Phone,
			Status:   "active",
		}

		if err := db.Create(&newSite).Error; err != nil {
			LogImportError("organization", org.RowNum, org.Name, err)
			result.Summary.Failed++
			result.Details = append(result.Details, BulkImportDetail{
				Row:    org.RowNum,
				Key:    org.Name,
				Action: "failed",
				Reason: fmt.Sprintf("create failed: %v", err),
			})
			continue
		}

		iamReq := &CreateOrganizationRequest{
			Name:        org.Name,
			NamespaceID: iamClient.GetNamespace(),
		}
		if _, err := iamClient.CreateOrganization(iamReq); err != nil {
			log.Printf("[BulkImport] IAM CreateOrganization warning for %s: %v", org.Name, err)
		}

		result.Summary.Created++
		result.Details = append(result.Details, BulkImportDetail{
			Row:    org.RowNum,
			Key:    org.Name,
			Action: "created",
		})
	}

	return result, nil
}
