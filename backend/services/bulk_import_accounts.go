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
	RoleTemplate     string
	OrganizationCode string
	Phone            string
	Tags             string
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
			RoleTemplate:     strings.TrimSpace(rec.Fields["role_template"]),
			OrganizationCode: strings.TrimSpace(rec.Fields["organization_code"]),
			Phone:            strings.TrimSpace(rec.Fields["phone"]),
			Tags:             strings.TrimSpace(rec.Fields["tags"]),
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

		existingUser, exists := existingByEmail[acc.Email]
		iamUser, iamExists := iamUserByEmail[acc.Email]

		if dryRun {
			if exists {
				result.Summary.Updated++
				result.Details = append(result.Details, BulkImportDetail{
					Row:    acc.RowNum,
					Key:    acc.Email,
					Action: "updated",
					Reason: "user exists, will update",
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

		// Compute permissions
		template := AllRoleTemplates[acc.RoleTemplate]

		tags := SplitTags(acc.Tags)
		tagsStr := strings.Join(tags, "|")

		if exists {
			// Update existing user
			updates := map[string]interface{}{
				"name":      acc.Name,
				"phone":     acc.Phone,
				"role":      acc.RoleTemplate,
				"user_type": "员工",
			}
			if siteID != nil {
				updates["site_id"] = *siteID
				updates["org_id"] = orgIDForLocal
			} else {
				updates["site_id"] = nil
			}
			if tagsStr != "" {
				updates["position"] = tagsStr
			}

			if err := db.Model(&existingUser).Updates(updates).Error; err != nil {
				LogImportError("account", acc.RowNum, acc.Email, err)
				result.Summary.Failed++
				result.Details = append(result.Details, BulkImportDetail{
					Row:    acc.RowNum,
					Key:    acc.Email,
					Action: "failed",
					Reason: fmt.Sprintf("local update failed: %v", err),
				})
				continue
			}

			// Update IAM user if exists
			if iamExists && iamUser.ID != "" {
				updateReq := &UpdateUserRequest{
					Name:  acc.Name,
					Email: acc.Email,
					Phone: acc.Phone,
				}
				if err := iamClient.UpdateUser(iamUser.ID, updateReq); err != nil {
					log.Printf("[BulkImport] IAM UpdateUser warning for %s: %v", acc.Email, err)
				}

				// Update IAM permissions
				if orgIDForIAM != "" {
					if err := iamClient.SetUserCustomerPermissions(orgIDForIAM, iamUser.ID, template.CusPermCodes); err != nil {
						log.Printf("[BulkImport] IAM SetUserCustomerPermissions warning for %s: %v", acc.Email, err)
					}
				}

				// Bind user to organization if applicable
				if orgIDForIAM != "" {
					if err := iamClient.BindUserToOrganization(iamUser.ID, orgIDForIAM, "member", ""); err != nil {
						log.Printf("[BulkImport] IAM BindUser warning for %s: %v", acc.Email, err)
					}
				}
			}

			result.Summary.Updated++
			result.Details = append(result.Details, BulkImportDetail{
				Row:    acc.RowNum,
				Key:    acc.Email,
				Action: "updated",
			})
		} else {
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
				Username: acc.Email,
				Name:     acc.Name,
				Email:    acc.Email,
				Phone:    acc.Phone,
			}
			iamResp, err := iamClient.CreateUser(createReq)
			if err != nil {
				if iamExists {
					// User already exists in IAM — link by IAMSub
					newUser.IAMSub = iamUser.ID
				} else if strings.Contains(err.Error(), "already exists") || strings.Contains(err.Error(), "conflict") {
					// Try to re-fetch the user from IAM to get the ID
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
						// Fallback: use email as IAMSub placeholder (should not normally happen)
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

			// Create local user
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

			// Set IAM permissions
			if newUser.IAMSub != "" && orgIDForIAM != "" {
				if err := iamClient.SetUserCustomerPermissions(orgIDForIAM, newUser.IAMSub, template.CusPermCodes); err != nil {
					log.Printf("[BulkImport] IAM SetUserCustomerPermissions warning for %s: %v", acc.Email, err)
				}
			}

			// Also bind user to organization if applicable
			if newUser.IAMSub != "" && orgIDForIAM != "" {
				if err := iamClient.BindUserToOrganization(newUser.IAMSub, orgIDForIAM, "member", ""); err != nil {
					log.Printf("[BulkImport] IAM BindUser warning for %s: %v", acc.Email, err)
				}
			}

			// Update cache
			existingByEmail[acc.Email] = newUser

			result.Summary.Created++
			result.Details = append(result.Details, BulkImportDetail{
				Row:    acc.RowNum,
				Key:    acc.Email,
				Action: "created",
			})
		}
	}

	return result, nil
}
