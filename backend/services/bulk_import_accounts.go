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

// AccountImportRecord represents a parsed account import row.
type AccountImportRecord struct {
	RowNum           int
	Email            string
	Name             string
	Username         string
	RoleTemplate     string
	OrganizationCode string
	Phone            string
	Type             string
}

// ImportAccountsCSV imports accounts from a CSV file.
func ImportAccountsCSV(ctx context.Context, r io.Reader, tenantID string, iamClient *IAMClient, permReg *PermissionRegistry, dryRun bool) (*BulkImportResult, error) {
	_, records, err := ParseCSV(r)
	if err != nil {
		return nil, err
	}

	// Parse records
	var accRecords []AccountImportRecord
	for _, rec := range records {
		accRecords = append(accRecords, AccountImportRecord{
			RowNum:           rec.RowNum,
			Email:            strings.TrimSpace(rec.Fields["email"]),
			Name:             strings.TrimSpace(rec.Fields["name"]),
			Username:         strings.TrimSpace(rec.Fields["username"]),
			RoleTemplate:     strings.TrimSpace(rec.Fields["role"]),
			OrganizationCode: strings.TrimSpace(rec.Fields["site"]),
			Phone:            strings.TrimSpace(rec.Fields["phone"]),
			Type:             strings.TrimSpace(rec.Fields["type"]),
		})
	}

	// Deduplicate by email (keep last)
	seen := make(map[string]int)
	for i, a := range accRecords {
		if a.Email != "" {
			seen[a.Email] = i
		}
	}
	var deduped []AccountImportRecord
	for i, a := range accRecords {
		if a.Email == "" || seen[a.Email] == i {
			deduped = append(deduped, a)
		}
	}
	accRecords = deduped

	db := database.GetDB().WithContext(ctx)
	result := &BulkImportResult{}
	result.Summary.Total = len(accRecords)

	// Preload existing users by email
	var existingUsers []models.User
	db.Where("tenant_id = ?", tenantID).Find(&existingUsers)
	existingByEmail := make(map[string]models.User)
	for _, u := range existingUsers {
		if u.Email != "" {
			existingByEmail[u.Email] = u
		}
	}

	// Preload sites by organization_code and name (lowercase for case-insensitive lookup)
	var sites []models.Site
	db.Where("tenant_id = ? AND deleted_at IS NULL", tenantID).Find(&sites)
	siteByCode := make(map[string]models.Site)
	siteByName := make(map[string]models.Site)
	for _, s := range sites {
		if s.OrganizationCode != "" {
			siteByCode[strings.ToLower(s.OrganizationCode)] = s
		}
		if s.Name != "" {
			siteByName[strings.ToLower(s.Name)] = s
		}
	}

	// Preload IAM users (best effort, log warning on failure)
	iamUsers, err := iamClient.ListUsers()
	if err != nil {
		log.Printf("[BulkImport] Warning: failed to list IAM users: %v — existing users may be re-created", err)
	}
	iamUserByEmail := make(map[string]User)
	for _, u := range iamUsers {
		if u.Email != "" {
			iamUserByEmail[u.Email] = u
		}
	}

	for _, acc := range accRecords {
		// Validate required fields
		if acc.Email == "" {
			result.Summary.Failed++
			result.Details = append(result.Details, BulkImportDetail{
				Row:    acc.RowNum,
				Key:    "",
				Action: "failed",
				Reason: "email is required",
			})
			continue
		}
		if err := ValidateEmail(acc.Email); err != nil {
			result.Summary.Failed++
			result.Details = append(result.Details, BulkImportDetail{
				Row:    acc.RowNum,
				Key:    acc.Email,
				Action: "failed",
				Reason: err.Error(),
			})
			continue
		}
		if acc.Name == "" {
			result.Summary.Failed++
			result.Details = append(result.Details, BulkImportDetail{
				Row:    acc.RowNum,
				Key:    acc.Email,
				Action: "failed",
				Reason: "name is required",
			})
			continue
		}
		if acc.RoleTemplate == "" {
			result.Summary.Failed++
			result.Details = append(result.Details, BulkImportDetail{
				Row:    acc.RowNum,
				Key:    acc.Email,
				Action: "failed",
				Reason: "role_template is required",
			})
			continue
		}
		if err := ValidateRoleTemplate(acc.RoleTemplate); err != nil {
			result.Summary.Failed++
			result.Details = append(result.Details, BulkImportDetail{
				Row:    acc.RowNum,
				Key:    acc.Email,
				Action: "failed",
				Reason: err.Error(),
			})
			continue
		}

		// Resolve organization
		var siteID *string
		var orgIDForLocal string
		var orgIDForIAM string
		if acc.OrganizationCode != "" {
			lowerCode := strings.ToLower(acc.OrganizationCode)
			site, ok := siteByCode[lowerCode]
			if !ok {
				site, ok = siteByName[strings.ToLower(acc.Name)]
			}
			if !ok {
				result.Summary.Failed++
				result.Details = append(result.Details, BulkImportDetail{
					Row:    acc.RowNum,
					Key:    acc.Email,
					Action: "failed",
					Reason: fmt.Sprintf("organization_code not found: %s", acc.OrganizationCode),
				})
				continue
			}
			siteID = &site.ID
			orgIDForLocal = site.OrgID
			orgIDForIAM = site.OrgID
		} else {
			orgIDForLocal = tenantID
		}

		_, exists := existingByEmail[acc.Email]
		iamUser, iamExists := iamUserByEmail[acc.Email]

		if dryRun {
			if exists || iamExists {
				result.Summary.Skipped++
				result.Details = append(result.Details, BulkImportDetail{
					Row:    acc.RowNum,
					Key:    acc.Email,
					Action: "skipped",
					Reason: "user already exists, skipped (create-only)",
				})
			} else {
				result.Summary.Created++
				result.Details = append(result.Details, BulkImportDetail{
					Row:    acc.RowNum,
					Key:    acc.Email,
					Action: "created",
					Reason: "new user",
				})
			}
			continue
		}

		if exists || iamExists {
			result.Summary.Skipped++
			result.Details = append(result.Details, BulkImportDetail{
				Row:    acc.RowNum,
				Key:    acc.Email,
				Action: "skipped",
				Reason: "user already exists, skipped (create-only)",
			})
			continue
		}

		// Compute permissions
		template := AllRoleTemplates[acc.RoleTemplate]

		tagsStr := ""

		// Create new user
		newUserID := uuid.New().String()
		newUser := models.User{
			ID:       newUserID,
			TenantID: tenantID,
			OrgID:    orgIDForLocal,
			Name:     acc.Name,
			Email:    acc.Email,
			Phone:    acc.Phone,
			Role:     acc.RoleTemplate,
			Status:   "active",
			UserType: "员工",
			IsShadow: false,
		}
		if siteID != nil {
			newUser.SiteID = siteID
		}
		if tagsStr != "" {
			newUser.Position = tagsStr
		}

		// Create IAM user first
		createReq := &CreateUserRequest{
			Username: acc.Username,
			Name:     acc.Name,
			Email:    acc.Email,
			Phone:    acc.Phone,
		}
		iamResp, err := iamClient.CreateUser(createReq)
		if err != nil {
			if iamExists {
				newUser.IAMSub = iamUser.ID
			} else if strings.Contains(err.Error(), "already exists") || strings.Contains(err.Error(), "conflict") {
				refreshedUsers, refreshErr := iamClient.ListUsers()
				if refreshErr == nil {
					for _, u := range refreshedUsers {
						if strings.EqualFold(u.Email, acc.Email) {
							newUser.IAMSub = u.ID
							break
						}
					}
				}
				if newUser.IAMSub == "" {
					log.Printf("[BulkImport] IAM user exists but could not resolve ID for %s", acc.Email)
					newUser.IAMSub = acc.Email
				}
			} else {
				LogImportError("account", acc.RowNum, acc.Email, err)
				result.Summary.Failed++
				result.Details = append(result.Details, BulkImportDetail{
					Row:    acc.RowNum,
					Key:    acc.Email,
					Action: "failed",
					Reason: fmt.Sprintf("IAM create failed: %v", err),
				})
				continue
			}
		} else {
			newUser.IAMSub = iamResp.UserID
		}

		if err := db.Create(&newUser).Error; err != nil {
			LogImportError("account", acc.RowNum, acc.Email, err)
			result.Summary.Failed++
			result.Details = append(result.Details, BulkImportDetail{
				Row:    acc.RowNum,
				Key:    acc.Email,
				Action: "failed",
				Reason: fmt.Sprintf("local create failed: %v", err),
			})
			continue
		}

		if newUser.IAMSub != "" && orgIDForIAM != "" {
			if err := iamClient.SetUserCustomerPermissions(orgIDForIAM, newUser.IAMSub, template.CusPermCodes); err != nil {
				log.Printf("[BulkImport] IAM SetUserCustomerPermissions warning for %s: %v", acc.Email, err)
			}
		}

		if newUser.IAMSub != "" && orgIDForIAM != "" {
			iamRole := GetBusinessRole(acc.RoleTemplate)
			if iamRole == "" {
				iamRole = "member"
			}
			if err := iamClient.BindUserToOrganization(newUser.IAMSub, orgIDForIAM, iamRole, ""); err != nil {
				log.Printf("[BulkImport] IAM BindUser warning for %s: %v", acc.Email, err)
			}
		}

		existingByEmail[acc.Email] = newUser

		result.Summary.Created++
		result.Details = append(result.Details, BulkImportDetail{
			Row:    acc.RowNum,
			Key:    acc.Email,
			Action: "created",
		})
	}

	return result, nil
}
