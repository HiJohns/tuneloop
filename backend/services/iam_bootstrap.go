package services

import (
	"fmt"
	"log"
	"os"
	"sync"
	"tuneloop-backend/models"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var (
	appCredentials     = make(map[string]string) // client_id -> client_secret
	appCredentialsLock sync.RWMutex
)

func GetAppSecret(clientID string) string {
	appCredentialsLock.RLock()
	defer appCredentialsLock.RUnlock()
	return appCredentials[clientID]
}

func BootstrapIAM(db *gorm.DB) error {
	// Register OAuth apps (tuneloop_web / tuneloop_wechat) with IAM.
	// Must run unconditionally — does not depend on BOOTSTRAP_CLIENT_ID.
	iamNs := os.Getenv("IAM_NAMESPACE")
	iamSecret := os.Getenv("IAM_SECRET")
	if iamNs != "" && iamSecret != "" {
		iamClient := NewIAMClient()
		pcRedirect := os.Getenv("EXTERNAL_WEB_URL")
		if pcRedirect == "" {
			pcRedirect = "http://localhost:5554"
		}
		pcRedirect += "/callback"
		wxRedirect := os.Getenv("EXTERNAL_MOBILE_URL")
		if wxRedirect == "" {
			wxRedirect = "http://localhost:5553"
		}
		wxRedirect += "/callback"

		apps := []AppRegistration{
			{AppType: "web", RedirectURIs: []string{pcRedirect}, IsDefault: true},
			{AppType: "wechat", RedirectURIs: []string{wxRedirect}},
		}

		// Try ActivateNamespace (beaconiam #177 — also creates org, returns org_id + apps)
		// Fatal on failure — silent fallback hides configuration issues.
		activateResp, actErr := iamClient.ActivateNamespace(iamNs, apps)
		if actErr != nil {
			return fmt.Errorf("namespace activation failed (check IAM server and namespace secret): %w", actErr)
		}
		appCredentialsLock.Lock()
		for _, app := range activateResp.Apps {
			appCredentials[app.ClientID] = app.ClientSecret
		}
		if activateResp.OrgID != "" {
			appCredentials["_org_id"] = activateResp.OrgID
		}
		appCredentialsLock.Unlock()
		log.Printf("[Bootstrap] Namespace activated: org_id=%s, apps=%d", activateResp.OrgID, len(activateResp.Apps))
	}

	// Cold start: create admin user if no local users exist.
	// IAM generates a random password and sends it via email (requires beaconiam #169).
	if iamNs != "" && iamSecret != "" {
		var userCount int64
		db.Model(&models.User{}).Count(&userCount)
		if userCount == 0 {
			adminEmail := os.Getenv("ADMIN_EMAIL")
			if adminEmail == "" {
				return fmt.Errorf("ADMIN_EMAIL not set — required for cold start. Add ADMIN_EMAIL to .env with the admin user's email address")
			}
			iamClient := NewIAMClient()
			_, err := iamClient.CreateAdminUser(iamNs, adminEmail, "Administrator")
			if err != nil {
				log.Printf("[Bootstrap] Warning: cold start admin creation failed: %v", err)
			} else {
				log.Printf("[Bootstrap] Cold start: admin user created (%s)", adminEmail)
				// beaconiam #202: RegisterAdmin now creates org + binds admin in one step.
				// No separate BindUserToOrg call needed.
				// Save admin to local users table so cold start check skips on restart.
				// Check by email to avoid duplicates (adminUserID may be empty if IAM API
				// does not return the user ID in the response).
				var existingUser models.User
				if err := db.Where("email = ?", adminEmail).First(&existingUser).Error; err != nil {
					appCredentialsLock.RLock()
					orgID := appCredentials["_org_id"]
					appCredentialsLock.RUnlock()
					if orgID == "" {
						orgID = "00000000-0000-0000-0000-000000000000"
					}
					tenantID := orgID
					localUser := models.User{
						IAMSub:   adminEmail,
						Name:     "Administrator",
						Email:    adminEmail,
						TenantID: tenantID,
						OrgID:    orgID,
					}
					if err := db.Create(&localUser).Error; err != nil {
						log.Printf("[Bootstrap] Warning: failed to save admin to local DB: %v", err)
					} else {
						log.Printf("[Bootstrap] Admin saved to local users table")
						tenantRecord := models.Tenant{
							ID:   orgID,
							Name: iamNs,
						}
						if err := db.Clauses(clause.OnConflict{DoNothing: true}).Create(&tenantRecord).Error; err != nil {
							log.Printf("[Bootstrap] Warning: failed to save tenant record: %v", err)
						}
					}
				}
			}
		}
	}

	// Sync IAM organizations and users to local tables on startup.
	// This ensures data persists across server restarts and recovers
	// organizations created directly in IAM.
	syncIAMOrganizations(db, iamNs)

	bootstrapClientID := os.Getenv("BOOTSTRAP_CLIENT_ID")
	if bootstrapClientID == "" {
		return nil
	}

	var count int64
	db.Model(&models.Client{}).Where("client_id = ?", bootstrapClientID).Count(&count)
	if count > 0 {
		fmt.Println("Bootstrap: Client already exists, skipping")
		return nil
	}

	bootstrapClientSecret := os.Getenv("BOOTSTRAP_CLIENT_SECRET")
	if bootstrapClientSecret == "" {
		bootstrapClientSecret = "bootstrap-secret-" + bootstrapClientID
	}

	redirectURIs := os.Getenv("BOOTSTRAP_REDIRECT_URIS")
	if redirectURIs == "" {
		wwwPort := os.Getenv("TUNELOOP_WWW_PORT")
		if wwwPort == "" {
			wwwPort = "5554"
		}
		wxPort := os.Getenv("TUNELOOP_WX_PORT")
		if wxPort == "" {
			wxPort = "5553"
		}
		redirectURIs = fmt.Sprintf("http://localhost:%s/callback,http://localhost:%s/callback", wwwPort, wxPort)
	}

	client := &models.Client{
		ClientID:     bootstrapClientID,
		ClientSecret: bootstrapClientSecret,
		Name:         "Bootstrap Client",
		RedirectURIs: redirectURIs,
	}

	if err := db.Create(client).Error; err != nil {
		return fmt.Errorf("failed to create bootstrap client: %w", err)
	}

	fmt.Printf("Bootstrap: Created IAM Client '%s'\n", bootstrapClientID)
	return nil
}

// syncIAMOrganizations synchronizes IAM organizations and users into local tables.
// Merchants = top-level orgs (no parent, name != namespace).
// Sites = child orgs (have parent).
// Users are mapped by iam_sub to avoid duplicates.
func syncIAMOrganizations(db *gorm.DB, iamNs string) {
	if iamNs == "" {
		return
	}
	iamSecret := os.Getenv("IAM_SECRET")
	if iamSecret == "" {
		return
	}

	client := NewIAMClient()

	// Fetch all organizations from IAM
	orgs, err := client.ListOrganizations()
	if err != nil {
		log.Printf("[Bootstrap] Warning: failed to list IAM organizations: %v", err)
		return
	}

	appCredentialsLock.RLock()
	namespaceOrgID := appCredentials["_org_id"]
	appCredentialsLock.RUnlock()
	if namespaceOrgID == "" {
		namespaceOrgID = "00000000-0000-0000-0000-000000000000"
	}

	for _, org := range orgs {
		if org.Name == iamNs {
			continue
		}

		// Check if this org already exists as a merchant or site
		var merchantCount int64
		db.Model(&models.Merchant{}).Where("org_id = ?", org.ID).Count(&merchantCount)
		var siteCount int64
		db.Model(&models.Site{}).Where("org_id = ?", org.ID).Count(&siteCount)

		if merchantCount > 0 || siteCount > 0 {
			continue
		}

		if org.ParentID == nil || *org.ParentID == "" {
			merchant := models.Merchant{
				ID:       uuid.New().String(),
				TenantID: namespaceOrgID,
				OrgID:    org.ID,
				Name:     org.Name,
				Code:     org.Name,
				AdminUID: "00000000-0000-0000-0000-000000000000",
				Status:   "active",
			}
			if err := db.Clauses(clause.OnConflict{DoNothing: true}).Create(&merchant).Error; err != nil {
				log.Printf("[Bootstrap] Warning: failed to create merchant from IAM org %s: %v", org.Name, err)
			} else {
				log.Printf("[Bootstrap] Synced IAM org %s -> merchant", org.Name)
			}
		} else {
			site := models.Site{
				ID:       uuid.New().String(),
				TenantID: namespaceOrgID,
				OrgID:    org.ID,
				Name:     org.Name,
				Status:   "active",
			}
			if err := db.Clauses(clause.OnConflict{DoNothing: true}).Create(&site).Error; err != nil {
				log.Printf("[Bootstrap] Warning: failed to create site from IAM org %s: %v", org.Name, err)
			} else {
				log.Printf("[Bootstrap] Synced IAM org %s -> site", org.Name)
			}
		}
	}

	// Sync IAM users to local users table
	users, err := client.ListUsers()
	if err != nil {
		log.Printf("[Bootstrap] Warning: failed to list IAM users: %v", err)
		return
	}

	for _, u := range users {
		if u.ID == "" {
			continue
		}
		orgID := u.OrgID
		if orgID == "" {
			orgID = namespaceOrgID
		}
		var existingUser models.User
		if err := db.Where("iam_sub = ?", u.ID).First(&existingUser).Error; err != nil {
			localUser := models.User{
				ID:       uuid.New().String(),
				IAMSub:   u.ID,
				Name:     u.Name,
				Email:    u.Email,
				Phone:    u.Phone,
				TenantID: namespaceOrgID,
				OrgID:    orgID,
				Status:   "active",
			}
			if u.Status != "" {
				localUser.Status = u.Status
			}
			if err := db.Clauses(clause.OnConflict{DoNothing: true}).Create(&localUser).Error; err != nil {
				log.Printf("[Bootstrap] Warning: failed to sync IAM user %s: %v", u.Email, err)
			} else {
				log.Printf("[Bootstrap] Synced IAM user %s -> local users", u.Email)
			}
		}
	}
}
