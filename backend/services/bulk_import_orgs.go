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
	RowNum            int
	OrganizationCode  string
	Name              string
	ParentCode        string
	Type              string
}

// ImportOrganizationsCSV imports organizations from a CSV file.
func ImportOrganizationsCSV(ctx context.Context, r io.Reader, tenantID string, iamClient *IAMClient, dryRun bool) (*BulkImportResult, error) {
	_, records, err := ParseCSV(r)
	if err != nil {
		return nil, err
	}

	// Parse records
	var orgRecords []OrgImportRecord
	for _, rec := range records {
		orgRecords = append(orgRecords, OrgImportRecord{
			RowNum:           rec.RowNum,
			OrganizationCode: strings.TrimSpace(rec.Fields["organization_code"]),
			Name:             strings.TrimSpace(rec.Fields["name"]),
			ParentCode:       strings.TrimSpace(rec.Fields["parent_code"]),
			Type:             strings.TrimSpace(rec.Fields["type"]),
		})
	}

	// Deduplicate by organization_code (keep last)
	seen := make(map[string]int)
	for i, o := range orgRecords {
		if o.OrganizationCode != "" {
			seen[o.OrganizationCode] = i
		}
	}
	var deduped []OrgImportRecord
	for i, o := range orgRecords {
		if o.OrganizationCode == "" || seen[o.OrganizationCode] == i {
			deduped = append(deduped, o)
		}
	}
	orgRecords = deduped

	// Build parent dependency graph and topological sort
	sorted, err := topoSortOrgs(orgRecords)
	if err != nil {
		return nil, fmt.Errorf("failed to sort organizations by dependency: %w", err)
	}

	db := database.GetDB().WithContext(ctx)
	result := &BulkImportResult{}
	result.Summary.Total = len(sorted)

	// Preload existing sites by organization_code and name
	// OrganizationCode stores CSV code (e.g., TUNELOOP), OrgID is UUID from IAM
	var existingSites []models.Site
	db.Where("tenant_id = ?", tenantID).Find(&existingSites)
	existingByCode := make(map[string]models.Site)
	existingByName := make(map[string]models.Site)
	for _, s := range existingSites {
		if s.OrganizationCode != "" {
			existingByCode[s.OrganizationCode] = s
		}
		if s.Name != "" {
			existingByName[s.Name] = s
		}
	}

	// Map to track organization_code -> local site_id (for parent resolution)
	codeToSiteID := make(map[string]string)
	codeToOrgID := make(map[string]string)
	for _, s := range existingSites {
		if s.OrganizationCode != "" {
			codeToSiteID[s.OrganizationCode] = s.ID
			codeToOrgID[s.OrganizationCode] = s.OrgID
		}
	}

	for _, org := range sorted {
		// Validate required fields
		if org.OrganizationCode == "" {
			result.Summary.Failed++
			result.Details = append(result.Details, BulkImportDetail{
				Row:    org.RowNum,
				Key:    "",
				Action: "failed",
				Reason: "organization_code is required",
			})
			continue
		}
		if org.Name == "" {
			result.Summary.Failed++
			result.Details = append(result.Details, BulkImportDetail{
				Row:    org.RowNum,
				Key:    org.OrganizationCode,
				Action: "failed",
				Reason: "name is required",
			})
			continue
		}

		existingSite, exists := existingByCode[org.OrganizationCode]
		if !exists {
			existingSite, exists = existingByName[org.Name]
		}
		if !exists {
			lowerCode := strings.ToLower(org.OrganizationCode)
			lowerName := strings.ToLower(org.Name)
			for _, s := range existingSites {
				if (s.OrganizationCode != "" && strings.ToLower(s.OrganizationCode) == lowerCode) ||
					(s.Name != "" && strings.ToLower(s.Name) == lowerName) {
					existingSite = s
					exists = true
					break
				}
			}
		}

		if dryRun {
			if exists {
				result.Summary.Updated++
				result.Details = append(result.Details, BulkImportDetail{
					Row:    org.RowNum,
					Key:    org.OrganizationCode,
					Action: "updated",
					Reason: "organization exists, will update",
				})
			} else {
				result.Summary.Created++
				result.Details = append(result.Details, BulkImportDetail{
					Row:    org.RowNum,
					Key:    org.OrganizationCode,
					Action: "created",
					Reason: "new organization",
				})
			}
			continue
		}

		// Resolve parent
		var parentID *uuid.UUID
		if org.ParentCode != "" {
			parentSiteID, ok := codeToSiteID[org.ParentCode]
			if !ok {
// Try to find in DB by organization_code
			var parentSite models.Site
			if err := db.Where("organization_code = ? AND tenant_id = ?", org.ParentCode, tenantID).First(&parentSite).Error; err == nil {
					parentSiteID = parentSite.ID
					codeToSiteID[org.ParentCode] = parentSiteID
					codeToOrgID[org.ParentCode] = parentSite.OrgID
				}
			}
			if parentSiteID != "" {
				pid, err := uuid.Parse(parentSiteID)
				if err == nil {
					parentID = &pid
				}
			}
		}

		if exists {
			// Update existing site
			updates := map[string]interface{}{
				"name": org.Name,
			}
			if org.Type != "" {
				updates["type"] = org.Type
			}
			if parentID != nil {
				updates["parent_id"] = parentID
			}
			if err := db.Model(&existingSite).Updates(updates).Error; err != nil {
				LogImportError("organization", org.RowNum, org.OrganizationCode, err)
				result.Summary.Failed++
				result.Details = append(result.Details, BulkImportDetail{
					Row:    org.RowNum,
					Key:    org.OrganizationCode,
					Action: "failed",
					Reason: fmt.Sprintf("update failed: %v", err),
				})
				continue
			}
			result.Summary.Updated++
			result.Details = append(result.Details, BulkImportDetail{
				Row:    org.RowNum,
				Key:    org.OrganizationCode,
				Action: "updated",
			})
		} else {
			// Create new site and IAM organization
			orgID := uuid.New().String()
			newSite := models.Site{
				ID:                uuid.New().String(),
				TenantID:          tenantID,
				OrgID:             orgID, // Valid UUID
				OrganizationCode: org.OrganizationCode, // CSV code
				Name:              org.Name,
				Type:              org.Type,
				Status:            "active",
			}
			if parentID != nil {
				newSite.ParentID = parentID
			}

			if err := db.Create(&newSite).Error; err != nil {
				LogImportError("organization", org.RowNum, org.OrganizationCode, err)
				result.Summary.Failed++
				result.Details = append(result.Details, BulkImportDetail{
					Row:    org.RowNum,
					Key:    org.OrganizationCode,
					Action: "failed",
					Reason: fmt.Sprintf("create failed: %v", err),
				})
				continue
			}

			// Also create in IAM (best effort)
			iamReq := &CreateOrganizationRequest{
				Name:        org.Name,
				NamespaceID: iamClient.GetNamespace(),
			}
			if parentID != nil {
				if parentOrgID, ok := codeToOrgID[org.ParentCode]; ok && parentOrgID != "" {
					iamReq.ParentID = parentOrgID
				}
			}
			if _, err := iamClient.CreateOrganization(iamReq); err != nil {
				log.Printf("[BulkImport] IAM CreateOrganization warning for %s: %v", org.OrganizationCode, err)
				// Non-fatal: local site is created, IAM sync can be retried
			}

			codeToSiteID[org.OrganizationCode] = newSite.ID
			codeToOrgID[org.OrganizationCode] = newSite.OrgID

			result.Summary.Created++
			result.Details = append(result.Details, BulkImportDetail{
				Row:    org.RowNum,
				Key:    org.OrganizationCode,
				Action: "created",
			})
		}
	}

	return result, nil
}

// topoSortOrgs sorts organizations so parents are created before children.
func topoSortOrgs(orgs []OrgImportRecord) ([]OrgImportRecord, error) {
	// Build maps
	codeToOrg := make(map[string]OrgImportRecord)
	children := make(map[string][]string) // parent_code -> []child_code
	inDegree := make(map[string]int)

	for _, o := range orgs {
		codeToOrg[o.OrganizationCode] = o
		inDegree[o.OrganizationCode] = 0
	}

	for _, o := range orgs {
		if o.ParentCode != "" {
			if _, ok := codeToOrg[o.ParentCode]; ok {
				children[o.ParentCode] = append(children[o.ParentCode], o.OrganizationCode)
				inDegree[o.OrganizationCode]++
			}
		}
	}

	// Kahn's algorithm
	var queue []string
	for code, deg := range inDegree {
		if deg == 0 {
			queue = append(queue, code)
		}
	}

	var sorted []OrgImportRecord
	for len(queue) > 0 {
		code := queue[0]
		queue = queue[1:]
		sorted = append(sorted, codeToOrg[code])
		for _, child := range children[code] {
			inDegree[child]--
			if inDegree[child] == 0 {
				queue = append(queue, child)
			}
		}
	}

	if len(sorted) != len(orgs) {
		return nil, fmt.Errorf("circular dependency detected in organization hierarchy")
	}

	return sorted, nil
}
