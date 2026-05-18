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

type siteInfo struct {
	ID    string
	OrgID string
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

	// Topological sort by parent_name so parents are processed before children
	sorted, err := topoSortOrgsByName(orgRecords)
	if err != nil {
		return nil, fmt.Errorf("failed to sort organizations by parent dependency: %w", err)
	}

	db := database.GetDB().WithContext(ctx)
	result := &BulkImportResult{}
	result.Summary.Total = len(sorted)

	// Preload existing sites
	var existingSites []models.Site
	db.Where("tenant_id = ? AND deleted_at IS NULL", tenantID).Find(&existingSites)
	existingByName := make(map[string]models.Site)
	for _, s := range existingSites {
		if s.Name != "" {
			existingByName[s.Name] = s
		}
	}

	// Track newly created/updated sites for intra-batch parent resolution
	siteByName := make(map[string]siteInfo)
	for _, s := range existingSites {
		if s.Name != "" {
			siteByName[s.Name] = siteInfo{ID: s.ID, OrgID: s.OrgID}
		}
	}

	resolveParent := func(org OrgImportRecord) (*uuid.UUID, string, error) {
		pn := strings.TrimSpace(org.ParentName)
		if pn == "" || pn == "-" {
			return nil, "", nil
		}
		parent, ok := siteByName[pn]
		if !ok {
			return nil, "", fmt.Errorf("row %d: parent name '%s' not found", org.RowNum, pn)
		}
		pid, err := uuid.Parse(parent.ID)
		if err != nil {
			return nil, "", fmt.Errorf("row %d: invalid parent ID for '%s': %w", org.RowNum, pn, err)
		}
		return &pid, parent.OrgID, nil
	}

	for _, org := range sorted {
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

		existingSite, exists := existingByName[org.Name]
		if !exists {
			lower := strings.ToLower(org.Name)
			for _, s := range existingSites {
				if s.Name != "" && strings.ToLower(s.Name) == lower {
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
					Key:    org.Name,
					Action: "updated",
					Reason: "name exists, will update",
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
			// Validate topology: parent_name must match current parent
			expectedParentID, _, pErr := resolveParent(org)
			if pErr != nil {
				result.Summary.Failed++
				result.Details = append(result.Details, BulkImportDetail{
					Row:    org.RowNum,
					Key:    org.Name,
					Action: "failed",
					Reason: pErr.Error(),
				})
				continue
			}
			currentParentID := existingSite.ParentID
			if (expectedParentID == nil && currentParentID != nil) ||
				(expectedParentID != nil && currentParentID == nil) ||
				(expectedParentID != nil && currentParentID != nil && *expectedParentID != *currentParentID) {
				result.Summary.Failed++
				result.Details = append(result.Details, BulkImportDetail{
					Row:    org.RowNum,
					Key:    org.Name,
					Action: "failed",
					Reason: fmt.Sprintf("topology change rejected: existing parent '%v' does not match requested parent '%s'", currentParentID, org.ParentName),
				})
				continue
			}

			// Update existing site
			updates := map[string]interface{}{
				"type":    org.Type,
				"address": org.Address,
				"phone":   org.Phone,
			}
			if err := db.Model(&existingSite).Updates(updates).Error; err != nil {
				LogImportError("organization", org.RowNum, org.Name, err)
				result.Summary.Failed++
				result.Details = append(result.Details, BulkImportDetail{
					Row:    org.RowNum,
					Key:    org.Name,
					Action: "failed",
					Reason: fmt.Sprintf("update failed: %v", err),
				})
				continue
			}

			result.Summary.Updated++
			result.Details = append(result.Details, BulkImportDetail{
				Row:    org.RowNum,
				Key:    org.Name,
				Action: "updated",
			})
		} else {
			// Create new site
			parentID, parentOrgID, pErr := resolveParent(org)
			if pErr != nil {
				result.Summary.Failed++
				result.Details = append(result.Details, BulkImportDetail{
					Row:    org.RowNum,
					Key:    org.Name,
					Action: "failed",
					Reason: pErr.Error(),
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
				ParentID: parentID,
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

			// Track for intra-batch parent resolution
			siteByName[org.Name] = siteInfo{ID: newSite.ID, OrgID: orgID}

			// Create IAM organization with parent
			iamReq := &CreateOrganizationRequest{
				Name:        org.Name,
				NamespaceID: iamClient.GetNamespace(),
			}
			if parentID != nil && parentOrgID != "" {
				iamReq.ParentID = parentOrgID
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
	}

	return result, nil
}

// topoSortOrgsByName sorts organizations so parents are processed before children.
func topoSortOrgsByName(orgs []OrgImportRecord) ([]OrgImportRecord, error) {
	nameToOrg := make(map[string]OrgImportRecord)
	children := make(map[string][]string)
	inDegree := make(map[string]int)

	for _, o := range orgs {
		nameToOrg[o.Name] = o
		inDegree[o.Name] = 0
	}

	for _, o := range orgs {
		pn := strings.TrimSpace(o.ParentName)
		if pn != "" && pn != "-" {
			if _, ok := nameToOrg[pn]; ok {
				children[pn] = append(children[pn], o.Name)
				inDegree[o.Name]++
			}
		}
	}

	var queue []string
	for name, deg := range inDegree {
		if deg == 0 {
			queue = append(queue, name)
		}
	}

	var sorted []OrgImportRecord
	for len(queue) > 0 {
		name := queue[0]
		queue = queue[1:]
		sorted = append(sorted, nameToOrg[name])
		for _, child := range children[name] {
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
