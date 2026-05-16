package services

import (
	"fmt"
	"log"
	"os"
	"sync"
	"tuneloop-backend/models"

	"gorm.io/gorm"
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
		pcRedirect := os.Getenv("IAM_PC_REDIRECT_URI")
		if pcRedirect == "" {
			pcRedirect = "http://localhost:5554/callback"
		}
		wxRedirect := os.Getenv("IAM_WX_REDIRECT_URI")
		if wxRedirect == "" {
			wxRedirect = "http://localhost:5553/callback"
		}

		apps := []AppRegistration{
			{AppType: "web", RedirectURIs: []string{pcRedirect}},
			{AppType: "wechat", RedirectURIs: []string{wxRedirect}},
		}

		// Try ActivateNamespace (beaconiam #177 — also creates org, returns org_id + apps)
		activateResp, actErr := iamClient.ActivateNamespace(iamNs, apps)
		if actErr == nil {
			appCredentialsLock.Lock()
			for _, app := range activateResp.Apps {
				appCredentials[app.ClientID] = app.ClientSecret
			}
			if activateResp.OrgID != "" {
				appCredentials["_org_id"] = activateResp.OrgID
			}
			appCredentialsLock.Unlock()
			log.Printf("[Bootstrap] Namespace activated: org_id=%s, apps=%d", activateResp.OrgID, len(activateResp.Apps))
		} else {
			log.Printf("[Bootstrap] ActivateNamespace failed (may not be deployed yet), falling back to RegisterApp: %v", actErr)
			for _, app := range apps {
				resp, err := iamClient.RegisterNamespaceApp(iamNs, app.AppType, app.RedirectURIs[0])
				if err != nil {
					log.Printf("[Bootstrap] Warning: failed to register IAM app %s_%s: %v", iamNs, app.AppType, err)
				} else {
					appCredentialsLock.Lock()
					appCredentials[resp.ClientID] = resp.ClientSecret
					appCredentialsLock.Unlock()
					log.Printf("[Bootstrap] Registered IAM app: client_id=%s", resp.ClientID)
				}
			}
		}
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
			adminUserID, err := iamClient.CreateAdminUser(iamNs, adminEmail, "Administrator")
			if err != nil {
				log.Printf("[Bootstrap] Warning: cold start admin creation failed: %v", err)
			} else {
				log.Printf("[Bootstrap] Cold start: admin user created (%s)", adminEmail)
				// Bind admin to org if org_id was returned by ActivateNamespace
				if adminUserID != "" {
					appCredentialsLock.RLock()
					orgID := appCredentials["_org_id"]
					appCredentialsLock.RUnlock()
					if orgID != "" {
						if bindErr := iamClient.BindUserToOrg(orgID, adminUserID); bindErr != nil {
							log.Printf("[Bootstrap] Warning: failed to bind admin to org: %v", bindErr)
						} else {
							log.Printf("[Bootstrap] Admin bound to org %s", orgID)
						}
					}
				}
			}
		}
	}

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
